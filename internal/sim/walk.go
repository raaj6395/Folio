package sim

import (
	"fmt"
	"strings"
)

// Edge is a connection the viewer draws. Edges are derived from the spec (pools, calls, publishes,
// consumers, replicas), so every hop the engine plays sits on one.
type Edge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Label     string `json:"label,omitempty"`
	Replicate bool   `json:"replicate,omitempty"` // primary → replica: drawn dashed, never carries a hop
}

type walker struct {
	sp      *Spec
	edges   []Edge
	seen    map[string]int // "from>to" → index in edges
	state   map[string]int // "node/handler" → 1 visiting, 2 done
	stack   []string
	used    map[string]bool
	loopErr error
}

// Walk derives the edges by following every request down every branch. It fails on a loop
// (a handler that ends up calling itself), and warns about handlers and nodes nothing uses.
// The spec must already have passed the structural checks.
func Walk(sp *Spec) (edges []Edge, warnings []string, err error) {
	w := &walker{sp: sp, seen: map[string]int{}, state: map[string]int{}, used: map[string]bool{}}
	for i := range sp.Requests {
		r := &sp.Requests[i]
		w.target(sp.client(r), r.To, r.Handler)
	}
	for _, n := range sp.Nodes {
		for _, rep := range n.Replicas {
			w.edge(n.ID, rep)
			w.edges[w.seen[n.ID+">"+rep]].Replicate = true
		}
	}
	if w.loopErr != nil {
		return nil, nil, w.loopErr
	}
	for _, l := range sp.Links {
		i, ok := w.seen[l.From+">"+l.To]
		if !ok {
			return nil, nil, fmt.Errorf("links: %s → %s is not a connection in this system; links only label connections that calls, pools, queues or replicas already make", l.From, l.To)
		}
		w.edges[i].Label = l.Label
	}
	linked := map[string]bool{}
	for _, e := range w.edges {
		linked[e.From], linked[e.To] = true, true
	}
	for _, n := range sp.Nodes {
		if !linked[n.ID] {
			warnings = append(warnings, fmt.Sprintf("node %q is not connected to anything: no request reaches it", n.ID))
		}
	}
	for name := range sp.Handlers {
		if !w.used[name] {
			warnings = append(warnings, fmt.Sprintf("handler %q is never run: no request, call or queue uses it", name))
		}
	}
	return w.edges, sorted(warnings), nil
}

func (w *walker) edge(from, to string) {
	k := from + ">" + to
	if _, ok := w.seen[k]; !ok {
		w.seen[k] = len(w.edges)
		w.edges = append(w.edges, Edge{From: from, To: to})
	}
}

// target is a call from → to running handler there (through the pool when to is a load balancer).
func (w *walker) target(from, to, handler string) {
	w.edge(from, to)
	n := w.sp.Node(to)
	if n.Kind == "load_balancer" {
		for _, m := range n.Pool {
			w.target(to, m, handler)
		}
		return
	}
	w.visit(to, handler)
}

func (w *walker) visit(node, handler string) {
	k := node + "/" + handler
	w.used[handler] = true
	switch w.state[k] {
	case 1:
		if w.loopErr == nil {
			i := 0
			for w.stack[i] != k {
				i++
			}
			w.loopErr = fmt.Errorf("handlers call each other in a loop: %s → %s; a request would never finish", strings.Join(w.stack[i:], " → "), k)
		}
		return
	case 2:
		return
	}
	w.state[k] = 1
	w.stack = append(w.stack, k)
	w.steps(node, w.sp.Handlers[handler])
	w.stack = w.stack[:len(w.stack)-1]
	w.state[k] = 2
}

func (w *walker) steps(node string, steps []Step) {
	for _, s := range steps {
		switch {
		case len(s.Parallel) > 0:
			for _, br := range s.Parallel {
				w.steps(node, br)
			}
		case s.Publish != "":
			q := w.sp.Node(s.Publish)
			w.edge(node, q.ID)
			for _, c := range q.Consumers {
				w.edge(q.ID, c)
				w.visit(c, q.Handler)
			}
		default:
			t := w.sp.Node(s.Call)
			switch t.Kind {
			case "service", "load_balancer":
				if s.Force != "" {
					w.edge(node, t.ID)
				} else {
					w.target(node, t.ID, s.Handler)
				}
			default:
				w.edge(node, t.ID)
				if t.Kind == "database" && s.Op == "read" {
					for _, rep := range t.Replicas {
						w.edge(node, rep)
					}
				}
			}
		}
		for _, br := range s.On {
			w.steps(node, br.Steps)
		}
	}
}
