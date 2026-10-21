package layout

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/raaj6395/folio/internal/store"
)

func TestPerf(t *testing.T) {
	if os.Getenv("FOLIO_PERF") == "" {
		t.Skip("set FOLIO_PERF=1 to measure rebuild times")
	}
	b := builder(t)
	section := []string{
		`{"type":"heading","level":2,"text":"Section"}`,
		`{"type":"paragraph","text":"REST maps **resources** to URLs and uses HTTP verbs to act on them. Each request is self-contained, which keeps servers simple and lets them scale horizontally."}`,
		`{"type":"bullets","items":[{"text":"Stateless","children":[{"text":"No server session"}]},{"text":"Cacheable"},{"text":"Uniform interface"}]}`,
		`{"type":"callout","variant":"key_point","body":["Idempotency lets clients retry safely."]}`,
		`{"type":"table","columns":["Method","Idempotent","Safe"],"rows":[["GET","yes","yes"],["PUT","yes","no"],["POST","no","no"]]}`,
		`{"type":"code","language":"go","code":"func main() {\n\tfmt.Println(\"hi\")\n}"}`,
		`{"type":"qa","question":"Is PUT idempotent?","answer":"Yes"}`,
	}
	for _, pages := range []int{10, 30} {
		var raws []string
		nb := store.Notebook{ID: fmt.Sprintf("n_perf%d", pages), Title: "Perf", Theme: "clean", Paper: "plain"}
		var r *Result
		for n := 0; r == nil || r.PageCount < pages; n++ {
			raws = append(raws, section...)
			nb.Version = n + 1
			var err error
			if r, err = b.Build(context.Background(), nb, blocks(t, raws...)); err != nil {
				t.Fatal(err)
			}
		}
		bs := blocks(t, raws...)
		var total time.Duration
		const runs = 5
		for i := range runs {
			nb.Version = 1000 + i
			start := time.Now()
			if _, err := b.Build(context.Background(), nb, bs); err != nil {
				t.Fatal(err)
			}
			total += time.Since(start)
		}
		start := time.Now()
		b.Build(context.Background(), nb, bs)
		cached := time.Since(start)
		t.Logf("%d pages (%d blocks): rebuild avg %v over %d runs; cached %v", r.PageCount, len(bs), (total / runs).Round(time.Millisecond), runs, cached.Round(time.Microsecond))
	}
}
