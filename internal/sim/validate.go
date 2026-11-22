package sim

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Summary is what a write of a system tells Claude.
type Summary struct {
	Nodes     int      `json:"nodes"`
	Edges     int      `json:"edges"`
	Requests  []string `json:"requests"`
	Scenarios string   `json:"scenarios"`
	Warnings  []string `json:"warnings"`
}

// Validate checks a decoded spec: structure, references, loops, that every request finishes with
// any single node down, and every scenario's expect. It sets engine to the current version when
// it is missing. All structural errors are reported at once so one fix round covers them.
func Validate(sp *Spec) (*Summary, error) {
	if sp.Engine == 0 {
		sp.Engine = Engine
	}
	v := &checker{sp: sp}
	v.structure()
	if len(v.errs) > 0 {
		return nil, v.err()
	}
	edges, warnings, err := Walk(sp)
	if err != nil {
		return nil, err
	}
	if err := exhaust(sp); err != nil {
		return nil, err
	}
	for i, sc := range sp.Scenarios {
		if err := checkScenario(sp, sc); err != nil {
			return nil, fmt.Errorf("scenarios[%d] %q: %v", i, sc.Label, err)
		}
	}
	sum := &Summary{Nodes: len(sp.Nodes), Edges: len(edges), Warnings: warnings, Scenarios: fmt.Sprintf("%d/%d pass", len(sp.Scenarios), len(sp.Scenarios))}
	if sum.Warnings == nil {
		sum.Warnings = []string{}
	}
	for _, r := range sp.Requests {
		sum.Requests = append(sum.Requests, r.ID)
	}
	return sum, nil
}

type checker struct {
	sp    *Spec
	errs  []string
	steps int
}

func (v *checker) add(path, format string, a ...any) {
	if len(v.errs) < 12 {
		v.errs = append(v.errs, path+": "+fmt.Sprintf(format, a...))
	}
}

func (v *checker) err() error {
	if len(v.errs) == 1 {
		return errors.New(v.errs[0])
	}
	return fmt.Errorf("%d problems:\n- %s", len(v.errs), strings.Join(v.errs, "\n- "))
}

// suggest names the closest existing id, for "unknown node" errors.
func suggest(name string, options []string) string {
	best, bestD := "", 4
	for _, o := range options {
		d := distance(name, o)
		if strings.Contains(o, name) || strings.Contains(name, o) {
			d = min(d, 2)
		}
		if d < bestD {
			best, bestD = o, d
		}
	}
	if best != "" {
		return fmt.Sprintf(" (did you mean %q?)", best)
	}
	if len(options) > 0 && len(options) <= 12 {
		return "; valid: " + strings.Join(options, ", ")
	}
	return ""
}

func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func (v *checker) ids(kinds ...string) []string {
	var out []string
	for _, n := range v.sp.Nodes {
		if len(kinds) == 0 || slices.Contains(kinds, n.Kind) {
			out = append(out, n.ID)
		}
	}
	return out
}

func (v *checker) handlerNames() []string {
	var out []string
	for k := range v.sp.Handlers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ref checks that id names a node of one of kinds.
func (v *checker) ref(path, id string, kinds ...string) bool {
	n := v.sp.Node(id)
	if n == nil {
		v.add(path, "unknown node %q%s", id, suggest(id, v.ids(kinds...)))
		return false
	}
	if !slices.Contains(kinds, n.Kind) {
		v.add(path, "%q is a %s; expected %s", id, n.Kind, strings.Join(kinds, " or "))
		return false
	}
	return true
}

func (v *checker) handlerRef(path, name string) {
	if name == "" {
		v.add(path, "handler is required")
	} else if _, ok := v.sp.Handlers[name]; !ok {
		v.add(path, "unknown handler %q%s", name, suggest(name, v.handlerNames()))
	}
}

func (v *checker) text(path, s string, max int) {
	if runes(s) > max {
		v.add(path, "is %d characters, max %d", runes(s), max)
	}
}

func (v *checker) structure() {
	sp := v.sp
	if sp.Engine != Engine {
		v.add("engine", "must be %d, got %d", Engine, sp.Engine)
	}
	if !slices.Contains(directions, sp.Direction) {
		v.add("direction", `must be "right" or "down", got %q`, sp.Direction)
	}
	if len(sp.Nodes) == 0 || len(sp.Nodes) > maxNodes {
		v.add("nodes", "must list 1 to %d nodes, got %d", maxNodes, len(sp.Nodes))
	}
	seen := map[string]bool{}
	groups := map[string]bool{}
	clients := 0
	for i, n := range sp.Nodes {
		p := fmt.Sprintf("nodes[%d]", i)
		if !validName.MatchString(n.ID) {
			v.add(p+".id", "%q must be lowercase letters, digits and _, starting with a letter (max 40)", n.ID)
		} else if seen[n.ID] {
			v.add(p+".id", "%q is used by another node", n.ID)
		}
		seen[n.ID] = true
		if !slices.Contains(Kinds, n.Kind) {
			v.add(p+".kind", "unknown kind %q; valid: %s", n.Kind, strings.Join(Kinds, ", "))
		}
		if n.Kind == "client" {
			clients++
		}
		if n.Icon != "" && !slices.Contains(Icons, n.Icon) {
			v.add(p+".icon", "unknown icon %q; valid: %s", n.Icon, strings.Join(Icons, ", "))
		}
		v.text(p+".label", n.Label, 40)
		v.text(p+".group", n.Group, 30)
		v.text(p+".about", n.About, 300)
		v.text(p+".when_down", n.WhenDown, 300)
		if n.Group != "" {
			groups[n.Group] = true
		}
		if n.MS < 0 || n.MS > 10000 {
			v.add(p+".ms", "must be 0 to 10000, got %d", n.MS)
		}
		only := func(field string, set bool, kind string) {
			if set && n.Kind != kind && slices.Contains(Kinds, n.Kind) {
				v.add(p+"."+field, "only a %s takes %s", kind, field)
			}
		}
		only("pool", len(n.Pool) > 0, "load_balancer")
		only("strategy", n.Strategy != "", "load_balancer")
		only("replicas", len(n.Replicas) > 0, "database")
		only("consumers", len(n.Consumers) > 0, "queue")
		only("handler", n.Handler != "", "queue")
		only("persistent", n.Persistent, "cache")
	}
	if clients == 0 && len(sp.Nodes) > 0 {
		v.add("nodes", `need at least one node of kind "client" to send requests`)
	}
	if len(v.errs) > 0 {
		return // references below assume ids and kinds are sound
	}
	for g := range groups {
		if seen[g] {
			v.add("nodes", "group %q has the same name as a node; rename one", g)
		}
	}
	for i, n := range sp.Nodes {
		p := fmt.Sprintf("nodes[%d]", i)
		switch n.Kind {
		case "load_balancer":
			if len(n.Pool) == 0 {
				v.add(p+".pool", "a load_balancer needs a pool of services")
			}
			for j, m := range n.Pool {
				v.ref(fmt.Sprintf("%s.pool[%d]", p, j), m, "service")
			}
			if dup := dupe(n.Pool); dup != "" {
				v.add(p+".pool", "lists %q twice", dup)
			}
			if n.Strategy != "" && n.Strategy != "round_robin" && n.Strategy != "hash" {
				v.add(p+".strategy", `must be "round_robin" or "hash", got %q`, n.Strategy)
			}
		case "database":
			for j, r := range n.Replicas {
				rp := fmt.Sprintf("%s.replicas[%d]", p, j)
				if r == n.ID {
					v.add(rp, "a database can't be its own replica")
				} else if v.ref(rp, r, "database") && len(sp.Node(r).Replicas) > 0 {
					v.add(rp, "%q has replicas of its own; replicas can't be chained", r)
				}
			}
		case "queue":
			if len(n.Consumers) == 0 {
				v.add(p+".consumers", "a queue needs consumers: the services that take its messages")
			}
			for j, c := range n.Consumers {
				v.ref(fmt.Sprintf("%s.consumers[%d]", p, j), c, "service")
			}
			v.handlerRef(p+".handler", n.Handler)
		}
	}
	if len(sp.Handlers) > maxHandlers {
		v.add("handlers", "max %d handlers, got %d", maxHandlers, len(sp.Handlers))
	}
	for _, name := range v.handlerNames() {
		p := "handlers." + name
		if !validName.MatchString(name) {
			v.add(p, "handler names must be lowercase letters, digits and _, starting with a letter")
		}
		v.stepList(p, sp.Handlers[name], 1)
	}
	if v.steps > maxSteps {
		v.add("handlers", "%d steps in all, max %d; simplify the handlers", v.steps, maxSteps)
	}
	if len(sp.Requests) == 0 || len(sp.Requests) > maxRequests {
		v.add("requests", "must list 1 to %d requests, got %d", maxRequests, len(sp.Requests))
	}
	reqs := map[string]bool{}
	for i, r := range sp.Requests {
		p := fmt.Sprintf("requests[%d]", i)
		if !validName.MatchString(r.ID) {
			v.add(p+".id", "%q must be lowercase letters, digits and _, starting with a letter", r.ID)
		} else if reqs[r.ID] {
			v.add(p+".id", "%q is used by another request", r.ID)
		}
		reqs[r.ID] = true
		v.text(p+".label", r.Label, 60)
		if r.From != "" {
			v.ref(p+".from", r.From, "client")
		} else if clients > 1 {
			v.add(p+".from", "there are %d clients; say which one sends this request", clients)
		}
		v.ref(p+".to", r.To, "load_balancer", "service")
		v.handlerRef(p+".handler", r.Handler)
		if len(r.Keys) == 0 || len(r.Keys) > maxKeys {
			v.add(p+".keys", "must list 1 to %d keys (the ids a request is about, like \"user:42\")", maxKeys)
		}
		for j, k := range r.Keys {
			if strings.TrimSpace(k) == "" || runes(k) > 60 {
				v.add(fmt.Sprintf("%s.keys[%d]", p, j), "must be 1 to 60 characters")
			}
		}
	}
	if len(sp.Scenarios) > maxScenarios {
		v.add("scenarios", "max %d scenarios, got %d", maxScenarios, len(sp.Scenarios))
	}
	for i, sc := range sp.Scenarios {
		p := fmt.Sprintf("scenarios[%d]", i)
		if strings.TrimSpace(sc.Label) == "" {
			v.add(p+".label", "is required")
		}
		v.text(p+".label", sc.Label, 40)
		v.text(p+".explain", sc.Explain, 400)
		for j, k := range sc.Kill {
			if len(sp.members(k)) == 0 {
				v.add(fmt.Sprintf("%s.kill[%d]", p, j), "%q is neither a node nor a group%s", k, suggest(k, v.ids()))
			}
		}
		r := sp.Request(sc.Request)
		if r == nil {
			v.add(p+".request", "unknown request %q", sc.Request)
		} else if sc.Key != "" && !slices.Contains(r.Keys, sc.Key) {
			v.add(p+".key", "%q is not one of request %q's keys: %s", sc.Key, r.ID, strings.Join(r.Keys, ", "))
		}
		if e := sc.Expect; e != nil {
			if e.Status != 0 && (e.Status < 100 || e.Status > 599) {
				v.add(p+".expect.status", "must be an HTTP status, got %d", e.Status)
			}
			for j, id := range append(slices.Clone(e.Visits), e.Avoids...) {
				if sp.Node(id) == nil {
					v.add(fmt.Sprintf("%s.expect", p), "entry %d: unknown node %q%s", j, id, suggest(id, v.ids()))
				}
			}
		}
	}
	for i, l := range sp.Links {
		p := fmt.Sprintf("links[%d]", i)
		v.ref(p+".from", l.From, Kinds...)
		v.ref(p+".to", l.To, Kinds...)
		if strings.TrimSpace(l.Label) == "" || runes(l.Label) > 30 {
			v.add(p+".label", "must be 1 to 30 characters")
		}
	}
}

func (v *checker) stepList(path string, steps []Step, depth int) {
	if len(steps) == 0 {
		v.add(path, "must contain at least one step")
	}
	if depth > maxDepth {
		v.add(path, "steps nest %d deep, max %d; flatten them or move a part into its own handler on another service", depth, maxDepth)
		return
	}
	for i, s := range steps {
		v.step(fmt.Sprintf("%s[%d]", path, i), s, depth)
	}
}

func (v *checker) step(p string, s Step, depth int) {
	v.steps++
	kinds := 0
	for _, set := range []bool{s.Call != "", s.Publish != "", len(s.Parallel) > 0} {
		if set {
			kinds++
		}
	}
	if kinds != 1 {
		v.add(p, `a step needs exactly one of "call", "publish" or "parallel"`)
		return
	}
	v.text(p+".say", s.Say, 120)
	v.text(p+".key", s.Key, 60)
	if s.Retry < 0 || s.Retry > 5 {
		v.add(p+".retry", "must be 0 to 5, got %d", s.Retry)
	}
	if s.Async && len(s.On) > 0 {
		v.add(p, "an async step doesn't wait for its outcome, so it can't have on")
	}
	if len(s.Parallel) > 0 {
		if s.Async || s.Retry > 0 || len(s.On) > 0 || s.Force != "" || s.Key != "" || s.Op != "" || s.Handler != "" {
			v.add(p, "parallel takes only its branches (and say)")
		}
		if len(s.Parallel) < 2 {
			v.add(p+".parallel", "needs at least 2 branches")
		}
		for i, br := range s.Parallel {
			v.stepList(fmt.Sprintf("%s.parallel[%d]", p, i), br, depth+1)
		}
		return
	}
	if s.Publish != "" {
		if !v.ref(p+".publish", s.Publish, "queue") {
			return
		}
		if s.Op != "" || s.Handler != "" || s.Limit != 0 || s.Force != "" {
			v.add(p, "publish takes only the queue (plus key, async, retry, on, say); the queue's consumers run its handler")
		}
	} else {
		n := v.sp.Node(s.Call)
		if n == nil {
			v.add(p+".call", "unknown node %q%s", s.Call, suggest(s.Call, v.ids()))
			return
		}
		switch n.Kind {
		case "client":
			v.add(p+".call", "%q is a client; clients send requests but can't be called", s.Call)
			return
		case "queue":
			v.add(p+".call", `%q is a queue; use {"publish": %q} to send it a message`, s.Call, s.Call)
			return
		case "service", "load_balancer":
			if s.Op != "" || s.Limit != 0 || s.Key != "" {
				v.add(p, `calls to a %s take "handler", not op, key or limit`, n.Kind)
			}
			if s.Force == "" {
				v.handlerRef(p+".handler", s.Handler)
			} else if s.Handler != "" {
				v.handlerRef(p+".handler", s.Handler)
			}
		case "cache", "database":
			ops := cacheOps
			if n.Kind == "database" {
				ops = dbOps
			}
			if !slices.Contains(ops, s.Op) {
				v.add(p+".op", "a %s takes op %s; got %q", n.Kind, strings.Join(ops, ", "), s.Op)
				return
			}
			if s.Handler != "" {
				v.add(p+".handler", "only services and load balancers run handlers")
			}
			if s.Op == "incr" && s.Limit < 1 {
				v.add(p+".limit", "incr needs a limit (the count at which requests are refused)")
			} else if s.Op != "incr" && s.Limit != 0 {
				v.add(p+".limit", "only incr takes a limit")
			}
		}
	}
	outs := v.sp.outcomes(s)
	if s.Force != "" && !slices.Contains(outs, s.Force) {
		v.add(p+".force", "%q is not a possible outcome here; valid: %s", s.Force, strings.Join(outs, ", "))
	}
	keys := make([]string, 0, len(s.On))
	for k := range s.On {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		bp := p + ".on." + k
		if !slices.Contains(outs, k) {
			v.add(bp, "%q is not a possible outcome here; valid: %s", k, strings.Join(outs, ", "))
			continue
		}
		br := s.On[k]
		if br.Do != "" && !slices.Contains(branchDos, br.Do) {
			v.add(bp, `must be "next", "return", "fail" or a list of steps, got %q`, br.Do)
		}
		if br.Do == "" {
			v.stepList(bp, br.Steps, depth+1)
		}
	}
}

func dupe(ss []string) string {
	seen := map[string]bool{}
	for _, s := range ss {
		if seen[s] {
			return s
		}
		seen[s] = true
	}
	return ""
}

func sorted(ss []string) []string {
	sort.Strings(ss)
	return ss
}

// exhaust runs every request, cold then warm, with nothing down and with each node down in turn.
// It catches runs that blow the engine's limits; loops are already refused by Walk.
func exhaust(sp *Spec) error {
	kills := []string{""}
	for _, n := range sp.Nodes {
		kills = append(kills, n.ID)
	}
	for _, r := range sp.Requests {
		for _, key := range r.Keys[:min(2, len(r.Keys))] {
			for _, k := range kills {
				st := State{}
				if k != "" {
					st.Down = []string{k}
				}
				for range 2 {
					res, err := Run(sp, st, Input{Action: "send", Request: r.ID, Key: key})
					if err != nil {
						with := ""
						if k != "" {
							with = " with " + k + " down"
						}
						return fmt.Errorf("request %q (key %s)%s: %v", r.ID, key, with, err)
					}
					st = res.State
				}
			}
		}
	}
	return nil
}

func checkScenario(sp *Spec, sc Scenario) error {
	res, err := RunScenario(sp, sc)
	if err != nil {
		return err
	}
	if sc.Expect == nil {
		return nil
	}
	got := Visited(res.Events)
	var problems []string
	if e := sc.Expect.Status; e != 0 && e != res.Status {
		problems = append(problems, fmt.Sprintf("expected status %d, got %d", e, res.Status))
	}
	for _, id := range sc.Expect.Visits {
		if !got[id] {
			problems = append(problems, fmt.Sprintf("expected the request to reach %q, it didn't", id))
		}
	}
	for _, id := range sc.Expect.Avoids {
		if got[id] {
			problems = append(problems, fmt.Sprintf("expected the request to avoid %q, it reached it", id))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s. Fix the handlers or the expect. What happened:\n%s", strings.Join(problems, "; "), strings.Join(Log(res), "\n"))
}

// Visited is the set of nodes a request actually reached (a hop to a dead node doesn't count).
func Visited(evs []Event) map[string]bool {
	got := map[string]bool{}
	for _, e := range evs {
		if e.Dir == "req" && !e.Fail && e.To != "" {
			got[e.To] = true
		}
	}
	return got
}
