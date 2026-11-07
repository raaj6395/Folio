package layout

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

// TestA4Unchanged guards A4 output against template changes made for other formats.
// Regenerate deliberately with FOLIO_UPDATE_GOLDEN=1.
func TestA4Unchanged(t *testing.T) {
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
	var got [][5]float64
	for _, th := range Themes {
		nb := store.Notebook{ID: "n_golden_" + th.Name, Title: "Golden", Theme: th.Name, Paper: th.Paper, Mode: "light", Density: "comfortable", Version: 1}
		r, err := builder(t).Build(context.Background(), nb, bs)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range r.Blocks {
			got = append(got, [5]float64{float64(p.Page), p.YMM, float64(p.EndPage), p.EndYMM, p.XMM})
		}
	}
	const path = "../../testdata/kitchen_sink_a4_positions.json"
	if os.Getenv("FOLIO_UPDATE_GOLDEN") != "" {
		js, _ := json.Marshal(got)
		os.WriteFile(path, js, 0o644)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	js, _ := json.Marshal(got)
	if string(js) != string(want) {
		t.Fatalf("A4 block positions changed:\n got  %s\n want %s", js, want)
	}
}
