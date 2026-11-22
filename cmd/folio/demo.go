package main

import (
	"encoding/json"
	"fmt"

	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/sim"
	"github.com/raaj6395/folio/internal/store"
	"github.com/raaj6395/folio/internal/viewer"
)

// runDemo installs the reference systems as system notebooks for the local user. Running it
// again refreshes them in place (a new version each), so it never makes duplicates.
func runDemo() {
	st, _ := open()
	defer st.Close()
	if err := st.EnsureLocalUser(); err != nil {
		fatal(err)
	}
	nbs, err := st.Notebooks(store.LocalUser)
	if err != nil {
		fatal(err)
	}
	byTitle := map[string]string{}
	for _, nb := range nbs {
		byTitle[nb.Title] = nb.ID
	}
	fmt.Println("Demo systems:")
	for _, d := range sim.Demos() {
		var m map[string]any
		json.Unmarshal(d.JSON, &m)
		m["type"] = "system"
		js, _ := json.Marshal(m)
		blk, err := model.Parse(js)
		if err != nil {
			fatal(fmt.Errorf("demo %s: %w", d.Name, err))
		}
		id, ok := byTitle[d.Title]
		if !ok {
			nb, err := st.CreateNotebook(store.LocalUser, store.Notebook{Title: d.Title, Theme: "clean", Paper: "plain", Mode: "dark", Density: store.DefaultDensity, Format: "system"})
			if err != nil {
				fatal(err)
			}
			id = nb.ID
		}
		_, _, err = st.Mutate(store.LocalUser, id, nil, "demo", "installed the demo system", func(nb *store.Notebook, bs []model.Block) ([]model.Block, error) {
			if nb.Format != "system" {
				return nil, fmt.Errorf("a notebook called %q already exists and is not a system notebook; rename it and run again", nb.Title)
			}
			if len(bs) == 1 {
				bs[0].Type, bs[0].Data = blk.Type, blk.Data
				return bs, nil
			}
			blk.ID = model.NewID("b_")
			return model.InsertAt(nil, 0, []model.Block{blk}), nil
		})
		if err != nil {
			fatal(err)
		}
		fmt.Printf("  %-36s http://localhost:%d/n/%s\n", d.Title, viewer.DefaultPort, id)
	}
	if viewer.Running(viewer.DefaultPort) {
		fmt.Println("The viewer is running: open a link above.")
	} else {
		fmt.Println("Start the viewer with `folio serve` (or `make run-viewer`), then open a link above.")
	}
}
