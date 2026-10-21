package layout

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/raaj6395/folio/internal/store"
)

func TestM25Blocks(t *testing.T) {
	b := builder(t)
	bs := blocks(t,
		`{"type":"heading","level":2,"text":"HTTP methods"}`,
		`{"type":"table","columns":["Method","Idempotent","Safe"],"rows":[["GET","yes","yes"],["PUT","yes","no"],["POST","**no**","no"]],"caption":"Method properties"}`,
		`{"type":"qa","question":"Is PUT idempotent?","answer":"Yes: repeating it gives the same state."}`,
		`{"type":"qa","question":"Which methods are safe?","answer":"GET, HEAD, OPTIONS","hide_answer":true}`,
		`{"type":"spacer","size":"large"}`,
		`{"type":"fill_in","prompt":"Explain idempotency in your own words.","lines":4}`,
	)
	nb := store.Notebook{ID: "n_m25", Title: "M2.5", Theme: "clean", Paper: "plain", Version: 1}
	r, err := b.Build(context.Background(), nb, bs)
	if err != nil {
		t.Fatal(err)
	}
	if r.PageCount != 1 {
		t.Fatalf("pages %d", r.PageCount)
	}
	sp := r.Blocks[4]
	if h := sp.EndYMM - sp.YMM; h < 19 || h > 21 {
		t.Fatalf("large spacer height %.1fmm, want ~20", h)
	}
	if d := os.Getenv("FOLIO_KEEP"); d != "" {
		b.RenderPNG(context.Background(), nb, bs, []int{1}, 90)
		exec.Command("cp", "-R", b.Dir(nb.ID), d).Run()
	}
}

func TestDiagramRender(t *testing.T) {
	b := builder(t)
	bs := blocks(t,
		`{"type":"heading","level":2,"text":"Request lifecycle"}`,
		`{"type":"diagram","source":"client -> lb: HTTPS\nlb -> api: route\napi -> cache: lookup\napi -> db: query\ndb -> api: rows\napi -> client: JSON","caption":"Request lifecycle"}`,
		`{"type":"diagram","source":"shape: sequence_diagram\nclient -> server: GET /users/42\nserver -> client: 200 OK","size":"half"}`,
	)
	nb := store.Notebook{ID: "n_d2", Title: "D2", Theme: "clean", Paper: "plain", Version: 1}
	r, err := b.Build(context.Background(), nb, bs)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 0 {
		t.Fatal(r.Warnings)
	}
	if d := os.Getenv("FOLIO_KEEP"); d != "" {
		b.RenderPNG(context.Background(), nb, bs, []int{1, r.PageCount}, 70)
		exec.Command("cp", "-R", b.Dir(nb.ID), d).Run()
	}
}

func TestTallDiagramKeepsCaption(t *testing.T) {
	b := builder(t)
	for k := 0; k <= 30; k += 2 {
		var raws []string
		for range k {
			raws = append(raws, `{"type":"paragraph","text":"filler line"}`)
		}
		raws = append(raws, `{"type":"diagram","source":"a -> b -> c -> d","caption":"Tall"}`)
		bs := blocks(t, raws...)
		r, err := b.Build(context.Background(), store.Notebook{ID: fmt.Sprintf("n_tall%d", k), Theme: "clean", Paper: "plain", Version: 1}, bs)
		if err != nil {
			t.Fatal(err)
		}
		if d := r.Blocks[len(r.Blocks)-1]; d.Page != d.EndPage {
			t.Fatalf("k=%d: diagram split from its caption: %+v", k, d)
		}
	}
}
