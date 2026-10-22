package mcpserver

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/account"
	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

const instructions = `Folio typesets study notes onto A4 pages. You write structured blocks; Folio measures text, flows it across pages and handles breaks. Never think in coordinates.

Notebooks are dark by default; pass mode "light" to create_notebook (or set_theme) only when the user asks for light/printing. Density (compact, comfortable default, airy) adds gaps automatically.
Every block has a stable ID (b_xxxxxxxx); use IDs for small targeted edits instead of rewriting.

Layout rules (full version in get_authoring_guide):
- Each H1 section fits on one page; put a page_break before every H1 except the first.
- Aim for 60-95% page fill. Under 50% before a page_break/new H1, or a few % spilling onto the next page, are problems to fix.
- Don't stack everything top to bottom: put related things side by side with two_column (half diagram left, steps/bullets/callout right; a column may start with a level-3 heading). Pair short callouts.
- Spacing comes from density; don't add spacers before every H2. In "legacy" notebooks switch density to comfortable via set_theme.
- Diagrams: at most ~6 nodes, 1-3 word labels, at most ~40% of the page tall. Prefer size half inside two_column (about 3 nodes across); chains of 4+ nodes go full width with direction: right.
- Short paragraphs (2-4 sentences); tables for comparisons, lists for enumerations; end each section with 1-2 qa or fill_in.
- Text is left-aligned; tables align text left and numbers right automatically.

Workflow:
1. get_authoring_guide if you haven't, then create_notebook (or list_notebooks).
2. Write a whole section per append_blocks call.
3. Read each write's layout, warnings and layout_hints; fix issues before moving on.
4. Before saying done: check_layout, fix what it reports, then render_pages once on the changed pages (max 2).
5. The user's notebook_guidelines (get_outline, first write response, get_notebook_guidelines) override these rules; save lasting preferences with set_notebook_guidelines.`

//go:embed guide.md
var guideBody string

//go:embed layout_rules.md
var layoutRules string

var guide = guideBody + "\n\n# Layout rules\n\n" + layoutRules

var papers = []string{"plain", "ruled", "dotted", "grid"}

type Server struct {
	st        *store.Store
	lb        *layout.Builder
	home      string
	publicURL string
	localUser string
	renders   *account.Limiter
	seenMu    sync.Mutex
	seen      map[string]time.Time
}

const sessionWindow = 30 * time.Minute

func (s *Server) firstWrite(user, notebook string) bool {
	s.seenMu.Lock()
	defer s.seenMu.Unlock()
	now := time.Now()
	for k, t := range s.seen {
		if now.Sub(t) > sessionWindow {
			delete(s.seen, k)
		}
	}
	k := user + "|" + notebook
	_, ok := s.seen[k]
	s.seen[k] = now
	return !ok
}

func (s *Server) allowRender(ctx context.Context) error {
	if !s.renders.Allow(userOf(ctx)) {
		return errors.New("rendering limit reached (30 renders/exports per minute); wait a minute and try again")
	}
	return nil
}

const blockDescription = `One block. Set "type" plus that type's fields:
- heading: level (1-3), text. Level 1 = section title.
- paragraph: text (max 2000 chars).
- bullets / numbered: items: [{text, children?: [items]}]; nesting depth max 3, max 40 items total.
- callout: variant (note|tip|warning|definition|key_point), title? (string), body: [string, ...] one string per paragraph.
- code: language (e.g. "go", "python", "" for plain), code (max 80 lines), caption?.
- table: columns: [2-6 header strings], rows: [[cell, ...], ...] (max 25 rows, each row exactly len(columns) cells), caption?, align? ([left|center|right per column]; default left, numeric columns right).
- qa: question, answer, hide_answer? (bool; true prints the answer upside down at the page bottom for self-testing).
- fill_in: prompt, lines (1-15). Blank ruled lines for the reader to write on.
- spacer: size (small|medium|large). Vertical space.
- two_column: left: [blocks], right: [blocks]. Side-by-side columns. Nested blocks may be heading (level 3 only), paragraph, bullets, numbered, callout, code, diagram (size half or small) or qa; they get their own IDs, which update_block, delete_blocks and read_blocks accept.
- diagram: source (D2 language, e.g. "client -> server: request"), size? (full|half|small, default full), caption?. Compile errors are returned with line numbers; use preview_diagram to check one first.
- divider: no fields. Horizontal rule.
- page_break: no fields. Starts a new page.
Text fields accept only this inline markdown: **bold**, *italic*, ` + "`code`" + `, [text](url), ==highlight==.
Unknown fields are rejected.`

var blockSchema = &jsonschema.Schema{
	Type:        "object",
	Description: blockDescription,
	Required:    []string{"type"},
	Properties: map[string]*jsonschema.Schema{
		"type": {Type: "string", Enum: anySlice(model.Types())},
	},
}

func anySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func schemaFor[T any]() *jsonschema.Schema {
	s, err := jsonschema.For[T](&jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[json.RawMessage](): blockSchema,
		reflect.TypeFor[model.Block]():     {Type: "object", Description: "A block: id, type and its type-specific fields"},
	}})
	if err != nil {
		panic(err)
	}
	return s
}

type userKey struct{}

func userOf(ctx context.Context) string {
	uid, _ := ctx.Value(userKey{}).(string)
	return uid
}

func (s *Server) userID(req *mcp.CallToolRequest) (string, error) {
	if req != nil && req.Extra != nil && req.Extra.TokenInfo != nil && req.Extra.TokenInfo.UserID != "" {
		return req.Extra.TokenInfo.UserID, nil
	}
	if s.localUser != "" {
		return s.localUser, nil
	}
	return "", errors.New("authentication required: connect with a Folio token (create one on the Connect page)")
}

func addTool[In, Out any](srv *Server, s *mcp.Server, name, desc string, h mcp.ToolHandlerFor[In, Out]) {
	tool := &mcp.Tool{Name: name, Description: desc, InputSchema: schemaFor[In]()}
	if reflect.TypeFor[Out]() != reflect.TypeFor[any]() {
		tool.OutputSchema = schemaFor[Out]()
	}
	mcp.AddTool(s, tool, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		uid, err := srv.userID(req)
		if err != nil {
			var zero Out
			return nil, zero, err
		}
		ctx = context.WithValue(ctx, userKey{}, uid)
		res, out, err := h(ctx, req, in)
		if err != nil {
			slog.Warn("tool error", "tool", name, "err", err)
			err = friendly(err)
		}
		return res, out, err
	})
}

func friendly(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound) && strings.HasPrefix(err.Error(), store.ErrNotFound.Error()+":"):
		return err
	case errors.Is(err, store.ErrNotFound):
		return errors.New("notebook not found; call list_notebooks to see valid notebook IDs")
	}
	return err
}

func New(st *store.Store, lb *layout.Builder, home, publicURL, localUser string) *mcp.Server {
	srv := &Server{st: st, lb: lb, home: home, publicURL: strings.TrimSuffix(publicURL, "/"), localUser: localUser, renders: account.NewLimiter(30, time.Minute), seen: map[string]time.Time{}}
	s := mcp.NewServer(&mcp.Implementation{Name: "folio", Version: "0.1.0"}, &mcp.ServerOptions{Instructions: instructions})
	srv.register(s)
	srv.registerEdit(s)
	s.AddResource(&mcp.Resource{URI: "folio://guide", Name: "authoring-guide", Title: "Folio authoring guide", MIMEType: "text/markdown"},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: "folio://guide", MIMEType: "text/markdown", Text: guide}}}, nil
		})
	return s
}

type BlockPage struct {
	ID       string  `json:"id"`
	Page     int     `json:"page"`
	HeightMM float64 `json:"height_mm,omitempty"`
}

type LayoutSummary struct {
	PageCount      int         `json:"page_count"`
	Blocks         []BlockPage `json:"blocks"`
	PageFill       []float64   `json:"page_fill_pct"`
	LastPageFill   float64     `json:"last_page_fill_pct"`
	LastPageFreeMM float64     `json:"last_page_free_mm"`
}

type MutationResult struct {
	Version            int           `json:"version"`
	ChangedBlockIDs    []string      `json:"changed_block_ids"`
	Layout             LayoutSummary `json:"layout"`
	Warnings           []string      `json:"warnings"`
	LayoutHints        []string      `json:"layout_hints,omitempty"`
	NotebookGuidelines string        `json:"notebook_guidelines,omitempty"`
}

func (s *Server) mutate(ctx context.Context, id string, expected *int, tool, summary string, fn func(nb *store.Notebook, bs []model.Block) ([]model.Block, []string, error)) (MutationResult, error) {
	var changed []string
	nb, bs, err := s.st.Mutate(userOf(ctx), id, expected, tool, summary, func(nb *store.Notebook, bs []model.Block) ([]model.Block, error) {
		next, ch, err := fn(nb, bs)
		changed = ch
		return next, err
	})
	if err != nil {
		return MutationResult{}, err
	}
	return s.result(ctx, nb, bs, changed), nil
}

func (s *Server) result(ctx context.Context, nb store.Notebook, bs []model.Block, changed []string) MutationResult {
	res := MutationResult{Version: nb.Version, ChangedBlockIDs: changed, Warnings: []string{}}
	if res.ChangedBlockIDs == nil {
		res.ChangedBlockIDs = []string{}
	}
	if s.firstWrite(userOf(ctx), nb.ID) {
		res.NotebookGuidelines = nb.Guidelines
	}
	r, err := s.lb.Build(ctx, nb, bs)
	if err != nil {
		res.Warnings = append(res.Warnings, "the change was saved but rendering failed: "+err.Error())
		return res
	}
	res.Layout = LayoutSummary{PageCount: r.PageCount, PageFill: r.PageFill, LastPageFill: r.LastPageFill, LastPageFreeMM: r.LastPageFreeMM(), Blocks: []BlockPage{}}
	res.Warnings = append(res.Warnings, r.Warnings...)
	res.LayoutHints = r.Hints()
	for _, id := range changed {
		res.Layout.Blocks = append(res.Layout.Blocks, BlockPage{id, r.Page(id), r.Height(id)})
	}
	return res
}

func validTheme(theme string) error {
	_, err := layout.ThemeByName(theme)
	return err
}

func validDensity(density string) error {
	if !slices.Contains(store.Densities, density) {
		return fmt.Errorf("unknown density %q; valid: %s", density, strings.Join(store.Densities, ", "))
	}
	return nil
}

func validMode(mode string) error {
	if !slices.Contains(layout.Modes, mode) {
		return fmt.Errorf("unknown mode %q; valid: dark, light", mode)
	}
	return nil
}

func validPaper(paper string) error {
	if !slices.Contains(papers, paper) {
		return fmt.Errorf("unknown paper %q; valid: %s", paper, strings.Join(papers, ", "))
	}
	return nil
}

func newIDs(bs []model.Block) []string {
	ids := make([]string, len(bs))
	for i := range bs {
		bs[i].ID = model.NewID("b_")
		bs[i] = bs[i].RenewIDs(func(string) bool { return false })
		ids[i] = bs[i].ID
	}
	return ids
}

func parentOf(bs []model.Block, id string) int {
	for i, b := range bs {
		for _, k := range b.Children() {
			if k.ID == id {
				return i
			}
		}
	}
	return -1
}

func findAny(bs []model.Block, id string) (model.Block, bool) {
	if i := indexOf(bs, id); i >= 0 {
		return bs[i], true
	}
	if p := parentOf(bs, id); p >= 0 {
		for _, k := range bs[p].Children() {
			if k.ID == id {
				return k, true
			}
		}
	}
	return model.Block{}, false
}

func revalidate(b model.Block) (model.Block, error) {
	js, _ := json.Marshal(b)
	nb, err := model.Parse(js)
	if err != nil {
		return b, fmt.Errorf("two_column %s: %v", b.ID, err)
	}
	nb.ID, nb.Position, nb.CreatedAt = b.ID, b.Position, b.CreatedAt
	return nb, nil
}
