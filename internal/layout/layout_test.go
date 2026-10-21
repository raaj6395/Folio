package layout

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"os"
	"testing"

	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

func blocks(t *testing.T, raws ...string) []model.Block {
	t.Helper()
	var bs []model.Block
	for _, r := range raws {
		b, err := model.Parse(json.RawMessage(r))
		if err != nil {
			t.Fatal(err)
		}
		b.ID = model.NewID("b_")
		bs = append(bs, b)
	}
	return bs
}

func builder(t *testing.T) *Builder {
	b, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPageBreakPositions(t *testing.T) {
	b := builder(t)
	bs := blocks(t, `{"type":"heading","level":1,"text":"One"}`, `{"type":"page_break"}`, `{"type":"paragraph","text":"Two"}`)
	nb := store.Notebook{ID: "n_test", Title: "T", Theme: "clean", Paper: "plain", Version: 1}
	r, err := b.Build(context.Background(), nb, bs)
	if err != nil {
		t.Fatal(err)
	}
	if r.PageCount != 2 || r.Page(bs[0].ID) != 1 || r.Page(bs[2].ID) != 2 {
		t.Fatalf("layout: %+v", r)
	}
	if st, err := os.Stat(r.PDF); err != nil || st.Size() == 0 {
		t.Fatalf("pdf missing: %v", err)
	}
}

func TestRenderPNGManyPages(t *testing.T) {
	b := builder(t)
	var raws []string
	for range 11 {
		raws = append(raws, `{"type":"paragraph","text":"x"}`, `{"type":"page_break"}`)
	}
	raws = append(raws, `{"type":"paragraph","text":"last"}`)
	nb := store.Notebook{ID: "n_many", Title: "T", Theme: "clean", Paper: "plain", Version: 1}
	pngs, err := b.RenderPNG(context.Background(), nb, blocks(t, raws...), []int{2, 12}, 40)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pngs {
		if !bytes.HasPrefix(p, []byte("\x89PNG")) {
			t.Fatal("not a PNG")
		}
	}
	if _, err := b.RenderPNG(context.Background(), nb, nil, []int{13}, 40); err == nil {
		t.Fatal("expected error for missing page")
	}
}

func TestPageFillAndEnds(t *testing.T) {
	b := builder(t)
	bs := blocks(t, `{"type":"paragraph","text":"top"}`, `{"type":"page_break"}`, `{"type":"heading","level":2,"text":"Second"}`, `{"type":"paragraph","text":"body"}`)
	nb := store.Notebook{ID: "n_fill", Title: "T", Theme: "clean", Paper: "plain", Version: 1}
	r, err := b.Build(context.Background(), nb, bs)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.PageFill) != 2 || r.PageFill[0] <= 0 || r.PageFill[0] > 10 || r.LastPageFill != r.PageFill[1] {
		t.Fatalf("fill: %+v", r.PageFill)
	}
	p := r.Blocks[2]
	if p.Page != 2 || p.EndPage != 2 || p.EndYMM <= p.YMM {
		t.Fatalf("heading extent: %+v", p)
	}
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "because page_break "+bs[1].ID) {
		t.Fatalf("want only the page_break warning, got %v", r.Warnings)
	}
}

func TestPagesMatchPDF(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed")
	}
	b := builder(t)
	var raws []string
	for i := range 60 {
		raws = append(raws, fmt.Sprintf(`{"type":"paragraph","text":"token%03d Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."}`, i))
	}
	bs := blocks(t, raws...)
	nb := store.Notebook{ID: "n_match", Title: "T", Theme: "clean", Paper: "plain", Version: 1}
	r, err := b.Build(context.Background(), nb, bs)
	if err != nil {
		t.Fatal(err)
	}
	if r.PageCount < 3 {
		t.Fatalf("want >=3 pages, got %d", r.PageCount)
	}
	for i, p := range r.Blocks {
		out, err := exec.Command("pdftotext", "-f", fmt.Sprint(p.Page), "-l", fmt.Sprint(p.Page), r.PDF, "-").Output()
		if err != nil {
			t.Fatal(err)
		}
		if tok := fmt.Sprintf("token%03d", i); !strings.Contains(string(out), tok) {
			t.Errorf("%s reported on page %d but not found there", tok, p.Page)
		}
	}
}

func TestKeepWithNextAndNoSplit(t *testing.T) {
	b := builder(t)
	callout := `{"type":"callout","variant":"note","title":"Box","body":["` + strings.Repeat("Callout text that wraps over several lines. ", 12) + `"]}`
	for k := 16; k <= 38; k += 2 {
		var raws []string
		for range k {
			raws = append(raws, `{"type":"paragraph","text":"filler line"}`)
		}
		raws = append(raws, `{"type":"heading","level":2,"text":"Section"}`, callout)
		bs := blocks(t, raws...)
		nb := store.Notebook{ID: fmt.Sprintf("n_k%d", k), Title: "T", Theme: "clean", Paper: "plain", Version: 1}
		r, err := b.Build(context.Background(), nb, bs)
		if err != nil {
			t.Fatal(err)
		}
		h, c := r.Blocks[k], r.Blocks[k+1]
		if h.Page != c.Page {
			t.Errorf("k=%d: heading on page %d, its callout on page %d", k, h.Page, c.Page)
		}
		if c.Page != c.EndPage {
			t.Errorf("k=%d: callout split across pages %d-%d", k, c.Page, c.EndPage)
		}
		for _, w := range r.Warnings {
			if strings.Contains(w, "last thing on page") {
				t.Errorf("k=%d: unexpected warning %s", k, w)
			}
		}
	}
}
