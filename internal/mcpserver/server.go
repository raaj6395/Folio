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

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

const instructions = `Folio typesets study notes onto A4 pages.

You write structured blocks (headings, paragraphs, lists, callouts, code, tables, diagrams, Q&A cards); Folio measures text, flows it across pages and handles page breaks. Never think in coordinates.

Every block has a stable ID (b_xxxxxxxx). Use IDs for small targeted edits instead of rewriting a notebook.

Recommended workflow: get_authoring_guide once → create_notebook → plan an outline → append_blocks section by section → read the layout summary each write returns → render_pages for a page or two to check how it looks → fix problems → export_pdf.`

//go:embed guide.md
var guide string

var themes = map[string]string{
	"clean": "Clean typeset look: Inter, crisp boxes, plain paper.",
}

var papers = []string{"plain", "ruled", "dotted", "grid"}

type Server struct {
	st   *store.Store
	lb   *layout.Builder
	home string
}

const blockDescription = `One block. Set "type" plus that type's fields:
- heading: level (1-3), text. Level 1 = section title.
- paragraph: text (max 2000 chars).
- bullets / numbered: items: [{text, children?: [items]}]; nesting depth max 3, max 40 items total.
- callout: variant (note|tip|warning|definition|key_point), title? (string), body: [string, ...] one string per paragraph.
- code: language (e.g. "go", "python", "" for plain), code (max 80 lines), caption?.
- table: columns: [2-6 header strings], rows: [[cell, ...], ...] (max 25 rows, each row exactly len(columns) cells), caption?.
- qa: question, answer, hide_answer? (bool; true prints the answer upside down at the page bottom for self-testing).
- fill_in: prompt, lines (1-15). Blank ruled lines for the reader to write on.
- spacer: size (small|medium|large). Vertical space.
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

func addTool[In, Out any](s *mcp.Server, name, desc string, h mcp.ToolHandlerFor[In, Out]) {
	tool := &mcp.Tool{Name: name, Description: desc, InputSchema: schemaFor[In]()}
	if reflect.TypeFor[Out]() != reflect.TypeFor[any]() {
		tool.OutputSchema = schemaFor[Out]()
	}
	mcp.AddTool(s, tool, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
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

func New(st *store.Store, lb *layout.Builder, home string) *mcp.Server {
	srv := &Server{st: st, lb: lb, home: home}
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
	ID   string `json:"id"`
	Page int    `json:"page"`
}

type LayoutSummary struct {
	PageCount    int         `json:"page_count"`
	Blocks       []BlockPage `json:"blocks"`
	LastPageFill float64     `json:"last_page_fill_pct"`
}

type MutationResult struct {
	Version         int           `json:"version"`
	ChangedBlockIDs []string      `json:"changed_block_ids"`
	Layout          LayoutSummary `json:"layout"`
	Warnings        []string      `json:"warnings"`
}

func (s *Server) mutate(ctx context.Context, id string, expected *int, tool, summary string, fn func(nb *store.Notebook, bs []model.Block) ([]model.Block, []string, error)) (MutationResult, error) {
	var changed []string
	nb, bs, err := s.st.Mutate(id, expected, tool, summary, func(nb *store.Notebook, bs []model.Block) ([]model.Block, error) {
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
	r, err := s.lb.Build(ctx, nb, bs)
	if err != nil {
		res.Warnings = append(res.Warnings, "the change was saved but rendering failed: "+err.Error())
		return res
	}
	res.Layout = LayoutSummary{PageCount: r.PageCount, LastPageFill: r.LastPageFill, Blocks: []BlockPage{}}
	res.Warnings = append(res.Warnings, r.Warnings...)
	for _, id := range changed {
		res.Layout.Blocks = append(res.Layout.Blocks, BlockPage{id, r.Page(id)})
	}
	return res
}

func validTheme(theme string) error {
	if _, ok := themes[theme]; !ok {
		names := []string{}
		for n := range themes {
			names = append(names, n)
		}
		slices.Sort(names)
		return fmt.Errorf("unknown theme %q; valid themes: %s (see list_themes)", theme, strings.Join(names, ", "))
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
		ids[i] = bs[i].ID
	}
	return ids
}
