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

	"github.com/raaj6395/folio/internal/diagram"
	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
	"github.com/raaj6395/folio/internal/viewer"
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
	Mode       string    `json:"mode"`
	Density    string    `json:"density"`
	Version    int       `json:"version"`
	PageCount  int       `json:"page_count"`
	BlockCount int       `json:"block_count"`
	UpdatedAt  time.Time `json:"updated_at"`
	Folder     string    `json:"folder,omitempty"`
}

func (s *Server) info(ctx context.Context, nb store.Notebook) (NotebookInfo, error) {
	return s.infoIn(ctx, nb, nil)
}

// folderKey normalises a folder path for matching: "semester 1 / Networks" → "semester 1/networks".
func folderKey(path string) string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(path), "/"), "/")
	for i, p := range parts {
		parts[i] = strings.Join(strings.Fields(p), " ")
	}
	return strings.ToLower(strings.Join(parts, "/"))
}

// folderPaths maps the user's folder IDs to paths like "Semester 1/Networks".
func (s *Server) folderPaths(ctx context.Context) (map[string]string, error) {
	fs, err := s.st.Folders(userOf(ctx))
	paths := map[string]string{}
	for _, f := range fs {
		paths[f.ID] = f.Path
	}
	return paths, err
}

// infoIn takes the folder paths when the caller already has them (nil: looked up if needed).
func (s *Server) infoIn(ctx context.Context, nb store.Notebook, paths map[string]string) (NotebookInfo, error) {
	bs, err := s.st.Blocks(nb.UserID, nb.ID)
	if err != nil {
		return NotebookInfo{}, err
	}
	if paths == nil && nb.FolderID != "" {
		if paths, err = s.folderPaths(ctx); err != nil {
			return NotebookInfo{}, err
		}
	}
	info := NotebookInfo{ID: nb.ID, Title: nb.Title, Theme: nb.Theme, Paper: nb.Paper, Mode: nb.Mode, Density: nb.Density, Version: nb.Version, BlockCount: len(bs), UpdatedAt: nb.UpdatedAt, Folder: paths[nb.FolderID]}
	if r := s.lb.Cached(nb); r != nil {
		info.PageCount = r.PageCount
	}
	return info, nil
}

type createIn struct {
	Title   string `json:"title" jsonschema:"Notebook title, shown in the page header"`
	Theme   string `json:"theme,omitempty" jsonschema:"Visual theme; default clean. See list_themes"`
	Paper   string `json:"paper,omitempty" jsonschema:"Paper background: plain, ruled, dotted or grid. Defaults to the theme's paper"`
	Mode    string `json:"mode,omitempty" jsonschema:"dark (default) or light. Use light only when the user asks for a light/white look or wants to print"`
	Density string `json:"density,omitempty" jsonschema:"Spacing: compact, comfortable (default) or airy. The renderer adds gaps before headings and around boxes; no spacers needed"`
	Folder  string `json:"folder,omitempty" jsonschema:"Folder path to file the notebook in, e.g. Semester 1/Networks (at most 3 levels). Missing folders are created; matching ignores case. Omit for the top level"`
}

type listIn struct {
	Folder string `json:"folder,omitempty" jsonschema:"Only notebooks in this folder path and its sub-folders"`
}

type moveNotebookIn struct {
	NotebookID string `json:"notebook_id" jsonschema:"ID of the notebook to move"`
	Folder     string `json:"folder" jsonschema:"Folder path, e.g. Semester 1/Networks; missing folders are created. Empty string = top level"`
}

type FolderInfo struct {
	Path          string `json:"path"`
	NotebookCount int    `json:"notebook_count"`
}

type appendIn struct {
	NotebookID      string            `json:"notebook_id" jsonschema:"ID of the notebook"`
	Blocks          []json.RawMessage `json:"blocks" jsonschema:"Blocks to add at the end, in order (max 50 per call)"`
	ExpectedVersion *int              `json:"expected_version,omitempty" jsonschema:"If set, the write is rejected unless the notebook is at this version (optimistic concurrency)"`
}

type OutlineEntry struct {
	ID      string `json:"id"`
	Parent  string `json:"parent,omitempty"`
	Type    string `json:"type"`
	Page    int    `json:"page"`
	Summary string `json:"summary"`
}

type Outline struct {
	NotebookID string         `json:"notebook_id"`
	Title      string         `json:"title"`
	Version    int            `json:"version"`
	Theme      string         `json:"theme"`
	Mode       string         `json:"mode"`
	Density    string         `json:"density"`
	PageCount  int            `json:"page_count"`
	Guidelines string         `json:"notebook_guidelines,omitempty"`
	Blocks     []OutlineEntry `json:"blocks"`
}

type guidelinesIn struct {
	NotebookID      string `json:"notebook_id" jsonschema:"ID of the notebook"`
	Text            string `json:"text" jsonschema:"The user's own layout and style preferences for this notebook, max 2000 characters. Empty clears them"`
	ExpectedVersion *int   `json:"expected_version,omitempty" jsonschema:"Reject unless the notebook is at this version"`
}

type GuidelinesOut struct {
	NotebookID string `json:"notebook_id"`
	Version    int    `json:"version"`
	Guidelines string `json:"notebook_guidelines"`
}

type checkIn struct {
	NotebookID string `json:"notebook_id" jsonschema:"ID of the notebook"`
	Pages      []int  `json:"pages,omitempty" jsonschema:"1-indexed pages to check; default all"`
}

type CheckOut struct {
	Version int                 `json:"version"`
	Density string              `json:"density"`
	Score   int                 `json:"score"`
	Pages   []layout.PageReport `json:"pages"`
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

type LayoutInfo struct {
	Version   int          `json:"version"`
	PageCount int          `json:"page_count"`
	PageFill  []float64    `json:"page_fill_pct"`
	Blocks    []layout.Pos `json:"blocks"`
	Warnings  []string     `json:"warnings"`
}

type previewIn struct {
	Source string `json:"source" jsonschema:"D2 diagram source"`
	Theme  string `json:"theme,omitempty" jsonschema:"Theme whose diagram style to use; default clean"`
	Size   string `json:"size,omitempty" jsonschema:"full, half or small (default full); affects the reported on-page size"`
	Mode   string `json:"mode,omitempty" jsonschema:"dark (default) or light, to match the notebook"`
}

type ThemesOut struct {
	Themes []layout.Theme `json:"themes"`
	Papers []string       `json:"papers"`
	Modes  []string       `json:"modes"`
}

type setThemeIn struct {
	NotebookID      string `json:"notebook_id" jsonschema:"ID of the notebook"`
	Theme           string `json:"theme" jsonschema:"Theme name from list_themes"`
	Paper           string `json:"paper,omitempty" jsonschema:"plain, ruled, dotted or grid; default: the theme's paper"`
	Mode            string `json:"mode,omitempty" jsonschema:"dark or light; omit to keep the current mode"`
	Density         string `json:"density,omitempty" jsonschema:"compact, comfortable or airy; omit to keep the current spacing"`
	ExpectedVersion *int   `json:"expected_version,omitempty" jsonschema:"Reject unless the notebook is at this version"`
}

type ViewerOut struct {
	URL     string `json:"url"`
	Running bool   `json:"running"`
	Hint    string `json:"hint,omitempty"`
}

type renameIn struct {
	NotebookID      string `json:"notebook_id" jsonschema:"ID of the notebook"`
	Title           string `json:"title" jsonschema:"New title"`
	ExpectedVersion *int   `json:"expected_version,omitempty" jsonschema:"Reject unless the notebook is at this version"`
}

type deleteNotebookIn struct {
	NotebookID string `json:"notebook_id" jsonschema:"ID of the notebook to delete"`
	Confirm    bool   `json:"confirm,omitempty" jsonschema:"Must be true; confirms the user asked for the deletion"`
}

type ExportOut struct {
	Path string `json:"path,omitempty"`
	URL  string `json:"url,omitempty"`
}

func (s *Server) register(m *mcp.Server) {
	addTool(s, m, "list_notebooks", "List notebooks, most recently updated first, with the folder each is in. Pass folder to list only that folder and its sub-folders.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, map[string][]NotebookInfo, error) {
			nbs, err := s.st.Notebooks(userOf(ctx))
			if err != nil {
				return nil, nil, err
			}
			paths, err := s.folderPaths(ctx)
			if err != nil {
				return nil, nil, err
			}
			want := folderKey(in.Folder)
			out := []NotebookInfo{}
			for _, nb := range nbs {
				if p := folderKey(paths[nb.FolderID]); want != "" && p != want && !strings.HasPrefix(p, want+"/") {
					continue
				}
				info, err := s.infoIn(ctx, nb, paths)
				if err != nil {
					return nil, nil, err
				}
				out = append(out, info)
			}
			return nil, map[string][]NotebookInfo{"notebooks": out}, nil
		})

	addTool(s, m, "list_folders", "List the user's folders (paths like Semester 1/Networks) with how many notebooks each holds, sub-folders included. Check it before filing a notebook so you reuse an existing folder.",
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, map[string][]FolderInfo, error) {
			fs, err := s.st.Folders(userOf(ctx))
			if err != nil {
				return nil, nil, err
			}
			out := []FolderInfo{}
			for _, f := range fs {
				out = append(out, FolderInfo{Path: f.Path, NotebookCount: f.Count})
			}
			return nil, map[string][]FolderInfo{"folders": out}, nil
		})

	addTool(s, m, "move_notebook", "Move a notebook into a folder (by path; missing folders are created) or to the top level with folder \"\". Doesn't change the notebook's content or version.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in moveNotebookIn) (*mcp.CallToolResult, NotebookInfo, error) {
			nb, err := s.st.Notebook(userOf(ctx), in.NotebookID)
			if err != nil {
				return nil, NotebookInfo{}, err
			}
			f, err := s.st.EnsureFolderPath(userOf(ctx), in.Folder)
			if err != nil {
				return nil, NotebookInfo{}, err
			}
			if err := s.st.MoveNotebook(userOf(ctx), nb.ID, f.ID); err != nil {
				return nil, NotebookInfo{}, err
			}
			nb.FolderID = f.ID
			info, err := s.info(ctx, nb)
			return nil, info, err
		})

	addTool(s, m, "create_notebook", "Create a new empty notebook, optionally in a folder. Returns its ID.",
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
			if in.Mode == "" {
				in.Mode = "dark"
			}
			if err := validMode(in.Mode); err != nil {
				return nil, NotebookInfo{}, err
			}
			if in.Density == "" {
				in.Density = store.DefaultDensity
			}
			if err := validDensity(in.Density); err != nil {
				return nil, NotebookInfo{}, err
			}
			folder, err := s.st.EnsureFolderPath(userOf(ctx), in.Folder)
			if err != nil {
				return nil, NotebookInfo{}, err
			}
			nb, err := s.st.CreateNotebook(userOf(ctx), in.Title, in.Theme, in.Paper, in.Mode, in.Density, folder.ID)
			if err != nil {
				return nil, NotebookInfo{}, err
			}
			info, err := s.info(ctx, nb)
			return nil, info, err
		})

	addTool(s, m, "append_blocks", "Add blocks at the end of a notebook. All blocks are validated first; if any is invalid nothing is written. Returns the new block IDs in order, where they landed, and layout warnings.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in appendIn) (*mcp.CallToolResult, MutationResult, error) {
			add, err := parseBlocks(ctx, in.Blocks)
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

	addTool(s, m, "get_outline", "Compact view of a notebook: every block's ID, type, page and a short summary. The main way to read current state.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in notebookIn) (*mcp.CallToolResult, Outline, error) {
			nb, bs, err := s.load(ctx, in.NotebookID)
			if err != nil {
				return nil, Outline{}, err
			}
			r, err := s.build(ctx, nb, bs)
			if err != nil {
				return nil, Outline{}, err
			}
			out := Outline{NotebookID: nb.ID, Title: nb.Title, Version: nb.Version, Theme: nb.Theme, Mode: nb.Mode, Density: nb.Density, PageCount: r.PageCount, Guidelines: nb.Guidelines, Blocks: []OutlineEntry{}}
			for _, b := range bs {
				out.Blocks = append(out.Blocks, OutlineEntry{ID: b.ID, Type: b.Type, Page: r.Page(b.ID), Summary: b.Summary()})
				for _, k := range b.Children() {
					out.Blocks = append(out.Blocks, OutlineEntry{ID: k.ID, Parent: b.ID, Type: k.Type, Page: r.Page(k.ID), Summary: k.Summary()})
				}
			}
			return nil, out, nil
		})

	addTool(s, m, "get_layout_info", "Where every block landed (page, y position and end position in mm from the page top), how full each page is, and layout warnings. Cheaper than rendering images.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in notebookIn) (*mcp.CallToolResult, LayoutInfo, error) {
			nb, bs, err := s.load(ctx, in.NotebookID)
			if err != nil {
				return nil, LayoutInfo{}, err
			}
			r, err := s.build(ctx, nb, bs)
			if err != nil {
				return nil, LayoutInfo{}, err
			}
			return nil, LayoutInfo{Version: nb.Version, PageCount: r.PageCount, PageFill: r.PageFill, Blocks: r.Blocks, Warnings: r.Warnings}, nil
		})

	addTool(s, m, "render_pages", "Render pages to PNG images so you can see how they look. Use after writing to check layout; fix what looks wrong.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in renderIn) (*mcp.CallToolResult, any, error) {
			if err := s.allowRender(ctx); err != nil {
				return nil, nil, err
			}
			if len(in.Pages) == 0 || len(in.Pages) > 4 {
				return nil, nil, fmt.Errorf("pages must list 1 to 4 page numbers, got %d", len(in.Pages))
			}
			if in.PPI == 0 {
				in.PPI = 80
			}
			if in.PPI < 30 || in.PPI > 150 {
				return nil, nil, fmt.Errorf("ppi must be between 30 and 150, got %d", in.PPI)
			}
			nb, bs, err := s.load(ctx, in.NotebookID)
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

	addTool(s, m, "preview_diagram", "Render D2 source to an image without changing any notebook. Use it to check a diagram before inserting it. Returns the D2 error (with line number) if it does not compile.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in previewIn) (*mcp.CallToolResult, any, error) {
			if err := s.allowRender(ctx); err != nil {
				return nil, nil, err
			}
			if in.Theme == "" {
				in.Theme = "clean"
			}
			th, err := layout.ThemeByName(in.Theme)
			if err != nil {
				return nil, nil, err
			}
			if in.Size == "" {
				in.Size = "full"
			}
			if in.Size != "full" && in.Size != "half" && in.Size != "small" {
				return nil, nil, fmt.Errorf(`size must be "full", "half" or "small", got %q`, in.Size)
			}
			if in.Mode == "" {
				in.Mode = "dark"
			}
			if err := validMode(in.Mode); err != nil {
				return nil, nil, err
			}
			png, w, h, pt, err := s.lb.PreviewPNG(ctx, in.Source, th, in.Mode, in.Size)
			if err != nil {
				return nil, nil, err
			}
			note := fmt.Sprintf("At size %q this diagram renders %.0f×%.0f mm on the page, labels ~%.1fpt.", in.Size, w, h, pt)
			if pt < 6 {
				note += " That is too small to read: simplify it, split it, or change its direction."
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: note}, &mcp.ImageContent{Data: png, MIMEType: "image/png"}}}, nil, nil
		})

	addTool(s, m, "export_pdf", "Export a notebook as a PDF. Over HTTP it returns a download URL; locally it writes a file and returns its path.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in exportIn) (*mcp.CallToolResult, ExportOut, error) {
			if err := s.allowRender(ctx); err != nil {
				return nil, ExportOut{}, err
			}
			nb, bs, err := s.load(ctx, in.NotebookID)
			if err != nil {
				return nil, ExportOut{}, err
			}
			if s.publicURL != "" {
				if _, err := s.build(ctx, nb, bs); err != nil {
					return nil, ExportOut{}, err
				}
				return nil, ExportOut{URL: s.publicURL + "/n/" + nb.ID + "/pdf"}, nil
			}
			path, err := s.Export(ctx, nb, bs, in.Path)
			return nil, ExportOut{Path: path}, err
		})

	addTool(s, m, "rename_notebook", "Change a notebook's title.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in renameIn) (*mcp.CallToolResult, NotebookInfo, error) {
			title := strings.TrimSpace(in.Title)
			if title == "" || len(title) > 200 {
				return nil, NotebookInfo{}, fmt.Errorf("title must be 1-200 characters")
			}
			var nb store.Notebook
			_, err := s.mutate(ctx, in.NotebookID, in.ExpectedVersion, "rename_notebook", "renamed to "+title,
				func(n *store.Notebook, bs []model.Block) ([]model.Block, []string, error) {
					n.Title = title
					nb = *n
					return bs, nil, nil
				})
			if err != nil {
				return nil, NotebookInfo{}, err
			}
			nb, err = s.st.Notebook(userOf(ctx), in.NotebookID)
			if err != nil {
				return nil, NotebookInfo{}, err
			}
			info, err := s.info(ctx, nb)
			return nil, info, err
		})

	addTool(s, m, "delete_notebook", "Delete a notebook. Requires confirm: true. Only do this when the user clearly asked to delete it.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in deleteNotebookIn) (*mcp.CallToolResult, map[string]string, error) {
			if !in.Confirm {
				return nil, nil, fmt.Errorf("set confirm: true to delete notebook %s; ask the user first", in.NotebookID)
			}
			if err := s.st.Delete(userOf(ctx), in.NotebookID); err != nil {
				return nil, nil, err
			}
			return nil, map[string]string{"deleted": in.NotebookID}, nil
		})

	addTool(s, m, "list_themes", "List available themes and paper backgrounds.",
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, ThemesOut, error) {
			return nil, ThemesOut{Themes: layout.Themes, Papers: papers, Modes: layout.Modes}, nil
		})

	addTool(s, m, "set_theme", "Change a notebook's theme, paper, mode (dark or light) and/or density (spacing). If paper is omitted, the theme's default paper is used; mode and density stay as they are unless given. To only switch dark/light or density, pass the current theme too.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in setThemeIn) (*mcp.CallToolResult, MutationResult, error) {
			if err := validTheme(in.Theme); err != nil {
				return nil, MutationResult{}, err
			}
			if in.Paper == "" {
				in.Paper = defaultPaper(in.Theme)
			}
			if err := validPaper(in.Paper); err != nil {
				return nil, MutationResult{}, err
			}
			if in.Mode != "" {
				if err := validMode(in.Mode); err != nil {
					return nil, MutationResult{}, err
				}
			}
			if in.Density != "" {
				if err := validDensity(in.Density); err != nil {
					return nil, MutationResult{}, err
				}
			}
			res, err := s.mutate(ctx, in.NotebookID, in.ExpectedVersion, "set_theme", fmt.Sprintf("theme %s, paper %s, mode %s, density %s", in.Theme, in.Paper, in.Mode, in.Density),
				func(nb *store.Notebook, bs []model.Block) ([]model.Block, []string, error) {
					nb.Theme, nb.Paper = in.Theme, in.Paper
					if in.Mode != "" {
						nb.Mode = in.Mode
					}
					if in.Density != "" {
						nb.Density = in.Density
					}
					return bs, nil, nil
				})
			return nil, res, err
		})

	addTool(s, m, "get_viewer_url", "URL of the live web viewer for a notebook, where the user can watch pages update as you write. Also says whether the viewer is running (start it with `folio serve`).",
		func(ctx context.Context, _ *mcp.CallToolRequest, in notebookIn) (*mcp.CallToolResult, ViewerOut, error) {
			if _, err := s.st.Notebook(userOf(ctx), in.NotebookID); err != nil {
				return nil, ViewerOut{}, err
			}
			if s.publicURL != "" {
				return nil, ViewerOut{URL: s.publicURL + "/n/" + in.NotebookID, Running: true}, nil
			}
			out := ViewerOut{URL: fmt.Sprintf("http://localhost:%d/n/%s", viewer.DefaultPort, in.NotebookID), Running: viewer.Running(viewer.DefaultPort)}
			if !out.Running {
				out.Hint = "The viewer is not running. Ask the user to run `folio serve` in a terminal, then open the URL."
			}
			return nil, out, nil
		})

	addTool(s, m, "check_layout", "Score a notebook's layout (0-100) and list every layout issue per page with block IDs and a concrete fix. Call it before saying you are done.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in checkIn) (*mcp.CallToolResult, CheckOut, error) {
			nb, bs, err := s.load(ctx, in.NotebookID)
			if err != nil {
				return nil, CheckOut{}, err
			}
			r, err := s.build(ctx, nb, bs)
			if err != nil {
				return nil, CheckOut{}, err
			}
			score, pages := r.Report()
			if len(in.Pages) > 0 {
				var sel []layout.PageReport
				total := 0
				for _, p := range in.Pages {
					if p < 1 || p > r.PageCount {
						return nil, CheckOut{}, fmt.Errorf("page %d does not exist; the notebook has %d page(s)", p, r.PageCount)
					}
					sel = append(sel, pages[p-1])
					total += pages[p-1].Score
				}
				pages, score = sel, (total+len(sel)/2)/len(sel)
			}
			return nil, CheckOut{Version: nb.Version, Density: nb.Density, Score: score, Pages: pages}, nil
		})

	addTool(s, m, "set_notebook_guidelines", "Save the user's own layout/style preferences on a notebook (max 2000 characters). Every agent then sees them in get_outline and in its first write. Versioned like any change.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in guidelinesIn) (*mcp.CallToolResult, GuidelinesOut, error) {
			text := strings.TrimSpace(in.Text)
			if n := len([]rune(text)); n > store.MaxGuidelines {
				return nil, GuidelinesOut{}, fmt.Errorf("guidelines are %d characters, max %d", n, store.MaxGuidelines)
			}
			nb, _, err := s.st.Mutate(userOf(ctx), in.NotebookID, in.ExpectedVersion, "set_notebook_guidelines", "updated notebook guidelines",
				func(nb *store.Notebook, bs []model.Block) ([]model.Block, error) {
					nb.Guidelines = text
					return bs, nil
				})
			if err != nil {
				return nil, GuidelinesOut{}, err
			}
			return nil, GuidelinesOut{NotebookID: nb.ID, Version: nb.Version, Guidelines: nb.Guidelines}, nil
		})

	addTool(s, m, "get_notebook_guidelines", "Read the user's saved layout/style preferences for a notebook.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in notebookIn) (*mcp.CallToolResult, GuidelinesOut, error) {
			nb, err := s.st.Notebook(userOf(ctx), in.NotebookID)
			if err != nil {
				return nil, GuidelinesOut{}, err
			}
			return nil, GuidelinesOut{NotebookID: nb.ID, Version: nb.Version, Guidelines: nb.Guidelines}, nil
		})

	addTool(s, m, "get_authoring_guide", "How to write good Folio notes: every block type with an example, and style guidance. Read it once before writing.",
		func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: guide}}}, nil, nil
		})
}

func defaultPaper(theme string) string {
	th, _ := layout.ThemeByName(theme)
	return th.Paper
}

func checkDiagram(ctx context.Context, b model.Block) error {
	for _, src := range b.DiagramSources() {
		if _, err := diagram.Render(ctx, src, diagram.Style{}); err != nil {
			return err
		}
	}
	return nil
}

func parseBlocks(ctx context.Context, raws []json.RawMessage) ([]model.Block, error) {
	if len(raws) == 0 {
		return nil, fmt.Errorf("blocks must contain at least one block")
	}
	if len(raws) > 50 {
		return nil, fmt.Errorf("blocks has %d entries, max 50 per call; split into several calls", len(raws))
	}
	bs, err := model.ParseAll(raws)
	if err != nil {
		return nil, err
	}
	for i, b := range bs {
		if err := checkDiagram(ctx, b); err != nil {
			return nil, fmt.Errorf("block[%d] (%s): %v", i, b.Type, err)
		}
	}
	return bs, nil
}

func (s *Server) load(ctx context.Context, id string) (store.Notebook, []model.Block, error) {
	nb, err := s.st.Notebook(userOf(ctx), id)
	if err != nil {
		return nb, nil, err
	}
	bs, err := s.st.Blocks(nb.UserID, id)
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
	pdf, err := s.lb.PDF(ctx, nb, bs)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(pdf)
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
	nb, err := st.NotebookAdmin(id)
	if err != nil {
		return "", friendly(err)
	}
	bs, err := st.Blocks(nb.UserID, id)
	if err != nil {
		return "", friendly(err)
	}
	return s.Export(ctx, nb, bs, path)
}
