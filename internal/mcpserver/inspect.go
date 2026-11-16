package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/raaj6395/folio/internal/layout"
)

// inspectBudget caps an inspection's JSON per page shown (~4k tokens); findings are never cut.
const inspectBudget = 16000

func (s *Server) inspect(ctx context.Context, in inspectIn) (layout.Inspection, error) {
	if in.Detail == "" {
		in.Detail = "summary"
	}
	if in.Detail != "summary" && in.Detail != "full" {
		return layout.Inspection{}, fmt.Errorf(`detail must be "summary" or "full", got %q`, in.Detail)
	}
	nb, bs, err := s.load(ctx, in.NotebookID)
	if err != nil {
		return layout.Inspection{}, err
	}
	r, err := s.build(ctx, nb, bs)
	if err != nil {
		return layout.Inspection{}, err
	}
	for _, p := range in.Pages {
		if p < 1 || p > r.PageCount {
			return layout.Inspection{}, fmt.Errorf("page %d does not exist; the notebook has %d page(s)", p, r.PageCount)
		}
	}
	score, reports := checked(nb, bs, r.With(r.ExtraIssues())).Report()
	if len(in.Pages) > 0 {
		total := 0
		for _, p := range in.Pages {
			total += reports[p-1].Score
		}
		score = (total + len(in.Pages)/2) / len(in.Pages)
	}
	out := r.Inspect(checked(nb, bs, r).Issues, score, in.Pages, in.BlockIDs, in.Detail == "full")
	out.Version = nb.Version
	trim(&out)
	return out, nil
}

// trim drops detail until the inspection fits its budget: block lines and boxes, then diagram
// coordinates, then ASCII maps. Findings always stay.
func trim(out *layout.Inspection) {
	limit := inspectBudget * max(1, len(out.Pages))
	size := func() int { js, _ := json.Marshal(out); return len(js) }
	steps := []struct {
		what string
		do   func()
	}{
		{"per-block lines and boxes", func() {
			for i := range out.Pages {
				out.Pages[i].Summary, out.Pages[i].Blocks = nil, nil
			}
		}},
		{"diagram nodes and edges (ask for one block_id with detail full)", func() {
			for i := range out.Diagrams {
				out.Diagrams[i].Nodes, out.Diagrams[i].Edges = nil, nil
			}
		}},
		{"ASCII maps (ask for fewer pages or block_ids)", func() {
			for i := range out.Diagrams {
				out.Diagrams[i].ASCII = ""
			}
		}},
	}
	for _, st := range steps {
		if size() <= limit {
			return
		}
		st.do()
		if out.Trimmed != "" {
			out.Trimmed += "; "
		}
		out.Trimmed += "dropped " + st.what + " to stay under ~4k tokens per page"
	}
}
