package layout

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/raaj6395/folio/internal/store"
)

const nasty = `#import "@preview/x:0.1.0": * #eval("1+1") $x^2$ [content] \ <label> @ref #set page(fill: red) */ // ] ) } "quote" ` + "`raw`" + ` ** == [link](javascript:alert(1)) [l](#eval("x"))`

func TestInjectionRendersLiterally(t *testing.T) {
	b := builder(t)
	s, _ := json.Marshal(nasty)
	bs := blocks(t,
		`{"type":"heading","level":1,"text":`+string(s)+`}`,
		`{"type":"paragraph","text":`+string(s)+`}`,
		`{"type":"bullets","items":[{"text":`+string(s)+`}]}`,
		`{"type":"callout","variant":"note","title":`+string(s)+`,"body":[`+string(s)+`]}`,
		`{"type":"code","language":"#eval(\"x\")","code":`+string(s)+`,"caption":`+string(s)+`}`,
	)
	nb := store.Notebook{ID: "n_inject", Title: nasty, Theme: "clean", Paper: "plain", Version: 1}
	r, err := b.Build(context.Background(), nb, bs)
	if err != nil {
		t.Fatal(err)
	}
	if d := os.Getenv("FOLIO_KEEP"); d != "" {
		b.RenderPNG(context.Background(), nb, bs, []int{1}, 90)
		exec.Command("cp", "-R", b.Dir(nb.ID), d).Run()
	}
	if r.PageCount != 1 {
		t.Fatalf("page count %d", r.PageCount)
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed; skipping literal-text check")
	}
	pdf, err := b.PDF(context.Background(), nb, bs)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("pdftotext", pdf, "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`#eval("1+1")`, `$x^2$`, `<label> @ref`, `#set page(fill: red)`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("PDF text missing literal %q", want)
		}
	}
}
