package layout

import (
	"context"
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
