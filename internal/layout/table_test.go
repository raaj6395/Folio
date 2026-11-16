package layout

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/raaj6395/folio/internal/store"
)

func table(cols, rows int, cell string) string {
	hs, rs := []string{}, [][]string{}
	for c := 0; c < cols; c++ {
		hs = append(hs, fmt.Sprintf("C%d", c+1))
	}
	for r := 0; r < rows; r++ {
		row := []string{}
		for c := 0; c < cols; c++ {
			row = append(row, cell)
		}
		rs = append(rs, row)
	}
	js, _ := json.Marshal(map[string]any{"type": "table", "columns": hs, "rows": rs})
	return string(js)
}

// Tables take any number of columns that fit the width and any number of rows; long ones flow on.
func TestTableSizes(t *testing.T) {
	b := builder(t)
	build := func(id string, raws ...string) *Result {
		t.Helper()
		nb := store.Notebook{ID: id, Title: "T", Theme: "clean", Paper: "plain", Density: "comfortable", Version: 1}
		r, err := b.Build(context.Background(), nb, blocks(t, raws...))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	wide := func(r *Result) int {
		n := 0
		for _, is := range r.Issues {
			if is.Check == "wide_table" {
				n++
			}
		}
		return n
	}

	if r := build("n_many", table(12, 5, "7")); wide(r) != 0 {
		t.Fatalf("12 short columns fit an A4 page: %v", r.Warnings)
	}
	r := build("n_wide", table(12, 5, "Characteristically"))
	if wide(r) != 1 || !strings.Contains(strings.Join(r.Warnings, "\n"), "but only 174mm fits") || r.Page(r.Issues[len(r.Issues)-1].Blocks[0]) != 1 {
		t.Fatalf("12 long-word columns can't fit: %v", r.Warnings)
	}
	r = build("n_long", `{"type":"paragraph","text":"Intro"}`, table(3, 120, "row"))
	p := r.Blocks[1]
	if p.Page != 1 || p.EndPage < 3 {
		t.Fatalf("a 120-row table starts right after the intro and continues: pages %d-%d", p.Page, p.EndPage)
	}
	for _, is := range r.Issues {
		if is.Check == "overflow" || is.Check == "pushed_block" {
			t.Errorf("long table: %s", is.Problem)
		}
	}
}
