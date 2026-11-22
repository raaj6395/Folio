package sim

import (
	"encoding/json"
	"math/rand/v2"
	"strings"
	"testing"
)

func demo(t *testing.T, name string) *Spec {
	t.Helper()
	for _, d := range Demos() {
		if d.Name == name {
			sp, err := Decode(d.JSON)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Validate(sp); err != nil {
				t.Fatal(err)
			}
			return sp
		}
	}
	t.Fatalf("no demo %s", name)
	return nil
}

func TestDemosValidWithoutWarnings(t *testing.T) {
	ds := Demos()
	if len(ds) != 3 {
		t.Fatalf("want 3 demos, got %d", len(ds))
	}
	for _, d := range ds {
		sp, err := Decode(d.JSON)
		if err != nil {
			t.Fatal(d.Name, err)
		}
		sum, err := Validate(sp)
		if err != nil {
			t.Fatal(d.Name, err)
		}
		if len(sum.Warnings) > 0 || d.Title == "" {
			t.Errorf("%s: warnings %v, title %q", d.Name, sum.Warnings, d.Title)
		}
	}
}

func send(t *testing.T, sp *Spec, st State, req, key string) *Result {
	t.Helper()
	res, err := Run(sp, st, Input{Action: "send", Request: req, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// The hand trace of the read path, cold then warm (M0).
func TestReadPathGolden(t *testing.T) {
	sp := demo(t, "read_path")
	res := send(t, sp, State{}, "read", "7")
	want := `1. [0 ms] Browser → Load Balancer: GET /products/7
2. [1 ms] Load Balancer → App Server 1 (round robin)
3. [6 ms] App Server 1 → Redis: GET product:7
4. [7 ms] Redis: MISS for product:7
5. [8 ms] App Server 1 → Postgres: read product:7
6. [38 ms] Postgres: returned product:7 (30 ms)
7. [39 ms] App Server 1 → Redis: SET product:7 (async)
8. [39 ms] App Server 1 → Load Balancer: OK
9. [40 ms] Redis: stored product:7 (async)
10. [40 ms] Load Balancer → Browser: OK
11. [41 ms] Browser got 200 OK in 41 ms`
	if got := strings.Join(Log(res), "\n"); got != want {
		t.Fatalf("cold read:\n%s", got)
	}
	warm := send(t, sp, res.State, "read", "7")
	if warm.Status != 200 || warm.TotalMS != 10 || Visited(warm.Events)["pg"] || !strings.Contains(warm.Events[1].Text, "App Server 2") {
		t.Fatalf("warm read should hit Redis on App Server 2 in 10 ms:\n%s", strings.Join(Log(warm), "\n"))
	}
	// Killing Redis empties it, so a revived Redis misses again.
	st := warm.State
	for _, a := range []string{"kill", "revive"} {
		r, err := Run(sp, st, Input{Action: a, Node: "redis"})
		if err != nil {
			t.Fatal(err)
		}
		st = r.State
	}
	if again := send(t, sp, st, "read", "7"); !Visited(again.Events)["pg"] {
		t.Fatal("a revived Redis should be cold")
	}
}

func TestBothAppServersDown503(t *testing.T) {
	sp := demo(t, "read_path")
	res := send(t, sp, State{Down: []string{"app1", "app2"}}, "read", "7")
	if res.Status != 503 || !strings.Contains(strings.Join(Log(res), "\n"), "no live servers") {
		t.Fatalf("%d\n%s", res.Status, strings.Join(Log(res), "\n"))
	}
}

func TestQueueWaitsAndDrainsOnRevive(t *testing.T) {
	sp := demo(t, "ticket_booking")
	res, err := RunScenario(sp, sp.Scenarios[1]) // mailers down
	if err != nil || res.Status != 200 || len(res.State.Backlog["email_q"]) != 1 {
		t.Fatalf("booking with mailers down: %v %d %v", err, res.Status, res.State.Backlog)
	}
	rev, err := Run(sp, res.State, Input{Action: "revive", Node: "mailer2"})
	if err != nil {
		t.Fatal(err)
	}
	log := strings.Join(Log(rev), "\n")
	if len(rev.State.Backlog) != 0 || !strings.Contains(log, "Email queue → Mailer 2: deliver booking:12A") || !strings.Contains(log, "done, message removed") {
		t.Fatalf("revive should drain the queue:\n%s", log)
	}
	// A failing consumer puts the message back.
	fail := send(t, sp, State{Down: []string{"email_api"}}, "book", "14C")
	if fail.Status != 200 || len(fail.State.Backlog["email_q"]) != 1 || !strings.Contains(strings.Join(Log(fail), "\n"), "goes back") && !strings.Contains(strings.Join(Log(fail), "\n"), "waits in the queue") {
		t.Fatalf("failed delivery should requeue:\n%s", strings.Join(Log(fail), "\n"))
	}
}

func TestRetryOnlyOn5xx(t *testing.T) {
	sp := demo(t, "ticket_booking")
	taken := send(t, sp, send(t, sp, State{}, "book", "12B").State, "book", "12B")
	if taken.Status != 409 || strings.Contains(strings.Join(Log(taken), "\n"), "retries") {
		t.Fatalf("409 must not be retried:\n%s", strings.Join(Log(taken), "\n"))
	}
	down := send(t, sp, State{Down: []string{"redis"}}, "book", "12B")
	if !strings.Contains(strings.Join(Log(down), "\n"), "retries Booking service (attempt 2 of 2)") {
		t.Fatalf("502 should be retried once:\n%s", strings.Join(Log(down), "\n"))
	}
}

func TestRateLimit429(t *testing.T) {
	sp := demo(t, "url_shortener")
	st := State{}
	var res *Result
	for range 6 {
		res = send(t, sp, st, "create", "abc123")
		st = res.State
	}
	if res.Status != 429 {
		t.Fatalf("6th create should be rate limited, got %d", res.Status)
	}
	f, _ := Run(sp, st, Input{Action: "flush", Node: "redis"})
	if send(t, sp, f.State, "create", "abc123").Status != 200 {
		t.Fatal("flushing Redis resets the counter")
	}
}

func TestHashMovesKeyWhenMemberDies(t *testing.T) {
	sp := demo(t, "url_shortener")
	before := send(t, sp, State{}, "redirect", "xyz789").Events[1].To
	after := send(t, sp, State{Down: []string{before}}, "redirect", "xyz789").Events[1].To
	again := send(t, sp, State{}, "redirect", "xyz789").Events[1].To
	if before == after || before != again {
		t.Fatalf("hash: %s, with it down %s, again %s", before, after, again)
	}
}

func mutate(t *testing.T, name string, edit func(m map[string]any)) error {
	t.Helper()
	var m map[string]any
	for _, d := range Demos() {
		if d.Name == name {
			json.Unmarshal(d.JSON, &m)
		}
	}
	edit(m)
	js, _ := json.Marshal(m)
	sp, err := Decode(js)
	if err != nil {
		return err
	}
	_, err = Validate(sp)
	return err
}

func node(m map[string]any, i int) map[string]any { return m["nodes"].([]any)[i].(map[string]any) }
func handler(m map[string]any, name string, i int) map[string]any {
	return m["handlers"].(map[string]any)[name].([]any)[i].(map[string]any)
}

func TestValidationMessages(t *testing.T) {
	cases := []struct {
		name string
		edit func(m map[string]any)
		want string
	}{
		{"unknown field", func(m map[string]any) { node(m, 1)["pools"] = []any{"app1"} }, `nodes[1]: unknown field "pools"; allowed: id, kind`},
		{"unknown nested field", func(m map[string]any) {
			handler(m, "get_product", 0)["on"].(map[string]any)["hit"] = []any{map[string]any{"cal": "pg"}}
		}, `handlers.get_product[0].on.hit[0]: unknown field "cal"`},
		{"wrong type", func(m map[string]any) { node(m, 1)["pool"] = "app1" }, `must be a list, got string`},
		{"typo node", func(m map[string]any) { handler(m, "get_product", 1)["call"] = "pgg" }, `handlers.get_product[1].call: unknown node "pgg" (did you mean "pg"?)`},
		{"bad op", func(m map[string]any) { handler(m, "get_product", 1)["op"] = "select" }, `a database takes op read, write; got "select"`},
		{"bad outcome", func(m map[string]any) { handler(m, "get_product", 1)["on"] = map[string]any{"miss": "next"} }, `"miss" is not a possible outcome here; valid: ok, down`},
		{"pool of caches", func(m map[string]any) { node(m, 1)["pool"] = []any{"redis"} }, `nodes[1].pool[0]: "redis" is a cache; expected service`},
		{"field on wrong kind", func(m map[string]any) { node(m, 4)["pool"] = []any{"app1"} }, `nodes[4].pool: only a load_balancer takes pool`},
		{"call a queue", func(m map[string]any) { handler(m, "get_product", 1)["call"] = "client" }, `"client" is a client; clients send requests but can't be called`},
		{"duplicate id", func(m map[string]any) { node(m, 3)["id"] = "app1" }, `nodes[3].id: "app1" is used by another node`},
		{"unknown handler", func(m map[string]any) { m["requests"].([]any)[0].(map[string]any)["handler"] = "get_prodct" }, `unknown handler "get_prodct" (did you mean "get_product"?)`},
		{"async with on", func(m map[string]any) { handler(m, "get_product", 2)["on"] = map[string]any{"ok": "next"} }, "an async step doesn't wait for its outcome"},
		{"several problems at once", func(m map[string]any) { node(m, 1)["kind"] = "lb"; node(m, 2)["icon"] = "rocket" }, "2 problems:"},
		{"scenario expect wrong", func(m map[string]any) {
			m["scenarios"].([]any)[1].(map[string]any)["expect"] = map[string]any{"status": 502}
		}, `scenarios[1] "Redis is down": expected status 502, got 200. Fix the handlers or the expect. What happened:`},
		{"link not a connection", func(m map[string]any) {
			m["links"] = []any{map[string]any{"from": "client", "to": "pg", "label": "SQL"}}
		}, "links: client → pg is not a connection"},
		{"loop", func(m map[string]any) {
			m["handlers"].(map[string]any)["get_product"] = []any{map[string]any{"call": "lb", "handler": "get_product"}}
		}, "handlers call each other in a loop"},
	}
	for _, c := range cases {
		err := mutate(t, "read_path", c.edit)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q in error, got %v", c.name, c.want, err)
		}
	}
	if err := mutate(t, "read_path", func(m map[string]any) { delete(m, "engine") }); err != nil {
		t.Errorf("engine defaults to 1: %v", err)
	}
}

func TestSpecRoundTrips(t *testing.T) {
	for _, d := range Demos() {
		sp, _ := Decode(d.JSON)
		js, _ := json.Marshal(sp)
		again, err := Decode(js)
		if err != nil {
			t.Fatal(err)
		}
		js2, _ := json.Marshal(again)
		if string(js) != string(js2) {
			t.Fatalf("%s doesn't round-trip", d.Name)
		}
	}
}

// Random kill/revive/flush/send sequences: every action finishes, events are in time order and
// every hop sits on an edge the viewer draws.
func TestFuzzActions(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for _, d := range Demos() {
		sp := demo(t, d.Name)
		edges, _, _ := Walk(sp)
		drawn := map[string]bool{}
		for _, e := range edges {
			drawn[e.From+">"+e.To], drawn[e.To+">"+e.From] = true, true
		}
		var names []string
		for _, n := range sp.Nodes {
			names = append(names, n.ID)
			if n.Group != "" {
				names = append(names, n.Group)
			}
		}
		st := State{}
		for i := range 3000 {
			var in Input
			switch rng.IntN(6) {
			case 0:
				in = Input{Action: "kill", Node: names[rng.IntN(len(names))]}
			case 1:
				in = Input{Action: "revive", Node: names[rng.IntN(len(names))]}
			case 2:
				in = Input{Action: "scenario", Scenario: rng.IntN(len(sp.Scenarios))}
			default:
				r := sp.Requests[rng.IntN(len(sp.Requests))]
				in = Input{Action: "send", Request: r.ID, Key: r.Keys[rng.IntN(len(r.Keys))]}
			}
			res, err := Run(sp, st, in)
			if err != nil {
				t.Fatalf("%s step %d %+v: %v", d.Name, i, in, err)
			}
			for j, e := range res.Events {
				if j > 0 && e.T < res.Events[j-1].T {
					t.Fatalf("%s: events out of order", d.Name)
				}
				if e.To != "" && !drawn[e.From+">"+e.To] {
					t.Fatalf("%s: hop %s → %s has no edge (%s)", d.Name, e.From, e.To, e.Text)
				}
			}
			if in.Action == "send" && res.Status == 0 {
				t.Fatalf("%s: send without a status", d.Name)
			}
			st = res.State
		}
	}
}

func TestCraftedStateIsTrimmed(t *testing.T) {
	sp := demo(t, "read_path")
	keys := make([]string, 5000)
	for i := range keys {
		keys[i] = strings.Repeat("k", i%100)
	}
	st := State{Down: []string{"ghost", "app1"}, Cache: map[string][]string{"redis": keys, "pg": {"x"}}, RR: map[string]int{"lb": -5}}
	res, err := Run(sp, st, Input{Action: "send", Request: "read", Key: "7"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.State.Down) != 1 || len(res.State.Cache["redis"]) > maxStateKeys+1 || res.State.Cache["pg"] != nil {
		t.Fatalf("state not trimmed: down %v, %d cached", res.State.Down, len(res.State.Cache["redis"]))
	}
	for _, bad := range []Input{{Action: "send", Request: "nope"}, {Action: "send", Request: "read", Key: "999"}, {Action: "kill", Node: "nope"}, {Action: "flush", Node: "pg"}, {Action: "scenario", Scenario: 99}, {Action: "explode"}} {
		if _, err := Run(sp, State{}, bad); err == nil {
			t.Errorf("%+v should fail", bad)
		}
	}
}
