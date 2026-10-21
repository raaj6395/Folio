package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

type empty struct{}

type notebookIn struct {
	NotebookID string `json:"notebook_id" jsonschema:"ID of the notebook (n_xxxxxxxx), from list_notebooks or create_notebook"`
}

type NotebookInfo struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Theme      string    `json:"theme"`
	Paper      string    `json:"paper"`
	Version    int       `json:"version"`
	PageCount  int       `json:"page_count"`
	BlockCount int       `json:"block_count"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (s *Server) info(ctx context.Context, nb store.Notebook) (NotebookInfo, error) {
	bs, err := s.st.Blocks(nb.ID)
	if err != nil {
		return NotebookInfo{}, err
	}
	info := NotebookInfo{ID: nb.ID, Title: nb.Title, Theme: nb.Theme, Paper: nb.Paper, Version: nb.Version, BlockCount: len(bs), UpdatedAt: nb.UpdatedAt}
	if r, err := s.lb.Build(ctx, nb, bs); err == nil {
		info.PageCount = r.PageCount
	}
	return info, nil
}

type createIn struct {
	Title string `json:"title" jsonschema:"Notebook title, shown in the page header"`
	Theme string `json:"theme,omitempty" jsonschema:"Visual theme; default clean. See list_themes"`
	Paper string `json:"paper,omitempty" jsonschema:"Paper background: plain, ruled, dotted or grid. Defaults to the theme's paper"`
}

type appendIn struct {
	NotebookID      string            `json:"notebook_id" jsonschema:"ID of the notebook"`
	Blocks          []json.RawMessage `json:"blocks" jsonschema:"Blocks to add at the end, in order (max 50 per call)"`
	ExpectedVersion *int              `json:"expected_version,omitempty" jsonschema:"If set, the write is rejected unless the notebook is at this version (optimistic concurrency)"`
}

type OutlineEntry struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Page    int    `json:"page"`
	Summary string `json:"summary"`
}

type Outline struct {
	NotebookID string         `json:"notebook_id"`
	Title      string         `json:"title"`
	Version    int            `json:"version"`
	Theme      string         `json:"theme"`
	PageCount  int            `json:"page_count"`
	Blocks     []OutlineEntry `json:"blocks"`
}

type renderIn struct {
	NotebookID string `json:"notebook_id" jsonschema:"ID of the notebook"`
	Pages      []int  `json:"pages" jsonschema:"1-indexed page numbers to render, max 4 per call"`
	PPI        int    `json:"ppi,omitempty" jsonschema:"Resolution in pixels per inch; default 80 (enough to judge layout), max 150"`
}

type exportIn struct {
	NotebookID string `json:"notebook_id" jsonschema:"ID of the notebook"`
	Path       string `json:"path,omitempty" jsonschema:"Absolute output path ending in .pdf; default ~/.folio/exports/<slug>-<date>.pdf"`
}

type ExportOut struct {
	Path string `json:"path"`
}

func (s *Server) register(m *mcp.Server) {
	addTool(m, "list_notebooks", "List all notebooks, most recently updated first.",
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, map[string][]NotebookInfo, error) {
			nbs, err := s.st.Notebooks()
			if err != nil {
				return nil, nil, err
			}
			out := []NotebookInfo{}
			for _, nb := range nbs {
				info, err := s.info(ctx, nb)
				if err != nil {
					return nil, nil, err
				}
				out = append(out, info)
			}
			return nil, map[string][]NotebookInfo{"notebooks": out}, nil
		})

	addTool(m, "create_notebook", "Create a new empty notebook. Returns its ID.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in createIn) (*mcp.CallToolResult, NotebookInfo, error) {
			if strings.TrimSpace(in.Title) == "" {
				return nil, NotebookInfo{}, fmt.Errorf("title must not be empty")
			}
			if in.Theme == "" {
				in.Theme = "clean"
			}
			if err := validTheme(in.Theme); err != nil {
				return nil, NotebookInfo{}, err
			}
			if in.Paper == "" {
				in.Paper = defaultPaper(in.Theme)
			}
			if err := validPaper(in.Paper); err != nil {
				return nil, NotebookInfo{}, err
			}
			nb, err := s.st.CreateNotebook(in.Title, in.Theme, in.Paper)
			if err != nil {
				return nil, NotebookInfo{}, err
			}
			info, err := s.info(ctx, nb)
			return nil, info, err
		})

	addTool(m, "append_blocks", "Add blocks at the end of a notebook. All blocks are validated first; if any is invalid nothing is written. Returns the new block IDs in order, where they landed, and layout warnings.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in appendIn) (*mcp.CallToolResult, MutationResult, error) {
			add, err := parseBlocks(in.Blocks)
			if err != nil {
				return nil, MutationResult{}, err
			}
			res, err := s.mutate(ctx, in.NotebookID, in.ExpectedVersion, "append_blocks", fmt.Sprintf("appended %d block(s)", len(add)),
				func(nb *store.Notebook, bs []model.Block) ([]model.Block, []string, error) {
					ids := newIDs(add)
					return model.InsertAt(bs, len(bs), add), ids, nil
				})
			return nil, res, err
		})

	addTool(m, "get_outline", "Compact view of a notebook: every block's ID, type, page and a short summary. The main way to read current state.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in notebookIn) (*mcp.CallToolResult, Outline, error) {
			nb, bs, err := s.load(in.NotebookID)
			if err != nil {
				return nil, Outline{}, err
			}
			r, err := s.lb.Build(ctx, nb, bs)
			if err != nil {
				return nil, Outline{}, err
			}
			out := Outline{NotebookID: nb.ID, Title: nb.Title, Version: nb.Version, Theme: nb.Theme, PageCount: r.PageCount, Blocks: []OutlineEntry{}}
			for _, b := range bs {
				out.Blocks = append(out.Blocks, OutlineEntry{ID: b.ID, Type: b.Type, Page: r.Page(b.ID), Summary: b.Summary()})
			}
			return nil, out, nil
		})

	addTool(m, "render_pages", "Render pages to PNG images so you can see how they look. Use after writing to check layout; fix what looks wrong.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in renderIn) (*mcp.CallToolResult, any, error) {
			if len(in.Pages) == 0 || len(in.Pages) > 4 {
				return nil, nil, fmt.Errorf("pages must list 1 to 4 page numbers, got %d", len(in.Pages))
			}
			if in.PPI == 0 {
				in.PPI = 80
			}
			if in.PPI < 30 || in.PPI > 150 {
				return nil, nil, fmt.Errorf("ppi must be between 30 and 150, got %d", in.PPI)
			}
			nb, bs, err := s.load(in.NotebookID)
			if err != nil {
				return nil, nil, err
			}
			pngs, err := s.lb.RenderPNG(ctx, nb, bs, in.Pages, in.PPI)
			if err != nil {
				return nil, nil, err
			}
			res := &mcp.CallToolResult{}
			for i, png := range pngs {
				res.Content = append(res.Content, &mcp.TextContent{Text: fmt.Sprintf("Page %d (version %d):", in.Pages[i], nb.Version)}, &mcp.ImageContent{Data: png, MIMEType: "image/png"})
			}
			return res, nil, nil
		})

	addTool(m, "export_pdf", "Export a notebook as a PDF file. Returns the absolute path.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in exportIn) (*mcp.CallToolResult, ExportOut, error) {
			nb, bs, err := s.load(in.NotebookID)
			if err != nil {
				return nil, ExportOut{}, err
			}
			path, err := s.Export(ctx, nb, bs, in.Path)
			return nil, ExportOut{path}, err
		})

	addTool(m, "get_authoring_guide", "How to write good Folio notes: every block type with an example, and style guidance. Read it once before writing.",
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: guide}}}, nil, nil
		})
}

func defaultPaper(theme string) string {
	if theme == "notebook" {
		return "ruled"
	}
	if theme == "dotgrid" {
		return "dotted"
	}
	return "plain"
}

func parseBlocks(raws []json.RawMessage) ([]model.Block, error) {
	if len(raws) == 0 {
		return nil, fmt.Errorf("blocks must contain at least one block")
	}
	if len(raws) > 50 {
		return nil, fmt.Errorf("blocks has %d entries, max 50 per call; split into several calls", len(raws))
	}
	return model.ParseAll(raws)
}

func (s *Server) load(id string) (store.Notebook, []model.Block, error) {
	nb, err := s.st.Notebook(id)
	if err != nil {
		return nb, nil, err
	}
	bs, err := s.st.Blocks(id)
	return nb, bs, err
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func (s *Server) Export(ctx context.Context, nb store.Notebook, bs []model.Block, path string) (string, error) {
	if path == "" {
		slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(nb.Title), "-"), "-")
		if slug == "" {
			slug = nb.ID
		}
		path = filepath.Join(s.home, "exports", fmt.Sprintf("%s-%s.pdf", slug, time.Now().Format("2006-01-02")))
	}
	if !filepath.IsAbs(path) || !strings.HasSuffix(strings.ToLower(path), ".pdf") {
		return "", fmt.Errorf("path must be absolute and end in .pdf, got %q", path)
	}
	r, err := s.lb.Build(ctx, nb, bs)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(r.PDF)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

func ExportByID(ctx context.Context, st *store.Store, lb *layout.Builder, home, id, path string) (string, error) {
	s := &Server{st: st, lb: lb, home: home}
	nb, bs, err := s.load(id)
	if err != nil {
		return "", friendly(err)
	}
	return s.Export(ctx, nb, bs, path)
}
