package layout

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

func appendPara(t *testing.T, nb *store.Notebook, bs []model.Block, text string) []model.Block {
	t.Helper()
	nb.Version++
	return model.InsertAt(append([]model.Block(nil), bs...), len(bs), blocks(t, `{"type":"paragraph","text":"`+text+`"}`))
}

func allPages(n int) []int {
	ps := make([]int, n)
	for i := range ps {
		ps[i] = i + 1
	}
	return ps
}

func TestEditRunsOneTypstAndPDFIsLazy(t *testing.T) {
	ctx := context.Background()
	b := builder(t)
	nb, bs := perfNotebook(t, b, "n_lazy", 2)
	bs = appendPara(t, &nb, bs, "edited")
	runs0, _ := b.Stats()
	if _, err := b.Build(ctx, nb, bs); err != nil {
		t.Fatal(err)
	}
	if runs, _ := b.Stats(); runs-runs0 != 1 {
		t.Fatalf("an edit ran %d typst processes, want 1 (layout only)", runs-runs0)
	}
	if pdfs, _ := filepath.Glob(filepath.Join(b.Dir(nb.ID), "*.pdf")); len(pdfs) != 0 {
		t.Fatalf("an edit must not build a PDF: %v", pdfs)
	}
	pdf, err := b.PDF(ctx, nb, bs)
	if data, _ := os.ReadFile(pdf); err != nil || !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("PDF(): %v", err)
	}
	runs1, _ := b.Stats()
	if again, _ := b.PDF(ctx, nb, bs); again != pdf {
		t.Fatal("second PDF() must return the cached file")
	}
	if runs, _ := b.Stats(); runs != runs1 {
		t.Fatal("second PDF() must not run typst")
	}
	bs = appendPara(t, &nb, bs, "edited again")
	if next, _ := b.PDF(ctx, nb, bs); next == pdf {
		t.Fatal("a new version needs a new PDF")
	}
	if pdfs, _ := filepath.Glob(filepath.Join(b.Dir(nb.ID), "*.pdf")); len(pdfs) != 1 {
		t.Fatalf("old PDFs must be removed: %v", pdfs)
	}
}

func TestFingerprints(t *testing.T) {
	nb := store.Notebook{ID: "n_fp", Title: "T", Theme: "clean", Paper: "plain", Mode: "light", Density: "comfortable"}
	bs := blocks(t,
		`{"type":"paragraph","text":"a"}`,
		`{"type":"qa","question":"q1","answer":"a1"}`,
		`{"type":"paragraph","text":"b"}`,
		`{"type":"qa","question":"q2","answer":"a2"}`,
		`{"type":"paragraph","text":"c"}`,
	)
	pos := func(pages ...int) *Result {
		r := &Result{PageCount: 3}
		for i, b := range bs {
			r.Blocks = append(r.Blocks, Pos{ID: b.ID, Type: b.Type, Page: pages[i], EndPage: pages[i], YMM: 20})
		}
		return r
	}
	base := pos(1, 1, 2, 3, 3).Fingerprints(nb, bs)

	grown := pos(1, 1, 2, 3, 3)
	grown.PageCount = 4
	if fp := grown.Fingerprints(nb, bs); fp[0] != base[0] || fp[1] != base[1] || fp[2] != base[2] {
		t.Error("a new last page must not change earlier pages")
	}

	withQA := append(blocks(t, `{"type":"qa","question":"q0","answer":"a0"}`), bs...)
	inserted := &Result{PageCount: 3}
	for i, b := range withQA {
		pg := []int{1, 1, 1, 2, 3, 3}[i]
		inserted.Blocks = append(inserted.Blocks, Pos{ID: b.ID, Type: b.Type, Page: pg, EndPage: pg, YMM: 20})
	}
	if fp := inserted.Fingerprints(nb, withQA); fp[2] == base[2] || fp[1] != base[1] {
		t.Error("inserting a Q&A on page 1 renumbers the Q&A on page 3 (must change) but not page 2, which has none")
	}

	retitled := nb
	retitled.Title = "New title"
	rethemed := nb
	rethemed.Theme = "notebook"
	for _, n := range []store.Notebook{retitled, rethemed} {
		for i, fp := range pos(1, 1, 2, 3, 3).Fingerprints(n, bs) {
			if fp == base[i] {
				t.Errorf("page %d must change when the title or theme changes", i+1)
			}
		}
	}

	edited := append([]model.Block(nil), bs...)
	edited[2].Data = []byte(`{"text":"changed"}`)
	if fp := pos(1, 1, 2, 3, 3).Fingerprints(nb, edited); fp[0] != base[0] || fp[1] == base[1] {
		t.Error("editing page 2 must change page 2 only")
	}

	shorter := []model.Block{bs[0], bs[1], bs[3], bs[4]}
	moved := &Result{PageCount: 2}
	for i, b := range shorter {
		pg := []int{1, 1, 2, 2}[i]
		moved.Blocks = append(moved.Blocks, Pos{ID: b.ID, Type: b.Type, Page: pg, EndPage: pg, YMM: 20})
	}
	if fp := moved.Fingerprints(nb, shorter); fp[1] == base[2] {
		t.Error("the page number is printed in the footer, so page 3's content moving to page 2 must not reuse page 3's image")
	}
}

func TestPageImagesAreReused(t *testing.T) {
	ctx := context.Background()
	b := builder(t)
	old := pruneAfter
	pruneAfter = 0
	defer func() { pruneAfter = old }()
	nb, bs := perfNotebook(t, b, "n_reuse", 4)
	r, _ := b.Build(ctx, nb, bs)
	n := r.PageCount
	if _, err := b.RenderPNG(ctx, nb, bs, allPages(n), 60); err != nil {
		t.Fatal(err)
	}
	if _, drawn := b.Stats(); drawn != int64(n) {
		t.Fatalf("first render drew %d pages, want %d", drawn, n)
	}

	bs = appendPara(t, &nb, bs, "a sentence at the very end")
	r, _ = b.Build(ctx, nb, bs)
	runs0, drawn0 := b.Stats()
	pngs, err := b.RenderPNG(ctx, nb, bs, allPages(r.PageCount), 60)
	if err != nil {
		t.Fatal(err)
	}
	runs1, drawn1 := b.Stats()
	if drawn1-drawn0 > 2 || runs1-runs0 != 1 {
		t.Fatalf("an edit at the end redrew %d pages with %d typst runs; want at most 2 pages in 1 run", drawn1-drawn0, runs1-runs0)
	}
	if _, err := b.RenderPNG(ctx, nb, bs, []int{1}, 60); err != nil {
		t.Fatal(err)
	}
	if runs2, _ := b.Stats(); runs2 != runs1 {
		t.Fatal("an unchanged page must be served without running typst")
	}

	fresh := builder(t)
	freshPNG, err := fresh.RenderPNG(ctx, nb, bs, []int{1}, 60)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pngs[0], freshPNG[0]) {
		t.Fatal("a reused page image must be byte-identical to a fresh render")
	}

	dark := nb
	dark.Mode = "dark"
	_, drawn2 := b.Stats()
	if _, err := b.RenderPNG(ctx, dark, bs, allPages(r.PageCount), 60); err != nil {
		t.Fatal(err)
	}
	if _, drawn3 := b.Stats(); drawn3-drawn2 != int64(r.PageCount) {
		t.Fatalf("dark mode is a separate image set: drew %d pages, want %d", drawn3-drawn2, r.PageCount)
	}
	entries, _ := os.ReadDir(filepath.Join(b.Dir(nb.ID), "pages"))
	if len(entries) != r.PageCount {
		t.Fatalf("cleanup should leave only the current %d page images, found %d", r.PageCount, len(entries))
	}
}
