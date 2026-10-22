package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type Block struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"-"`
	Position  string          `json:"-"`
	CreatedAt time.Time       `json:"-"`
	UpdatedAt time.Time       `json:"-"`
}

func (b Block) MarshalJSON() ([]byte, error) {
	head, _ := json.Marshal(struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}{b.ID, b.Type})
	if len(b.Data) <= 2 {
		return head, nil
	}
	return append(append(head[:len(head)-1], ','), b.Data[1:]...), nil
}

type data interface {
	validate() error
	doc() map[string]any
	summary() string
}

var registry = map[string]func() data{
	"heading":    func() data { return &Heading{} },
	"paragraph":  func() data { return &Paragraph{} },
	"bullets":    func() data { return &List{} },
	"numbered":   func() data { return &List{} },
	"callout":    func() data { return &Callout{} },
	"code":       func() data { return &Code{} },
	"table":      func() data { return &Table{} },
	"qa":         func() data { return &QA{} },
	"fill_in":    func() data { return &FillIn{} },
	"spacer":     func() data { return &Spacer{} },
	"diagram":    func() data { return &Diagram{} },
	"two_column": func() data { return &TwoColumn{} },
	"divider":    func() data { return &Empty{} },
	"page_break": func() data { return &Empty{} },
}

func Types() []string {
	var ts []string
	for t := range registry {
		ts = append(ts, t)
	}
	slices.Sort(ts)
	return ts
}

func fieldNames(v any) []string {
	var names []string
	t := reflect.TypeOf(v).Elem()
	for i := range t.NumField() {
		names = append(names, strings.Split(t.Field(i).Tag.Get("json"), ",")[0])
	}
	return names
}

func Parse(raw json.RawMessage) (Block, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Block{}, fmt.Errorf("block must be a JSON object: %v", err)
	}
	var typ string
	json.Unmarshal(fields["type"], &typ)
	mk, ok := registry[typ]
	if !ok {
		return Block{}, fmt.Errorf("unknown or missing type %q; valid types: %s", typ, strings.Join(Types(), ", "))
	}
	delete(fields, "type")
	delete(fields, "id")
	d := mk()
	stripped, _ := json.Marshal(fields)
	dec := json.NewDecoder(bytes.NewReader(stripped))
	dec.DisallowUnknownFields()
	if err := dec.Decode(d); err != nil {
		return Block{Type: typ}, decodeErr(err, d)
	}
	if err := d.validate(); err != nil {
		return Block{Type: typ}, err
	}
	canon, _ := json.Marshal(d)
	return Block{Type: typ, Data: canon}, nil
}

func decodeErr(err error, d data) error {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return fmt.Errorf("field %q must be %s, got %s", te.Field, jsonKind(te.Type), te.Value)
	}
	if msg, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		allowed := fieldNames(d)
		if len(allowed) == 0 {
			return fmt.Errorf("unknown field %s; this type takes no fields besides type", msg)
		}
		return fmt.Errorf("unknown field %s; allowed fields: %s", msg, strings.Join(allowed, ", "))
	}
	return err
}

func jsonKind(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Int, reflect.Int64:
		return "an integer"
	case reflect.Bool:
		return "a boolean"
	case reflect.Slice:
		return "a list of " + strings.TrimPrefix(jsonKind(t.Elem()), "a ")
	case reflect.Struct:
		return "an object"
	}
	return t.String()
}

func (b Block) decode() data {
	d := registry[b.Type]()
	json.Unmarshal(b.Data, d)
	return d
}

func (b Block) Doc() map[string]any {
	m := b.decode().doc()
	m["id"] = b.ID
	m["type"] = b.Type
	return m
}

func (b Block) Summary() string {
	s := strings.Join(strings.Fields(b.decode().summary()), " ")
	if utf8.RuneCountInString(s) > 80 {
		s = string([]rune(s)[:77]) + "..."
	}
	return s
}

func plain(s string) string {
	var b strings.Builder
	for _, sp := range ParseInline(s) {
		b.WriteString(sp.Text)
	}
	return b.String()
}

func required(name, v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("field %q is required and must not be empty", name)
	}
	return nil
}

type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
}

func (h *Heading) validate() error {
	if h.Level < 1 || h.Level > 3 {
		return fmt.Errorf("level must be 1, 2 or 3, got %d", h.Level)
	}
	return required("text", h.Text)
}
func (h *Heading) doc() map[string]any {
	return map[string]any{"level": h.Level, "text": ParseInline(h.Text)}
}
func (h *Heading) summary() string { return fmt.Sprintf("H%d %s", h.Level, plain(h.Text)) }

type Paragraph struct {
	Text string `json:"text"`
}

func (p *Paragraph) validate() error {
	if n := utf8.RuneCountInString(p.Text); n > 2000 {
		return fmt.Errorf("text is %d chars, max 2000; split it into several paragraphs", n)
	}
	return required("text", p.Text)
}
func (p *Paragraph) doc() map[string]any { return map[string]any{"text": ParseInline(p.Text)} }
func (p *Paragraph) summary() string     { return plain(p.Text) }

type Item struct {
	Text     string `json:"text"`
	Children []Item `json:"children,omitempty"`
}

type List struct {
	Items []Item `json:"items"`
}

func (l *List) validate() error {
	if len(l.Items) == 0 {
		return errors.New(`field "items" must contain at least one item`)
	}
	count := 0
	var walk func(items []Item, depth int, path string) error
	walk = func(items []Item, depth int, path string) error {
		for i, it := range items {
			p := fmt.Sprintf("%s%d", path, i)
			if depth > 3 {
				return fmt.Errorf("item %s is nested %d levels deep, max 3", p, depth)
			}
			count++
			if strings.TrimSpace(it.Text) == "" {
				return fmt.Errorf("item %s has empty text", p)
			}
			if err := walk(it.Children, depth+1, p+"."); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(l.Items, 1, ""); err != nil {
		return err
	}
	if count > 40 {
		return fmt.Errorf("list has %d items in total, max 40; split it into several lists", count)
	}
	return nil
}
func docItems(items []Item) []map[string]any {
	out := []map[string]any{}
	for _, it := range items {
		out = append(out, map[string]any{"text": ParseInline(it.Text), "children": docItems(it.Children)})
	}
	return out
}
func (l *List) doc() map[string]any { return map[string]any{"items": docItems(l.Items)} }
func (l *List) summary() string {
	return fmt.Sprintf("%d items: %s", len(l.Items), plain(l.Items[0].Text))
}

var calloutVariants = []string{"note", "tip", "warning", "definition", "key_point"}

type Callout struct {
	Variant string   `json:"variant"`
	Title   string   `json:"title,omitempty"`
	Body    []string `json:"body"`
}

func (c *Callout) validate() error {
	if !slices.Contains(calloutVariants, c.Variant) {
		return fmt.Errorf("variant must be one of %s, got %q", strings.Join(calloutVariants, ", "), c.Variant)
	}
	if len(c.Body) == 0 {
		return errors.New(`field "body" must be a list with at least one paragraph string`)
	}
	return nil
}
func (c *Callout) doc() map[string]any {
	body := [][]Span{}
	for _, p := range c.Body {
		body = append(body, ParseInline(p))
	}
	return map[string]any{"variant": c.Variant, "title": ParseInline(c.Title), "body": body}
}
func (c *Callout) summary() string {
	if c.Title != "" {
		return c.Variant + ": " + plain(c.Title)
	}
	return c.Variant + ": " + plain(c.Body[0])
}

type Code struct {
	Language string `json:"language"`
	Code     string `json:"code"`
	Caption  string `json:"caption,omitempty"`
}

func (c *Code) validate() error {
	if err := required("code", c.Code); err != nil {
		return err
	}
	if n := strings.Count(strings.TrimRight(c.Code, "\n"), "\n") + 1; n > 80 {
		return fmt.Errorf("code has %d lines, max 80; split it into several code blocks", n)
	}
	return nil
}
func (c *Code) doc() map[string]any {
	return map[string]any{"language": c.Language, "code": strings.TrimRight(c.Code, "\n"), "caption": ParseInline(c.Caption)}
}
func (c *Code) summary() string {
	first, _, _ := strings.Cut(strings.TrimSpace(c.Code), "\n")
	return fmt.Sprintf("code(%s): %s", c.Language, first)
}

type Table struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
	Caption string     `json:"caption,omitempty"`
	Align   []string   `json:"align,omitempty"`
}

var alignments = []string{"left", "center", "right"}

var numericCell = regexp.MustCompile(`^[-+]?[$€£₹]?\d[\d,]*(\.\d+)?\s*[%x×]?$`)

func (t *Table) alignments() []string {
	if len(t.Align) > 0 {
		return t.Align
	}
	out := make([]string, len(t.Columns))
	for c := range t.Columns {
		out[c] = "left"
		numbers := 0
		for _, r := range t.Rows {
			cell := strings.TrimSpace(plain(r[c]))
			if numericCell.MatchString(cell) {
				numbers++
			} else if cell != "" && cell != "-" && cell != "—" {
				numbers = -1
				break
			}
		}
		if numbers > 0 {
			out[c] = "right"
		}
	}
	return out
}

func (t *Table) validate() error {
	if n := len(t.Columns); n < 2 || n > 6 {
		return fmt.Errorf("columns has %d entries, must be 2-6", n)
	}
	if len(t.Rows) == 0 {
		return errors.New(`field "rows" must contain at least one row`)
	}
	if len(t.Rows) > 25 {
		return fmt.Errorf("table has %d rows, max 25; split it into several tables", len(t.Rows))
	}
	for i, r := range t.Rows {
		if len(r) != len(t.Columns) {
			return fmt.Errorf("row %d has %d cells, expected %d (columns: %s)", i, len(r), len(t.Columns), strings.Join(t.Columns, ", "))
		}
	}
	if len(t.Align) > 0 && len(t.Align) != len(t.Columns) {
		return fmt.Errorf("align has %d entries, expected one per column (%d)", len(t.Align), len(t.Columns))
	}
	for i, a := range t.Align {
		if !slices.Contains(alignments, a) {
			return fmt.Errorf(`align[%d] must be "left", "center" or "right", got %q`, i, a)
		}
	}
	return nil
}
func (t *Table) doc() map[string]any {
	cols := [][]Span{}
	for _, c := range t.Columns {
		cols = append(cols, ParseInline(c))
	}
	rows := [][][]Span{}
	for _, r := range t.Rows {
		row := [][]Span{}
		for _, c := range r {
			row = append(row, ParseInline(c))
		}
		rows = append(rows, row)
	}
	return map[string]any{"columns": cols, "rows": rows, "caption": ParseInline(t.Caption), "align": t.alignments()}
}
func (t *Table) summary() string {
	return fmt.Sprintf("table: %dx%d %s", len(t.Columns), len(t.Rows), strings.Join(t.Columns, "/"))
}

type QA struct {
	Question   string `json:"question"`
	Answer     string `json:"answer"`
	HideAnswer bool   `json:"hide_answer,omitempty"`
}

func (q *QA) validate() error {
	if err := required("question", q.Question); err != nil {
		return err
	}
	return required("answer", q.Answer)
}
func (q *QA) doc() map[string]any {
	return map[string]any{"question": ParseInline(q.Question), "answer": ParseInline(q.Answer), "hide_answer": q.HideAnswer}
}
func (q *QA) summary() string { return "Q: " + plain(q.Question) }

type FillIn struct {
	Prompt string `json:"prompt"`
	Lines  int    `json:"lines"`
}

func (f *FillIn) validate() error {
	if f.Lines < 1 || f.Lines > 15 {
		return fmt.Errorf("lines must be 1-15, got %d", f.Lines)
	}
	return required("prompt", f.Prompt)
}
func (f *FillIn) doc() map[string]any {
	return map[string]any{"prompt": ParseInline(f.Prompt), "lines": f.Lines}
}
func (f *FillIn) summary() string {
	return fmt.Sprintf("fill in (%d lines): %s", f.Lines, plain(f.Prompt))
}

type Spacer struct {
	Size string `json:"size"`
}

func (s *Spacer) validate() error {
	if !slices.Contains([]string{"small", "medium", "large"}, s.Size) {
		return fmt.Errorf(`size must be "small", "medium" or "large", got %q`, s.Size)
	}
	return nil
}
func (s *Spacer) doc() map[string]any { return map[string]any{"size": s.Size} }
func (s *Spacer) summary() string     { return "spacer " + s.Size }

type Diagram struct {
	Source  string `json:"source"`
	Caption string `json:"caption,omitempty"`
	Size    string `json:"size,omitempty"`
}

func (d *Diagram) validate() error {
	if d.Size == "" {
		d.Size = "full"
	}
	if !slices.Contains([]string{"full", "half", "small"}, d.Size) {
		return fmt.Errorf(`size must be "full", "half" or "small", got %q`, d.Size)
	}
	return required("source", d.Source)
}
func (d *Diagram) doc() map[string]any {
	return map[string]any{"source": d.Source, "size": d.Size, "caption": ParseInline(d.Caption)}
}
func (d *Diagram) summary() string {
	nodes := map[string]bool{}
	for _, line := range strings.Split(d.Source, "\n") {
		for _, part := range strings.Split(line, "->") {
			name, _, _ := strings.Cut(strings.TrimSpace(part), ":")
			if name = strings.TrimSpace(strings.Trim(name, "<>-{}")); name != "" && !strings.ContainsAny(name, " .") {
				nodes[name] = true
			}
		}
	}
	if d.Caption != "" {
		return fmt.Sprintf("diagram: ~%d nodes, %s", len(nodes), plain(d.Caption))
	}
	return fmt.Sprintf("diagram: ~%d nodes", len(nodes))
}

func (b Block) DiagramSource() (src, size string, ok bool) {
	if b.Type != "diagram" {
		return "", "", false
	}
	d := b.decode().(*Diagram)
	return d.Source, d.Size, true
}

var nestable = []string{"heading", "paragraph", "bullets", "numbered", "callout", "code", "diagram", "qa"}

var validID = regexp.MustCompile(`^b_[a-z2-7]{8}$`)

type TwoColumn struct {
	Left  []json.RawMessage `json:"left"`
	Right []json.RawMessage `json:"right"`
}

func (tc *TwoColumn) validate() error {
	for _, side := range []struct {
		name string
		raws *[]json.RawMessage
	}{{"left", &tc.Left}, {"right", &tc.Right}} {
		if len(*side.raws) == 0 {
			return fmt.Errorf("%s must contain at least one block", side.name)
		}
		for i, raw := range *side.raws {
			b, err := Parse(raw)
			if err != nil {
				return fmt.Errorf("%s[%d] (%s): %v", side.name, i, b.Type, err)
			}
			if !slices.Contains(nestable, b.Type) {
				return fmt.Errorf("%s[%d]: type %s is not allowed inside two_column; allowed: %s", side.name, i, b.Type, strings.Join(nestable, ", "))
			}
			if b.Type == "heading" && b.decode().(*Heading).Level != 3 {
				return fmt.Errorf("%s[%d] (heading): only level 3 headings are allowed inside two_column", side.name, i)
			}
			if src, size, _ := b.DiagramSource(); src != "" && size == "full" {
				return fmt.Errorf(`%s[%d] (diagram): size must be "half" or "small" inside two_column`, side.name, i)
			}
			var head struct {
				ID string `json:"id"`
			}
			json.Unmarshal(raw, &head)
			b.ID = head.ID
			if !validID.MatchString(b.ID) {
				b.ID = NewID("b_")
			}
			(*side.raws)[i], _ = json.Marshal(b)
		}
	}
	return nil
}

func (tc *TwoColumn) children() (left, right []Block) {
	conv := func(raws []json.RawMessage) []Block {
		var out []Block
		for _, raw := range raws {
			var fields map[string]json.RawMessage
			json.Unmarshal(raw, &fields)
			var b Block
			json.Unmarshal(fields["id"], &b.ID)
			json.Unmarshal(fields["type"], &b.Type)
			delete(fields, "id")
			delete(fields, "type")
			b.Data, _ = json.Marshal(fields)
			out = append(out, b)
		}
		return out
	}
	return conv(tc.Left), conv(tc.Right)
}

func (tc *TwoColumn) doc() map[string]any {
	l, r := tc.children()
	docs := func(bs []Block) []map[string]any {
		out := []map[string]any{}
		for _, b := range bs {
			out = append(out, b.Doc())
		}
		return out
	}
	return map[string]any{"left": docs(l), "right": docs(r)}
}

func (tc *TwoColumn) summary() string {
	l, r := tc.children()
	return fmt.Sprintf("two columns: %d left / %d right; %s | %s", len(l), len(r), l[0].Summary(), r[0].Summary())
}

func (b Block) Children() []Block {
	if b.Type != "two_column" {
		return nil
	}
	l, r := b.decode().(*TwoColumn).children()
	return append(l, r...)
}

func (b Block) WithChildren(edit func(side string, kids []Block) []Block) Block {
	tc := b.decode().(*TwoColumn)
	l, r := tc.children()
	enc := func(bs []Block) []json.RawMessage {
		out := []json.RawMessage{}
		for _, k := range bs {
			js, _ := json.Marshal(k)
			out = append(out, js)
		}
		return out
	}
	tc.Left, tc.Right = enc(edit("left", l)), enc(edit("right", r))
	b.Data, _ = json.Marshal(tc)
	return b
}

func (b Block) RenewIDs(keep func(id string) bool) Block {
	if b.Type != "two_column" {
		return b
	}
	return b.WithChildren(func(_ string, kids []Block) []Block {
		for i := range kids {
			if !keep(kids[i].ID) {
				kids[i].ID = NewID("b_")
			}
		}
		return kids
	})
}

func (b Block) DiagramSources() []string {
	if src, _, ok := b.DiagramSource(); ok {
		return []string{src}
	}
	var out []string
	for _, k := range b.Children() {
		out = append(out, k.DiagramSources()...)
	}
	return out
}

type Empty struct{}

func (*Empty) validate() error     { return nil }
func (*Empty) doc() map[string]any { return map[string]any{} }
func (*Empty) summary() string     { return "" }

func ParseAll(raws []json.RawMessage) ([]Block, error) {
	blocks := make([]Block, len(raws))
	for i, raw := range raws {
		b, err := Parse(raw)
		if err != nil {
			if b.Type == "" {
				return nil, fmt.Errorf("block[%d]: %v", i, err)
			}
			return nil, fmt.Errorf("block[%d] (%s): %v", i, b.Type, err)
		}
		blocks[i] = b
	}
	return blocks, nil
}
