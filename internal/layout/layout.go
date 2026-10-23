package layout

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/raaj6395/folio"
	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

const (
	pageHeightMM    = 297.0
	marginTopMM     = 20.0
	contentHeightMM = pageHeightMM - 2*marginTopMM
	Timeout         = 20 * time.Second
)

type Pos struct {
	ID      string  `json:"id"`
	Type    string  `json:"type"`
	Page    int     `json:"page"`
	YMM     float64 `json:"y_mm"`
	EndPage int     `json:"end_page"`
	EndYMM  float64 `json:"end_y_mm"`
	Level   int     `json:"level,omitempty"`
	XMM     float64 `json:"-"`
}

type Result struct {
	Key          string        `json:"key"`
	DocHash      string        `json:"doc_hash"`
	PageCount    int           `json:"page_count"`
	Blocks       []Pos         `json:"blocks"`
	LastPageFill float64       `json:"last_page_fill_pct"`
	PageFill     []float64     `json:"page_fill_pct"`
	Nested       []Pos         `json:"nested,omitempty"`
	Warnings     []string      `json:"warnings"`
	Issues       []Issue       `json:"issues"`
	Elapsed      time.Duration `json:"-"`
}

func (r *Result) LastPageFreeMM() float64 {
	if r.PageCount == 0 {
		return contentHeightMM
	}
	return round1(contentHeightMM * (100 - r.LastPageFill) / 100)
}

func (r *Result) Height(id string) float64 {
	for _, p := range r.Blocks {
		if p.ID == id {
			if p.EndPage > p.Page {
				return round1(contentBottomMM - p.YMM + float64(p.EndPage-p.Page-1)*contentHeightMM + p.EndYMM - marginTopMM)
			}
			return round1(p.EndYMM - p.YMM)
		}
	}
	return 0
}

func (r *Result) Page(id string) int {
	for _, p := range append(r.Blocks, r.Nested...) {
		if p.ID == id {
			return p.Page
		}
	}
	return 0
}

type Builder struct {
	Home       string
	Root       string
	fonts      string
	mu         sync.Mutex
	runs       atomic.Int64
	rasterised atomic.Int64
}

func (b *Builder) Stats() (typstRuns, pagesRasterised int64) {
	return b.runs.Load(), b.rasterised.Load()
}

func New(home string) (*Builder, error) {
	fonts := filepath.Join(home, "fonts")
	if err := extract(folio.Fonts, "fonts", fonts); err != nil {
		return nil, fmt.Errorf("installing fonts: %w", err)
	}
	return &Builder{Home: home, Root: "build", fonts: fonts}, nil
}

func (b *Builder) FontDir() string { return b.fonts }

func extract(src fs.FS, root, dst string) error {
	return fs.WalkDir(src, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, strings.TrimPrefix(strings.TrimPrefix(p, root), "/"))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		if old, err := os.ReadFile(target); err == nil && bytes.Equal(old, data) {
			return nil
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func (b *Builder) Dir(notebookID string) string {
	return filepath.Join(b.Home, b.Root, notebookID)
}

func (b *Builder) typst(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	b.runs.Add(1)
	args = append(args, "--root", dir, "--font-path", b.fonts, "--ignore-system-fonts", "-j", "1")
	cmd := exec.CommandContext(ctx, "typst", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("typst timed out after %s", Timeout)
	}
	if err != nil {
		var ee *exec.Error
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("typst is not installed or not on PATH; run `folio doctor`")
		}
		msg := strings.TrimSpace(stderr.String())
		slog.Error("FOLIO BUG: typst failed on the fixed template", "dir", dir, "args", args, "stderr", msg)
		if len(msg) > 1500 {
			msg = msg[:1500] + "…"
		}
		return nil, fmt.Errorf("internal render error (a Folio bug, not your content; details logged): %s", msg)
	}
	return stdout.Bytes(), nil
}

var templateHash = func() string {
	h := sha256.New()
	fs.WalkDir(folio.Templates, "templates", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := fs.ReadFile(folio.Templates, p)
			h.Write(data)
		}
		return nil
	})
	h.Write([]byte(renderVersion))
	return hex.EncodeToString(h.Sum(nil))[:8]
}()

const renderVersion = "r3"

func key(nb store.Notebook) string {
	return fmt.Sprintf("v%d-%s-%s-%s-%s-t%s", nb.Version, nb.Theme, nb.Paper, nb.Mode, nb.Density, templateHash)
}

func (b *Builder) Build(ctx context.Context, nb store.Notebook, blocks []model.Block) (*Result, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.build(ctx, nb, blocks)
}

func (b *Builder) build(ctx context.Context, nb store.Notebook, blocks []model.Block) (*Result, error) {
	dir := b.Dir(nb.ID)
	cachePath := filepath.Join(dir, "layout.json")
	if data, err := os.ReadFile(cachePath); err == nil {
		var r Result
		if json.Unmarshal(data, &r) == nil && r.Key == key(nb) {
			return &r, nil
		}
	}
	start := time.Now()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := extract(folio.Templates, "templates", dir); err != nil {
		return nil, err
	}
	th, err := ThemeByName(nb.Theme)
	if err != nil {
		return nil, err
	}
	docs, dwarns, err := b.docBlocks(ctx, dir, th.D2For(nb.Mode), blocks)
	if err != nil {
		return nil, err
	}
	doc := map[string]any{"title": nb.Title, "theme": nb.Theme, "paper": nb.Paper, "mode": nb.Mode, "density": nb.Density, "blocks": docs}
	js, _ := json.Marshal(doc)
	if err := os.WriteFile(filepath.Join(dir, "doc.json"), js, 0o644); err != nil {
		return nil, err
	}
	raw, err := b.typst(ctx, dir, "eval", `query(<blockpos>).map(it => (id: it.value.id, k: it.value.k, page: it.location().page(), y: it.location().position().y / 1pt, x: it.location().position().x / 1pt))`, "--in", "main.typ")
	if err != nil {
		return nil, err
	}
	var marks []mark
	if err := json.Unmarshal(raw, &marks); err != nil {
		return nil, fmt.Errorf("parsing typst layout output: %w", err)
	}
	r := analyze(marks, blocks, nb.Density)
	for _, is := range dwarns {
		is.Page = r.Page(is.Blocks[0])
		r.Issues = append(r.Issues, is)
		r.Warnings = append(r.Warnings, is.String())
	}
	r.Key, r.DocHash = key(nb), docHash(js)
	r.Elapsed = time.Since(start)
	js, _ = json.Marshal(r)
	os.WriteFile(cachePath, js, 0o644)
	slog.Info("built notebook", "id", nb.ID, "version", nb.Version, "pages", r.PageCount, "ms", r.Elapsed.Milliseconds())
	return r, nil
}

func docHash(js []byte) string {
	sum := sha256.Sum256(js)
	return hex.EncodeToString(sum[:8])
}

func (b *Builder) current(ctx context.Context, nb store.Notebook, blocks []model.Block) (*Result, error) {
	r, err := b.build(ctx, nb, blocks)
	if err != nil {
		return nil, err
	}
	dir := b.Dir(nb.ID)
	if js, err := os.ReadFile(filepath.Join(dir, "doc.json")); err == nil && docHash(js) == r.DocHash {
		return r, nil
	}
	os.Remove(filepath.Join(dir, "layout.json"))
	return b.build(ctx, nb, blocks)
}

func (b *Builder) PDF(ctx context.Context, nb store.Notebook, blocks []model.Block) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.current(ctx, nb, blocks)
	if err != nil {
		return "", err
	}
	dir := b.Dir(nb.ID)
	name := "out-" + r.Key + ".pdf"
	if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
		return filepath.Join(dir, name), nil
	}
	if _, err := b.typst(ctx, dir, "compile", "main.typ", name); err != nil {
		return "", err
	}
	olds, _ := filepath.Glob(filepath.Join(dir, "out*.pdf"))
	for _, o := range olds {
		if filepath.Base(o) != name {
			os.Remove(o)
		}
	}
	return filepath.Join(dir, name), nil
}

var pruneAfter = 10 * time.Minute

func (b *Builder) RenderPNG(ctx context.Context, nb store.Notebook, blocks []model.Block, pages []int, ppi int) ([][]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.current(ctx, nb, blocks)
	if err != nil {
		return nil, err
	}
	for _, p := range pages {
		if p < 1 || p > r.PageCount {
			return nil, fmt.Errorf("page %d does not exist; the notebook has %d page(s) (pages are 1-indexed)", p, r.PageCount)
		}
	}
	dir := b.Dir(nb.ID)
	pdir := filepath.Join(dir, "pages")
	fps := r.Fingerprints(nb, blocks)
	file := func(p int) string { return filepath.Join(pdir, fmt.Sprintf("%s-%d.png", fps[p-1], ppi)) }
	var missing []string
	for _, p := range pages {
		if _, err := os.Stat(file(p)); err != nil && !slices.Contains(missing, strconv.Itoa(p)) {
			missing = append(missing, strconv.Itoa(p))
		}
	}
	if len(missing) > 0 {
		tmp := filepath.Join("pages", "tmp")
		os.RemoveAll(filepath.Join(dir, tmp))
		if err := os.MkdirAll(filepath.Join(dir, tmp), 0o755); err != nil {
			return nil, err
		}
		if _, err := b.typst(ctx, dir, "compile", "main.typ", filepath.Join(tmp, "{p}.png"), "--pages", strings.Join(missing, ","), "--ppi", strconv.Itoa(ppi)); err != nil {
			return nil, err
		}
		for _, p := range missing {
			n, _ := strconv.Atoi(p)
			if err := os.Rename(filepath.Join(dir, tmp, p+".png"), file(n)); err != nil {
				return nil, err
			}
		}
		b.rasterised.Add(int64(len(missing)))
		prune(pdir, fps)
	}
	out := make([][]byte, len(pages))
	for i, p := range pages {
		if out[i], err = os.ReadFile(file(p)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func prune(pdir string, current []string) {
	entries, _ := os.ReadDir(pdir)
	for _, e := range entries {
		fp, _, _ := strings.Cut(e.Name(), "-")
		if slices.Contains(current, fp) {
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) >= pruneAfter {
			os.RemoveAll(filepath.Join(pdir, e.Name()))
		}
	}
}
