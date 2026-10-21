package layout

import (
	"bytes"
	"context"
	"encoding/json"
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
