package layout

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

func canvasNB(id string) store.Notebook {
	return store.Notebook{ID: id, Title: "Canvas", Theme: "clean", Paper: "dotted", Mode: "light", Density: "comfortable", Format: "canvas", Version: 1}
}

// A 14-step chain drawn left to right is far wider than the A4 column.
const wideChain = `{"type":"diagram","source":"direction: right\na -> b -> c -> d -> e -> f -> g -> h -> i -> j -> k -> l -> m -> n"}`

func TestCanvasDiagramKeepsNaturalSize(t *testing.T) {
	b := builder(t)
	bs := blocks(t, `{"type":"heading","level":1,"text":"Flow"}`, wideChain)
	r, err := b.Build(context.Background(), canvasNB("n_canvas_wide"), bs)
	if err != nil {
		t.Fatal(err)
	}
	if r.PageCount != 1 || r.CanvasWMM <= textWidthMM+2*CanvasMarginMM+50 {
		t.Fatalf("a wide diagram must widen the canvas past the A4 column: pages %d, width %.1f", r.PageCount, r.CanvasWMM)
	}
	var doc struct{ Blocks []map[string]any }
	data, _ := os.ReadFile(b.Dir("n_canvas_wide") + "/doc.json")
	json.Unmarshal(data, &doc)
	if w := doc.Blocks[1]["width_mm"].(float64); w <= textWidthMM || r.CanvasWMM != round1(w+2*CanvasMarginMM) {
		t.Fatalf("diagram width %.1f must be uncapped and set the canvas width %.1f", w, r.CanvasWMM)
	}
	if len(r.Issues) != 0 {
		t.Fatalf("unexpected issues: %v", r.Warnings)
	}
	// The same notebook on A4 shrinks the diagram into the column.
	a4 := canvasNB("n_canvas_wide_a4")
	a4.Format = "a4"
	if r, err := b.Build(context.Background(), a4, bs); err != nil || r.CanvasWMM != 0 {
		t.Fatalf("a4: %v %+v", err, r)
	}
}

func TestCanvasTextWrapsInColumn(t *testing.T) {
	b := builder(t)
	bs := blocks(t, `{"type":"paragraph","text":"`+strings.Repeat("word ", 300)+`"}`,
		`{"type":"two_column","left":[{"type":"paragraph","text":"left side"}],"right":[{"type":"paragraph","text":"right side"}]}`)
	r, err := b.Build(context.Background(), canvasNB("n_canvas_text"), bs)
	if err != nil {
		t.Fatal(err)
	}
	if r.CanvasWMM != textWidthMM+2*CanvasMarginMM || r.Height(bs[0].ID) < 30 {
		t.Fatalf("text must wrap in the 174mm column: width %.1f, paragraph height %.1f", r.CanvasWMM, r.Height(bs[0].ID))
	}
	kids := bs[1].Children()
	var lx, rx float64
	for _, p := range r.Nested {
		if p.ID == kids[0].ID {
			lx = p.XMM
		}
		if p.ID == kids[1].ID {
			rx = p.XMM
		}
	}
	if rx < lx+50 {
		t.Fatalf("two_column collapsed on a canvas: left x %.1f, right x %.1f", lx, rx)
	}
}

func TestCanvasIgnoresPageBreak(t *testing.T) {
	b := builder(t)
	bs := blocks(t, `{"type":"paragraph","text":"one"}`, `{"type":"page_break"}`, `{"type":"paragraph","text":"two"}`)
	r, err := b.Build(context.Background(), canvasNB("n_canvas_pb"), bs)
	if err != nil {
		t.Fatal(err)
	}
	if r.PageCount != 1 || len(r.Issues) != 1 || r.Issues[0].Check != "page_break_ignored" || r.Issues[0].Blocks[0] != bs[1].ID {
		t.Fatalf("pages %d, issues %+v", r.PageCount, r.Issues)
	}
	if score, _ := r.Report(); score != 95 {
		t.Fatalf("score %d", score)
	}
}

func TestCanvasTooLarge(t *testing.T) {
	bs := blocks(t, `{"type":"paragraph","text":"x"}`)
	marks := []mark{{ID: bs[0].ID, K: "s", Page: 1, Y: 80}, {ID: bs[0].ID, K: "e", Page: 1, Y: 90}, {ID: "_end", Page: 1, Y: 90}}
	if r := analyzeCanvas(marks, bs, "comfortable", 4000); len(r.Issues) != 0 {
		t.Fatalf("4000mm is fine: %v", r.Warnings)
	}
	if r := analyzeCanvas(marks, bs, "comfortable", 5100); len(r.Issues) != 1 || r.Issues[0].Check != "canvas_too_large" {
		t.Fatalf("5100mm must warn: %+v", r.Issues)
	}
}

func TestCanvasKitchenSinkAndOutputs(t *testing.T) {
	data, err := os.ReadFile("../../testdata/kitchen_sink.json")
	if err != nil {
		t.Fatal(err)
	}
	var raws []json.RawMessage
	json.Unmarshal(data, &raws)
	bs, err := model.ParseAll(raws)
	if err != nil {
		t.Fatal(err)
	}
	for i := range bs {
		bs[i].ID = model.NewID("b_")
	}
	bs = append(bs, blocks(t, wideChain)...)
	b := builder(t)
	nb := canvasNB("n_canvas_ks")
	r, err := b.Build(context.Background(), nb, bs)
	if err != nil {
		t.Fatal(err)
	}
	if r.PageCount != 1 {
		t.Fatalf("every block type must fit on one canvas, got %d pages", r.PageCount)
	}
	svg, err := b.SVG(context.Background(), nb, bs)
	if err != nil {
		t.Fatal(err)
	}
	if out, _ := os.ReadFile(svg); !bytes.Contains(out, []byte("<svg")) {
		t.Fatal("SVG export is not an SVG")
	}
	if dir := os.Getenv("FOLIO_KITCHEN"); dir != "" {
		os.MkdirAll(dir, 0o755)
		out, _ := os.ReadFile(svg)
		os.WriteFile(dir+"/kitchen-sink-canvas.svg", out, 0o644)
	}
	ppi := r.CanvasPPI(150, 400)
	pngs, err := b.RenderPNG(context.Background(), nb, bs, []int{1}, ppi)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(pngs[0]))
	if err != nil || max(cfg.Width, cfg.Height) > 400 {
		t.Fatalf("canvas PNG must fit 400px, got %dx%d (%v)", cfg.Width, cfg.Height, err)
	}
	a4 := nb
	a4.Format = "a4"
	if _, err := b.SVG(context.Background(), a4, bs); err == nil {
		t.Fatal("SVG export must be canvas-only")
	}
}

func TestCanvasTiles(t *testing.T) {
	b := builder(t)
	bs := blocks(t, wideChain)
	nb := canvasNB("n_canvas_tiles")
	r, err := b.Build(context.Background(), nb, bs)
	if err != nil {
		t.Fatal(err)
	}
	cols, rows := r.TileGrid()
	if cols < 2 || rows != 1 {
		t.Fatalf("a %.0f×%.0f mm canvas: grid %d×%d", r.CanvasWMM, r.CanvasHMM, cols, rows)
	}
	pngs, err := b.RenderTiles(context.Background(), nb, bs, []int{1, cols})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := png.DecodeConfig(bytes.NewReader(pngs[0]))
	last, _ := png.DecodeConfig(bytes.NewReader(pngs[1]))
	if first.Width != 1535 || first.Height > 1024 || last.Width >= first.Width {
		t.Fatalf("tile sizes: first %dx%d, last (partial) %dx%d", first.Width, first.Height, last.Width, last.Height)
	}
	if _, err := b.RenderTiles(context.Background(), nb, bs, []int{cols + 1}); err == nil || !strings.Contains(err.Error(), "numbered row by row") {
		t.Fatalf("out of range tile: %v", err)
	}
	a4 := nb
	a4.Format = "a4"
	if _, err := b.RenderTiles(context.Background(), a4, bs, []int{1}); err == nil {
		t.Fatal("tiles must be canvas-only")
	}
}

// TestCanvasTilesMatchThePage proves tiles show the right part of the canvas: each tile must match
// the same rows of the whole canvas rendered at the tile resolution.
func TestCanvasTilesMatchThePage(t *testing.T) {
	b := builder(t)
	var raws []string
	for i := range 14 {
		raws = append(raws, fmt.Sprintf(`{"type":"heading","level":1,"text":"Section %d"}`, i), `{"type":"paragraph","text":"`+strings.Repeat("Some words to fill the line. ", 12)+`"}`)
	}
	bs := blocks(t, raws...)
	nb := canvasNB("n_canvas_tilepx")
	r, err := b.Build(context.Background(), nb, bs)
	if err != nil {
		t.Fatal(err)
	}
	if cols, rows := r.TileGrid(); cols != 1 || rows < 2 {
		t.Fatalf("want one column of several tiles, got %d×%d (%.0f×%.0f mm)", cols, rows, r.CanvasWMM, r.CanvasHMM)
	}
	full, err := b.RenderPNG(context.Background(), nb, bs, []int{1}, TilePPI)
	if err != nil {
		t.Fatal(err)
	}
	tiles, err := b.RenderTiles(context.Background(), nb, bs, []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	page, _ := png.Decode(bytes.NewReader(full[0]))
	// diff is the mean brightness difference between a tile and the page rows starting at y0.
	diff := func(tile []byte, y0 int) float64 {
		img, _ := png.Decode(bytes.NewReader(tile))
		var sum float64
		n := 0
		for y := 0; y < img.Bounds().Dy() && y0+y < page.Bounds().Dy(); y += 3 {
			for x := 0; x < img.Bounds().Dx(); x += 3 {
				a, _, _, _ := img.At(x, y).RGBA()
				p, _, _, _ := page.At(x, y0+y).RGBA()
				sum += math.Abs(float64(a>>8) - float64(p>>8))
				n++
			}
		}
		return sum / float64(n)
	}
	tileH := int(math.Round(TileHMM / 25.4 * TilePPI))
	right, wrong := diff(tiles[1], tileH), diff(tiles[1], 0)
	if d := diff(tiles[0], 0); d > 8 || right > 8 || wrong < 2*right {
		t.Fatalf("tiles must show their own part of the canvas: tile1 diff %.2f, tile2 diff %.2f (vs top %.2f)", d, right, wrong)
	}
}
