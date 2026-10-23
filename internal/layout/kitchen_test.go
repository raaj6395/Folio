package layout

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

func TestKitchenSinkAllThemes(t *testing.T) {
	data, err := os.ReadFile("../../testdata/kitchen_sink.json")
	if err != nil {
		t.Fatal(err)
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		t.Fatal(err)
	}
	bs, err := model.ParseAll(raws)
	if err != nil {
		t.Fatal(err)
	}
	for i := range bs {
		bs[i].ID = model.NewID("b_")
	}
	b := builder(t)
	out := os.Getenv("FOLIO_KITCHEN")
	for _, th := range Themes {
		papers := []string{th.Paper}
		if th.Name == "clean" {
			papers = append(papers, "grid")
		}
		for _, paper := range papers {
			for _, mode := range Modes {
				nb := store.Notebook{ID: "n_ks_" + th.Name + "_" + paper + "_" + mode, Title: "Kitchen sink (" + th.Name + ", " + mode + ")", Theme: th.Name, Paper: paper, Mode: mode, Version: 1}
				r, err := b.Build(context.Background(), nb, bs)
				if err != nil {
					t.Fatalf("%s/%s: %v", th.Name, paper, err)
				}
				if r.PageCount < 3 {
					t.Errorf("%s/%s: only %d pages", th.Name, paper, r.PageCount)
				}
				t.Logf("%s/%s/%s: %d pages, warnings: %v", th.Name, paper, mode, r.PageCount, r.Warnings)
				if out != "" {
					os.MkdirAll(out, 0o755)
					pdf, err := b.PDF(context.Background(), nb, bs)
					if err != nil {
						t.Fatal(err)
					}
					exec.Command("cp", pdf, filepath.Join(out, "kitchen-sink-"+th.Name+"-"+paper+"-"+mode+".pdf")).Run()
				}
			}
		}
	}
}
