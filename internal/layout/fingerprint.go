package layout

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"

	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

func (r *Result) Fingerprints(nb store.Notebook, bs []model.Block) []string {
	var qaStarts []int
	for _, b := range bs {
		if b.Type == "qa" {
			qaStarts = append(qaStarts, r.Page(b.ID))
		}
		for _, k := range b.Children() {
			if k.Type == "qa" {
				qaStarts = append(qaStarts, r.Page(k.ID))
			}
		}
	}
	hs := make([]hash.Hash, r.PageCount)
	for i := range hs {
		qaBefore, qaHere := 0, false
		for _, p := range qaStarts {
			if p > 0 && p < i+1 {
				qaBefore++
			}
			if p == i+1 {
				qaHere = true
			}
		}
		hs[i] = sha256.New()
		fmt.Fprintf(hs[i], "%s|%s|%s|%s|%s|%s|page %d|", templateHash, nb.Title, nb.Theme, nb.Paper, nb.Mode, nb.Density, i+1)
		if qaHere {
			fmt.Fprintf(hs[i], "qa %d|", qaBefore)
		}
	}
	for i, p := range r.Blocks {
		for pg := max(p.Page, 1); pg <= min(max(p.EndPage, p.Page), r.PageCount); pg++ {
			fmt.Fprintf(hs[pg-1], "%s:%s:%s:%.1f;", p.ID, bs[i].Type, bs[i].Data, p.YMM)
		}
	}
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = hex.EncodeToString(h.Sum(nil))[:16]
	}
	return out
}
