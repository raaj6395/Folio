package sim

import (
	"fmt"
	"hash/fnv"
	"net/http"
	"slices"
	"sort"
	"strings"
)

// State is everything that changes while someone plays with a system. The viewer keeps it and
// sends it with every action, so the server stores nothing per viewer.
type State struct {
	Down     []string                  `json:"down"`
	Cache    map[string][]string       `json:"cache"`
	Counters map[string]map[string]int `json:"counters"`
	RR       map[string]int            `json:"rr"`
	Backlog  map[string][]string       `json:"backlog"` // queue id → keys of waiting messages
}

// Input is one action: send a request, kill/revive a node or group, flush a cache, reset, or play a scenario.
type Input struct {
	Action   string `json:"action"`
	Request  string `json:"request,omitempty"`
	Key      string `json:"key,omitempty"`
	Node     string `json:"node,omitempty"`
	Scenario int    `json:"scenario,omitempty"`
}

// Event is one hop along an edge (Dir "req" or "res"), or a log line when To is empty.
// T and Dur are simulated milliseconds; events are sorted by T.
type Event struct {
	T     int    `json:"t"`
	Dur   int    `json:"dur,omitempty"`
	From  string `json:"from"`
	To    string `json:"to,omitempty"`
	Dir   string `json:"dir,omitempty"`
	Async bool   `json:"async,omitempty"`
	Fail  bool   `json:"fail,omitempty"`
	Text  string `json:"text"`
}

type Result struct {
	Status  int     `json:"status,omitempty"` // HTTP status a send ended with
	TotalMS int     `json:"total_ms"`
	Events  []Event `json:"events"`
	State   State   `json:"state"`
}

// Engine limits: no run of a valid spec comes near them; they bound what a crafted state can cost.
const (
	maxEvents    = 2000
	maxCallDepth = 24
	maxBacklog   = 50
	maxStateKeys = 500
)

type world struct {
	down     map[string]bool
	cache    map[string]map[string]bool
	counters map[string]map[string]int
	rr       map[string]int
	backlog  map[string][]string
}

// load turns the viewer's state into a world, dropping anything that doesn't fit the spec
// (a node Claude removed, an oversized list) instead of failing.
func load(sp *Spec, st State) *world {
	w := &world{down: map[string]bool{}, cache: map[string]map[string]bool{}, counters: map[string]map[string]int{}, rr: map[string]int{}, backlog: map[string][]string{}}
	kind := func(id string) string {
		if n := sp.Node(id); n != nil {
			return n.Kind
		}
		return ""
	}
	for _, id := range st.Down {
		if kind(id) != "" {
			w.down[id] = true
		}
	}
	for id, keys := range st.Cache {
		if kind(id) == "cache" {
			w.cache[id] = map[string]bool{}
			for _, k := range keys[:min(len(keys), maxStateKeys)] {
				if runes(k) <= 60 {
					w.cache[id][k] = true
				}
			}
		}
	}
	for id, cs := range st.Counters {
		if kind(id) == "cache" && len(cs) <= maxStateKeys {
			w.counters[id] = map[string]int{}
			for k, c := range cs {
				if runes(k) <= 60 {
					w.counters[id][k] = max(0, min(c, 1<<20))
				}
			}
		}
	}
	for id, c := range st.RR {
		if kind(id) != "" {
			w.rr[id] = max(0, min(c, 1<<20))
		}
	}
	for id, keys := range st.Backlog {
		if kind(id) == "queue" {
			for _, k := range keys[:min(len(keys), maxBacklog)] {
				if runes(k) <= 60 {
					w.backlog[id] = append(w.backlog[id], k)
				}
			}
		}
	}
	return w
}

func (w *world) export() State {
	st := State{Down: []string{}, Cache: map[string][]string{}, Counters: w.counters, RR: w.rr, Backlog: map[string][]string{}}
	for id := range w.down {
		st.Down = append(st.Down, id)
	}
	sort.Strings(st.Down)
	for id, keys := range w.cache {
		if len(keys) > 0 {
			var ks []string
			for k := range keys {
				ks = append(ks, k)
			}
			sort.Strings(ks)
			st.Cache[id] = ks
		}
	}
	for id, keys := range w.backlog {
		if len(keys) > 0 {
			st.Backlog[id] = keys
		}
	}
	return st
}

type run struct {
	sp    *Spec
	w     *world
	ev    []Event
	key   string
	async int
	depth int
	err   error
}

// Run plays one action. It never changes sp. Errors mean a bad input or a spec that blows the
// engine's limits, which Validate refuses, so the viewer only sees them for crafted requests.
func Run(sp *Spec, st State, in Input) (*Result, error) {
	r := &run{sp: sp, w: load(sp, st)}
	res := &Result{}
	switch in.Action {
	case "send":
		req := sp.Request(in.Request)
		if req == nil {
			return nil, fmt.Errorf("unknown request %q", in.Request)
		}
		key := in.Key
		if key == "" {
			key = req.Keys[0]
		}
		if !slices.Contains(req.Keys, key) {
			return nil, fmt.Errorf("key %q is not one of request %q's keys: %s", key, req.ID, strings.Join(req.Keys, ", "))
		}
		res.Status, res.TotalMS = r.send(req, key)
	case "kill", "revive":
		ids := sp.members(in.Node)
		if len(ids) == 0 {
			return nil, fmt.Errorf("%q is neither a node nor a group", in.Node)
		}
		for _, id := range ids {
			if in.Action == "kill" {
				r.w.down[id] = true
				if n := sp.Node(id); n.Kind == "cache" && !n.Persistent {
					delete(r.w.cache, id)
					delete(r.w.counters, id)
				}
			} else {
				delete(r.w.down, id)
			}
		}
		if in.Action == "revive" {
			r.drain()
		}
	case "flush":
		n := sp.Node(in.Node)
		if n == nil || n.Kind != "cache" {
			return nil, fmt.Errorf("%q is not a cache", in.Node)
		}
		delete(r.w.cache, n.ID)
		delete(r.w.counters, n.ID)
	case "reset":
		r.w = load(sp, State{})
	case "scenario":
		if in.Scenario < 0 || in.Scenario >= len(sp.Scenarios) {
			return nil, fmt.Errorf("scenario %d does not exist; there are %d", in.Scenario, len(sp.Scenarios))
		}
		return RunScenario(sp, sp.Scenarios[in.Scenario])
	default:
		return nil, fmt.Errorf("unknown action %q; valid: send, kill, revive, flush, reset, scenario", in.Action)
	}
	if r.err != nil {
		return nil, r.err
	}
	sort.SliceStable(r.ev, func(i, j int) bool { return r.ev[i].T < r.ev[j].T })
	res.Events = r.ev
	if res.Events == nil {
		res.Events = []Event{}
	}
	res.State = r.w.export()
	return res, nil
}

// RunScenario starts from a fresh state, optionally warms caches with one send, kills the
// scenario's nodes and sends its request.
func RunScenario(sp *Spec, sc Scenario) (*Result, error) {
	req := sp.Request(sc.Request)
	if req == nil {
		return nil, fmt.Errorf("unknown request %q", sc.Request)
	}
	send := Input{Action: "send", Request: req.ID, Key: sc.Key}
	st := State{}
	if sc.Warm {
		res, err := Run(sp, st, send)
		if err != nil {
			return nil, err
		}
		st = res.State
	}
	for _, k := range sc.Kill {
		res, err := Run(sp, st, Input{Action: "kill", Node: k})
		if err != nil {
			return nil, err
		}
		st = res.State
	}
	return Run(sp, st, send)
}

// Log is a result as numbered text lines, for simulate_system and validation errors.
func Log(res *Result) []string {
	out := make([]string, 0, len(res.Events))
	for i, e := range res.Events {
		line := fmt.Sprintf("%d. [%d ms] %s", i+1, e.T, e.Text)
		if e.Async {
			line += " (async)"
		}
		out = append(out, line)
	}
	return out
}

func (r *run) name(id string) string { return r.sp.Node(id).Name() }

func (r *run) emit(e Event) {
	if r.err != nil {
		return
	}
	if len(r.ev) >= maxEvents {
		r.err = fmt.Errorf("one action made more than %d hops; reduce fan-out, retries or parallel branches", maxEvents)
		return
	}
	e.Async = e.Async || r.async > 0
	r.ev = append(r.ev, e)
}

func (r *run) note(t int, from, text string) { r.emit(Event{T: t, From: from, Text: text}) }

func (r *run) hop(t, dur int, from, to, text string) {
	r.emit(Event{T: t, Dur: dur, From: from, To: to, Dir: "req", Text: text})
}

func (r *run) reply(t int, from, to, text string, fail bool) {
	r.emit(Event{T: t, Dur: 1, From: from, To: to, Dir: "res", Fail: fail, Text: text})
}

// dead records a hop to a node that is down: the caller gets no answer.
func (r *run) dead(t int, from, to, text string) (string, int, int) {
	r.emit(Event{T: t, Dur: 1, From: from, To: to, Dir: "req", Fail: true, Text: text + " — no answer, " + r.name(to) + " is down"})
	return "down", 502, t + 1
}

func (r *run) send(req *Request, key string) (status, total int) {
	r.key = key
	client := r.sp.client(req)
	label := req.Label
	if label == "" {
		label = req.Handler
	}
	text := fmt.Sprintf("%s → %s: %s", r.name(client), r.name(req.To), r.fill(label))
	out, status, end := r.invoke(client, r.sp.Node(req.To), req.Handler, text, "", 0)
	if out == "ok" {
		status = 200
	}
	r.note(end, client, fmt.Sprintf("%s got %d %s in %d ms", r.name(client), status, http.StatusText(status), end))
	return status, end
}

func (r *run) fill(s string) string { return strings.ReplaceAll(s, "{key}", r.key) }

func (r *run) keyFor(s Step) string {
	if s.Key == "" {
		return r.key
	}
	return r.fill(s.Key)
}

// retryable: like real retry policies, retry connection failures and 5xx, never 4xx (a 409 stays a 409).
func retryable(out string, status int) bool {
	return out == "down" || out == "unavailable" || out == "error" && status >= 500
}

// call runs one call or publish step from node, with its retries.
func (r *run) call(from string, s Step, t int) (out string, status, end int) {
	for attempt := 0; ; attempt++ {
		out, status, end = r.once(from, s, t)
		if attempt >= s.Retry || !retryable(out, status) || r.err != nil {
			return
		}
		target := s.Call + s.Publish
		r.note(end, from, fmt.Sprintf("%s retries %s (attempt %d of %d)", r.name(from), r.name(target), attempt+2, s.Retry+1))
		t = end
	}
}

func (r *run) once(from string, s Step, t int) (string, int, int) {
	if s.Publish != "" {
		return r.publish(from, s, t)
	}
	n := r.sp.Node(s.Call)
	switch n.Kind {
	case "service", "load_balancer":
		text := r.fill(s.Say)
		if text == "" {
			text = fmt.Sprintf("%s → %s: %s", r.name(from), n.Name(), s.Handler)
		}
		return r.invoke(from, n, s.Handler, text, s.Force, t)
	case "cache":
		return r.cacheOp(from, n, s, t)
	}
	return r.dbOp(from, n, s, t)
}

// invoke calls a service or load balancer and returns ok, error (with the status that failed the
// handler), unavailable (no live pool member) or down.
func (r *run) invoke(from string, n *Node, handler, text, force string, t int) (string, int, int) {
	if r.w.down[n.ID] {
		return r.dead(t, from, n.ID, text)
	}
	r.hop(t, n.ms(), from, n.ID, text)
	t1 := t + n.ms()
	if force != "" {
		r.reply(t1, n.ID, from, fmt.Sprintf("%s → %s: %s", n.Name(), r.name(from), force), defaultDo(force) == "fail")
		return force, failStatus(force, 0), t1 + 1
	}
	if n.Kind == "load_balancer" {
		m, why := r.pick(n.ID, n.Pool, n.Strategy == "hash")
		if m == "" {
			r.reply(t1, n.ID, from, n.Name()+": no live servers in the pool → 503", true)
			return "unavailable", 503, t1 + 1
		}
		mn := r.sp.Node(m)
		out, status, end := r.invoke(n.ID, mn, handler, fmt.Sprintf("%s → %s (%s)", n.Name(), mn.Name(), why), "", t1)
		r.reply(end, n.ID, from, r.answer(n.ID, from, out, status), out != "ok")
		return out, status, end + 1
	}
	r.depth++
	defer func() { r.depth-- }()
	if r.depth > maxCallDepth {
		r.err = fmt.Errorf("calls nest more than %d deep", maxCallDepth)
		return "error", 500, t1
	}
	res, status, end := r.steps(n.ID, r.sp.Handlers[handler], t1)
	if res == "fail" {
		r.reply(end, n.ID, from, r.answer(n.ID, from, "error", status), true)
		return "error", status, end + 1
	}
	r.reply(end, n.ID, from, r.answer(n.ID, from, "ok", 0), false)
	return "ok", 0, end + 1
}

func (r *run) answer(from, to, out string, status int) string {
	if out == "ok" {
		return fmt.Sprintf("%s → %s: OK", r.name(from), r.name(to))
	}
	return fmt.Sprintf("%s → %s: %d %s", r.name(from), r.name(to), status, http.StatusText(status))
}

func failStatus(out string, nested int) int {
	if defaultDo(out) != "fail" {
		return 0
	}
	if nested != 0 {
		return nested
	}
	return statusOf(out)
}

// pick chooses a live node from list: round robin (a counter per chooser) or by a hash of the key.
// why says how, and which members were skipped because they are down.
func (r *run) pick(chooser string, list []string, hash bool) (string, string) {
	var live, skipped []string
	for _, id := range list {
		if r.w.down[id] {
			skipped = append(skipped, r.name(id))
		} else {
			live = append(live, id)
		}
	}
	if len(live) == 0 {
		return "", ""
	}
	var chosen, why string
	if hash {
		h := fnv.New32a()
		h.Write([]byte(r.key))
		chosen, why = live[int(h.Sum32()%uint32(len(live)))], "hash of "+r.key
	} else {
		start := r.w.rr[chooser] % len(list)
		for i := range list {
			if c := list[(start+i)%len(list)]; !r.w.down[c] {
				chosen, r.w.rr[chooser] = c, start+i+1
				break
			}
		}
		why = "round robin"
	}
	if len(skipped) > 0 {
		why += "; skipped " + strings.Join(skipped, ", ") + ": down"
	}
	return chosen, why
}

// steps runs a step list on node. It returns "next" (ran to the end), "return" or "fail" (with a status).
func (r *run) steps(node string, steps []Step, t int) (string, int, int) {
	for _, s := range steps {
		if r.err != nil {
			return "fail", 500, t
		}
		if len(s.Parallel) > 0 {
			if s.Say != "" {
				r.note(t, node, r.fill(s.Say))
			}
			end, failed := t, 0
			for _, br := range s.Parallel {
				res, status, e := r.steps(node, br, t)
				end = max(end, e)
				if res == "fail" && failed == 0 {
					failed = max(status, 1)
				}
			}
			if failed != 0 {
				return "fail", failed, end
			}
			t = end
			continue
		}
		if s.Async {
			r.async++
			r.call(node, s, t)
			r.async--
			continue
		}
		out, status, end := r.call(node, s, t)
		br, ok := s.On[out]
		if !ok {
			br = Branch{Do: defaultDo(out)}
		}
		switch br.Do {
		case "next":
			t = end
		case "return":
			return "return", 0, end
		case "fail":
			if status == 0 {
				status = statusOf(out)
			}
			return "fail", status, end
		default:
			res, st, e := r.steps(node, br.Steps, end)
			if res != "next" {
				return res, st, e
			}
			t = e
		}
	}
	return "next", 0, t
}

var opWord = map[string]string{"get": "GET", "set": "SET", "del": "DEL", "setnx": "SETNX", "incr": "INCR"}

func (r *run) cacheOp(from string, n *Node, s Step, t int) (string, int, int) {
	key := r.keyFor(s)
	text := r.fill(s.Say)
	if text == "" {
		text = fmt.Sprintf("%s → %s: %s %s", r.name(from), n.Name(), opWord[s.Op], key)
	}
	if r.w.down[n.ID] {
		return r.dead(t, from, n.ID, text)
	}
	keys := r.w.cache[n.ID]
	if keys == nil {
		keys = map[string]bool{}
		r.w.cache[n.ID] = keys
	}
	out, what := "ok", ""
	switch {
	case s.Force != "":
		out, what = s.Force, s.Force
	case s.Op == "get":
		out, what = "miss", "MISS for "+key
		if keys[key] {
			out, what = "hit", "HIT for "+key
		}
	case s.Op == "set":
		keys[key], what = true, "stored "+key
	case s.Op == "del":
		delete(keys, key)
		what = "deleted " + key
	case s.Op == "setnx":
		if keys[key] {
			out, what = "taken", key+" is already taken"
		} else {
			keys[key], what = true, "lock acquired on "+key
		}
	case s.Op == "incr":
		cs := r.w.counters[n.ID]
		if cs == nil {
			cs = map[string]int{}
			r.w.counters[n.ID] = cs
		}
		cs[key]++
		what = fmt.Sprintf("%s count %d of %d", key, cs[key], s.Limit)
		if cs[key] > s.Limit {
			out, what = "limited", fmt.Sprintf("%s count %d is over the limit of %d", key, cs[key], s.Limit)
		}
	}
	r.hop(t, n.ms(), from, n.ID, text)
	t1 := t + n.ms()
	r.reply(t1, n.ID, from, n.Name()+": "+what, defaultDo(out) == "fail")
	return out, failStatus(out, 0), t1 + 1
}

func (r *run) dbOp(from string, n *Node, s Step, t int) (string, int, int) {
	key := r.keyFor(s)
	target, via := n, ""
	if s.Op == "read" && len(n.Replicas) > 0 {
		if m, why := r.pick(n.ID, n.Replicas, false); m != "" {
			target, via = r.sp.Node(m), " (replica, "+why+")"
		} else if !r.w.down[n.ID] {
			via = " (all replicas down, reading the primary)"
		}
	}
	text := r.fill(s.Say)
	if text == "" {
		text = fmt.Sprintf("%s → %s: %s %s%s", r.name(from), target.Name(), s.Op, key, via)
	}
	if r.w.down[target.ID] {
		return r.dead(t, from, target.ID, text)
	}
	r.hop(t, target.ms(), from, target.ID, text)
	t1 := t + target.ms()
	out, what := "ok", fmt.Sprintf("returned %s (%d ms)", key, target.ms())
	if s.Op == "write" {
		what = fmt.Sprintf("wrote %s (%d ms)", key, target.ms())
	}
	if s.Force != "" {
		out, what = s.Force, s.Force
	}
	r.reply(t1, target.ID, from, target.Name()+": "+what, defaultDo(out) == "fail")
	return out, failStatus(out, 0), t1 + 1
}

func (r *run) publish(from string, s Step, t int) (string, int, int) {
	q := r.sp.Node(s.Publish)
	key := r.keyFor(s)
	text := r.fill(s.Say)
	if text == "" {
		text = fmt.Sprintf("%s → %s: publish %s", r.name(from), q.Name(), key)
	}
	if r.w.down[q.ID] {
		return r.dead(t, from, q.ID, text)
	}
	r.hop(t, q.ms(), from, q.ID, text)
	t1 := t + q.ms()
	r.reply(t1, q.ID, from, q.Name()+": queued "+key, false)
	r.async++
	r.deliver(q, key, t1)
	r.async--
	return "ok", 0, t1 + 1
}

// deliver hands one message to a live consumer, which runs the queue's handler. With no live
// consumer, or when the handler fails, the message waits in the queue until a revive drains it.
func (r *run) deliver(q *Node, key string, t int) {
	c, why := r.pick(q.ID, q.Consumers, false)
	if c == "" {
		r.enqueue(q, key, t, "no live consumer")
		return
	}
	cn := r.sp.Node(c)
	r.hop(t, cn.ms(), q.ID, c, fmt.Sprintf("%s → %s: deliver %s (%s)", q.Name(), cn.Name(), key, why))
	saved := r.key
	r.key = key
	r.depth++
	res, status, end := r.steps(c, r.sp.Handlers[q.Handler], t+cn.ms())
	r.depth--
	r.key = saved
	if res == "fail" {
		r.reply(end, c, q.ID, fmt.Sprintf("%s → %s: failed (%d)", cn.Name(), q.Name(), status), true)
		r.enqueue(q, key, end+1, "processing failed")
		return
	}
	r.reply(end, c, q.ID, fmt.Sprintf("%s → %s: done, message removed", cn.Name(), q.Name()), false)
}

func (r *run) enqueue(q *Node, key string, t int, why string) {
	if len(r.w.backlog[q.ID]) >= maxBacklog {
		r.note(t, q.ID, fmt.Sprintf("%s: %s and the queue is full, message %s dropped", q.Name(), why, key))
		return
	}
	r.w.backlog[q.ID] = append(r.w.backlog[q.ID], key)
	r.note(t, q.ID, fmt.Sprintf("%s: %s, %s waits in the queue (%d waiting)", q.Name(), why, key, len(r.w.backlog[q.ID])))
}

// drain redelivers every waiting message, after a revive.
func (r *run) drain() {
	t := 0
	for i := range r.sp.Nodes {
		q := &r.sp.Nodes[i]
		msgs := r.w.backlog[q.ID]
		if q.Kind != "queue" || len(msgs) == 0 {
			continue
		}
		if !slices.ContainsFunc(q.Consumers, func(c string) bool { return !r.w.down[c] }) {
			continue // still no consumer: leave them waiting, quietly
		}
		delete(r.w.backlog, q.ID)
		for _, k := range msgs {
			r.deliver(q, k, t)
			t += 2
		}
	}
}
