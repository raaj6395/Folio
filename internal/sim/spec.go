// Package sim is the system notebook: a JSON spec of a system (nodes, handlers, requests) and a
// deterministic engine that plays requests through it. Claude writes the spec; the viewer animates
// what Run returns. All behaviour lives here so validation, simulate_system and the viewer agree.
package sim

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Engine is the newest engine version. A spec stores the version it was written for, so a later
// engine with different behaviour never changes how an old system runs.
const Engine = 1

type Spec struct {
	Engine    int               `json:"engine"`
	Direction string            `json:"direction,omitempty"`
	Nodes     []Node            `json:"nodes"`
	Handlers  map[string][]Step `json:"handlers,omitempty"`
	Requests  []Request         `json:"requests"`
	Scenarios []Scenario        `json:"scenarios,omitempty"`
	Links     []Link            `json:"links,omitempty"`
}

type Node struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Label      string   `json:"label,omitempty"`
	Icon       string   `json:"icon,omitempty"`
	Group      string   `json:"group,omitempty"`
	MS         int      `json:"ms,omitempty"`
	About      string   `json:"about,omitempty"`
	WhenDown   string   `json:"when_down,omitempty"`
	Pool       []string `json:"pool,omitempty"`
	Strategy   string   `json:"strategy,omitempty"`
	Replicas   []string `json:"replicas,omitempty"`
	Consumers  []string `json:"consumers,omitempty"`
	Handler    string   `json:"handler,omitempty"`
	Persistent bool     `json:"persistent,omitempty"`
}

type Step struct {
	Call     string            `json:"call,omitempty"`
	Op       string            `json:"op,omitempty"`
	Key      string            `json:"key,omitempty"`
	Limit    int               `json:"limit,omitempty"`
	Handler  string            `json:"handler,omitempty"`
	Publish  string            `json:"publish,omitempty"`
	Parallel [][]Step          `json:"parallel,omitempty"`
	Async    bool              `json:"async,omitempty"`
	Retry    int               `json:"retry,omitempty"`
	On       map[string]Branch `json:"on,omitempty"`
	Force    string            `json:"force,omitempty"`
	Say      string            `json:"say,omitempty"`
}

// Branch is what a step does after an outcome: "next", "return", "fail", or a list of steps.
type Branch struct {
	Do    string
	Steps []Step
}

func (b *Branch) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &b.Do); err == nil {
		return nil
	}
	if err := json.Unmarshal(data, &b.Steps); err != nil {
		return errors.New(`must be "next", "return", "fail" or a list of steps`)
	}
	return nil
}

func (b Branch) MarshalJSON() ([]byte, error) {
	if b.Do != "" {
		return json.Marshal(b.Do)
	}
	return json.Marshal(b.Steps)
}

type Request struct {
	ID      string   `json:"id"`
	Label   string   `json:"label,omitempty"`
	From    string   `json:"from,omitempty"`
	To      string   `json:"to"`
	Handler string   `json:"handler"`
	Keys    []string `json:"keys"`
}

type Scenario struct {
	Label   string   `json:"label"`
	Kill    []string `json:"kill,omitempty"`
	Request string   `json:"request"`
	Key     string   `json:"key,omitempty"`
	Warm    bool     `json:"warm,omitempty"`
	Expect  *Expect  `json:"expect,omitempty"`
	Explain string   `json:"explain,omitempty"`
}

type Expect struct {
	Status int      `json:"status,omitempty"`
	Visits []string `json:"visits,omitempty"`
	Avoids []string `json:"avoids,omitempty"`
}

type Link struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}

var (
	Kinds      = []string{"client", "load_balancer", "service", "cache", "database", "queue"}
	Icons      = []string{"client", "phone", "load_balancer", "gateway", "server", "worker", "cache", "database", "queue", "cdn", "storage", "search", "auth", "external"}
	cacheOps   = []string{"get", "set", "del", "setnx", "incr"}
	dbOps      = []string{"read", "write"}
	defaultMS  = map[string]int{"client": 0, "load_balancer": 1, "service": 5, "cache": 1, "database": 20, "queue": 2}
	validName  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	branchDos  = []string{"next", "return", "fail"}
	directions = []string{"", "right", "down"}
)

// Limits keep layout, validation and every engine run small.
const (
	maxNodes     = 60
	maxHandlers  = 40
	maxSteps     = 300
	maxDepth     = 5
	maxRequests  = 20
	maxScenarios = 20
	maxKeys      = 10
	MaxSpecBytes = 40 << 10
)

// Decode parses a spec strictly: unknown fields and wrong types are errors that say where they are.
// It does not validate; call Validate for that.
func Decode(data []byte) (*Spec, error) {
	if len(data) > MaxSpecBytes {
		return nil, fmt.Errorf("spec is %d KB, max %d KB; split the system or shorten its text", len(data)>>10, MaxSpecBytes>>10)
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("spec must be a JSON object: %v", err)
	}
	if err := unknownFields(raw, reflect.TypeFor[Spec](), ""); err != nil {
		return nil, err
	}
	var sp Spec
	if err := json.Unmarshal(data, &sp); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return nil, fmt.Errorf("%s: must be %s, got %s", te.Field, kindOf(te.Type), te.Value)
		}
		return nil, errors.New(strings.TrimPrefix(err.Error(), "json: "))
	}
	return &sp, nil
}

func kindOf(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Int:
		return "an integer"
	case reflect.Bool:
		return "true or false"
	case reflect.Slice:
		return "a list"
	case reflect.Map, reflect.Struct:
		return "an object"
	}
	return t.String()
}

// unknownFields walks the decoded JSON next to the Go type and reports the first key the type lacks.
func unknownFields(v any, t reflect.Type, path string) error {
	switch t.Kind() {
	case reflect.Pointer:
		return unknownFields(v, t.Elem(), path)
	case reflect.Slice:
		if l, ok := v.([]any); ok {
			for i, e := range l {
				if err := unknownFields(e, t.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	case reflect.Map:
		if m, ok := v.(map[string]any); ok {
			for k, e := range m {
				if err := unknownFields(e, t.Elem(), join(path, k)); err != nil {
					return err
				}
			}
		}
	case reflect.Struct:
		if t == reflect.TypeFor[Branch]() {
			return unknownFields(v, reflect.TypeFor[[]Step](), path)
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil // json.Unmarshal reports the type error
		}
		fields := map[string]reflect.Type{}
		var names []string
		for i := range t.NumField() {
			name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
			fields[name] = t.Field(i).Type
			names = append(names, name)
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			ft, ok := fields[k]
			if !ok {
				return fmt.Errorf("%s: unknown field %q; allowed: %s", cmpPath(path), k, strings.Join(names, ", "))
			}
			if err := unknownFields(m[k], ft, join(path, k)); err != nil {
				return err
			}
		}
	}
	return nil
}

func join(path, k string) string {
	if path == "" {
		return k
	}
	return path + "." + k
}

func cmpPath(p string) string {
	if p == "" {
		return "spec"
	}
	return p
}

// Node returns the node with this id, or nil.
func (sp *Spec) Node(id string) *Node {
	for i := range sp.Nodes {
		if sp.Nodes[i].ID == id {
			return &sp.Nodes[i]
		}
	}
	return nil
}

func (sp *Spec) Request(id string) *Request {
	for i := range sp.Requests {
		if sp.Requests[i].ID == id {
			return &sp.Requests[i]
		}
	}
	return nil
}

// Name is how a node is written in log lines: its label, else its id.
func (n *Node) Name() string {
	if n.Label != "" {
		return n.Label
	}
	return n.ID
}

func (n *Node) ms() int {
	if n.MS > 0 {
		return n.MS
	}
	return defaultMS[n.Kind]
}

// client is who sends r: its from, else the first client node.
func (sp *Spec) client(r *Request) string {
	if r.From != "" {
		return r.From
	}
	for _, n := range sp.Nodes {
		if n.Kind == "client" {
			return n.ID
		}
	}
	return ""
}

// members are the node ids a kill of name (a node id or a group name) takes down.
func (sp *Spec) members(name string) []string {
	if sp.Node(name) != nil {
		return []string{name}
	}
	var ids []string
	for _, n := range sp.Nodes {
		if n.Group == name {
			ids = append(ids, n.ID)
		}
	}
	return ids
}

// outcomes are the results a step can have, in the order the docs list them.
func (sp *Spec) outcomes(s Step) []string {
	if s.Publish != "" {
		return []string{"ok", "down"}
	}
	n := sp.Node(s.Call)
	if n == nil {
		return nil
	}
	switch n.Kind {
	case "service":
		return []string{"ok", "error", "down"}
	case "load_balancer":
		return []string{"ok", "error", "unavailable", "down"}
	case "cache":
		switch s.Op {
		case "get":
			return []string{"hit", "miss", "down"}
		case "setnx":
			return []string{"ok", "taken", "down"}
		case "incr":
			return []string{"ok", "limited", "down"}
		}
		return []string{"ok", "down"}
	case "database":
		return []string{"ok", "down"}
	}
	return nil
}

// defaultDo is what happens after an outcome the step's on doesn't mention.
func defaultDo(outcome string) string {
	switch outcome {
	case "ok", "hit", "miss":
		return "next"
	}
	return "fail"
}

// statusOf is the HTTP status a request ends with when this outcome fails it.
func statusOf(outcome string) int {
	switch outcome {
	case "taken":
		return 409
	case "limited":
		return 429
	case "unavailable":
		return 503
	case "error":
		return 500
	}
	return 502
}

func runes(s string) int { return utf8.RuneCountInString(s) }
