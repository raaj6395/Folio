package layout

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

var perfSection = []string{
	`{"type":"heading","level":2,"text":"Section"}`,
	`{"type":"paragraph","text":"REST maps **resources** to URLs and uses HTTP verbs to act on them. Each request is self-contained, which keeps servers simple and lets them scale horizontally."}`,
	`{"type":"bullets","items":[{"text":"Stateless","children":[{"text":"No server session"}]},{"text":"Cacheable"},{"text":"Uniform interface"}]}`,
	`{"type":"callout","variant":"key_point","body":["Idempotency lets clients retry safely."]}`,
	`{"type":"table","columns":["Method","Idempotent","Safe"],"rows":[["GET","yes","yes"],["PUT","yes","no"],["POST","no","no"]]}`,
	`{"type":"code","language":"go","code":"func main() {\n\tfmt.Println(\"hi\")\n}"}`,
	`{"type":"qa","question":"Is PUT idempotent?","answer":"Yes"}`,
}

func perfNotebook(t *testing.T, b *Builder, id string, pages int) (store.Notebook, []model.Block) {
	t.Helper()
	nb := store.Notebook{ID: id, Title: "Perf", Theme: "clean", Paper: "plain", Mode: "light", Density: "comfortable"}
	var raws []string
	for {
		raws = append(raws, perfSection...)
		nb.Version++
		bs := blocks(t, raws...)
		r, err := b.Build(context.Background(), nb, bs)
		if err != nil {
			t.Fatal(err)
		}
		if r.PageCount >= pages {
			return nb, bs
		}
	}
}

func TestPerf(t *testing.T) {
	if os.Getenv("FOLIO_PERF") == "" {
		t.Skip("set FOLIO_PERF=1 to measure edit cost")
	}
	ctx := context.Background()
	for _, pages := range []int{8, 30} {
		b := builder(t)
		nb, bs := perfNotebook(t, b, fmt.Sprintf("n_perf%d", pages), pages)
		r, _ := b.Build(ctx, nb, bs)
		all := make([]int, r.PageCount)
		for i := range all {
			all[i] = i + 1
		}
		if _, err := b.RenderPNG(ctx, nb, bs, all, 110); err != nil {
			t.Fatal(err)
		}
		for _, where := range []string{"start", "middle", "end"} {
			edited := append([]model.Block(nil), bs...)
			extra := blocks(t, `{"type":"paragraph","text":"One more sentence added during the measurement."}`)[0]
			switch where {
			case "start":
				edited = model.InsertAt(edited, 1, []model.Block{extra})
			case "middle":
				edited = model.InsertAt(edited, len(edited)/2, []model.Block{extra})
			default:
				edited = model.InsertAt(edited, len(edited), []model.Block{extra})
			}
			nb.Version++
			runs0, pages0 := b.Stats()
			start := time.Now()
			r, err := b.Build(ctx, nb, edited)
			if err != nil {
				t.Fatal(err)
			}
			edit := time.Since(start)
			runs1, _ := b.Stats()
			all = all[:0]
			for i := range r.PageCount {
				all = append(all, i+1)
			}
			start = time.Now()
			if _, err := b.RenderPNG(ctx, nb, edited, all, 110); err != nil {
				t.Fatal(err)
			}
			refresh := time.Since(start)
			runs2, pages2 := b.Stats()
			t.Logf("%2d pages, edit at %-6s: edit %4dms (%d typst), viewer refresh %4dms (%d typst, %d pages drawn)",
				r.PageCount, where, edit.Milliseconds(), runs1-runs0, refresh.Milliseconds(), runs2-runs1, pages2-pages0)
		}
	}
}
