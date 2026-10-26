// Package notetemplates holds the blueprint Claude follows for each kind of note (a DSA
// problem, an article, a code fix…) and checks a notebook's sections against it.
// Each template is one embedded <id>.md file: a small header, then the text Claude reads.
package notetemplates

import (
	"embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/model"
)

//go:embed *.md
var files embed.FS

type Template struct {
	ID       string
	Name     string
	Label    string // dashboard pill; empty for the default topic template
	UseWhen  string
	Density  string // density create_notebook uses when none is given; empty = the store default
	MaxPages int    // 0 = no limit
	// Required holds groups of H2 headings. A section is checked against the group it
	// matches best, so one template can have several kinds of page (system design parts).
	Required [][]string
	Body     string
}

// order is how templates are offered to Claude: most specific first, the topic default last.
var order = []string{"dsa_problem", "dsa_pattern", "code_solution", "article", "lecture", "comparison", "cheat_sheet", "system_design", "topic"}

var All = load()

func load() []Template {
	var ts []Template
	for _, id := range order {
		raw, err := files.ReadFile(id + ".md")
		if err != nil {
			panic(err)
		}
		ts = append(ts, parse(id, string(raw)))
	}
	if entries, _ := files.ReadDir("."); len(entries) != len(order) {
		panic("notetemplates: every .md file must be listed in order")
	}
	return ts
}

func parse(id, raw string) Template {
	head, body, ok := strings.Cut(strings.TrimPrefix(raw, "---\n"), "\n---\n")
	if !ok {
		panic("notetemplates: " + id + ".md has no header")
	}
	t := Template{ID: id, Body: strings.TrimSpace(body)}
	for _, line := range strings.Split(head, "\n") {
		k, v, _ := strings.Cut(line, ":")
		v = strings.TrimSpace(v)
		switch k {
		case "name":
			t.Name = v
		case "label":
			t.Label = v
		case "use_when":
			t.UseWhen = v
		case "density":
			t.Density = v
		case "max_pages":
			t.MaxPages, _ = strconv.Atoi(v)
		case "required":
			for _, g := range strings.Split(v, "|") {
				t.Required = append(t.Required, splitTrim(g))
			}
		default:
			panic(fmt.Sprintf("notetemplates: %s.md: unknown header line %q", id, line))
		}
	}
	if t.Name == "" || t.UseWhen == "" {
		panic("notetemplates: " + id + ".md needs name and use_when")
	}
	return t
}

func splitTrim(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

func IDs() []string { return order }

func ByID(id string) (Template, bool) {
	for _, t := range All {
		if t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}

// Label is the dashboard pill for a notebook's template ("" for none or topic).
func Label(id string) string {
	t, _ := ByID(id)
	return t.Label
}

// Index lists every template with when to use it, one line each.
func Index() string {
	var b strings.Builder
	for _, t := range All {
		fmt.Fprintf(&b, "- %s: %s\n", t.ID, t.UseWhen)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// Guide is what get_authoring_guide returns for this template.
func (t Template) Guide() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Template: %s (%s)\n", t.Name, t.ID)
	b.WriteString("The user's notebook_guidelines override this template; the layout rules (get_authoring_guide without template) still apply.\n")
	b.WriteString("Page counts below are limits, not permission: still run check_layout and fix every warning (a few lines spilling onto a page, an under-filled page).\n")
	for _, g := range t.Required {
		fmt.Fprintf(&b, "Required H2 headings: %s. Write them as given (extra words after them are fine); Folio checks them.\n", strings.Join(g, ", "))
	}
	if t.MaxPages > 0 {
		fmt.Fprintf(&b, "At most %d page(s); Folio checks it.\n", t.MaxPages)
	}
	if t.Density != "" {
		fmt.Fprintf(&b, "Density: %s (create_notebook uses it when you pass this template and no density).\n", t.Density)
	}
	return b.String() + "\n" + t.Body
}

// Checklist is the template's "## Checklist" section, sent with the first write of a session.
func (t Template) Checklist() string {
	_, rest, ok := strings.Cut(t.Body, "## Checklist\n")
	if !ok {
		return ""
	}
	rest, _, _ = strings.Cut(rest, "\n## ")
	return strings.TrimSpace(rest)
}

// Skeleton is the example unit in the template's "## Skeleton" section.
func (t Template) Skeleton() []json.RawMessage {
	_, rest, _ := strings.Cut(t.Body, "## Skeleton")
	_, rest, _ = strings.Cut(rest, "```json\n")
	js, _, _ := strings.Cut(rest, "```")
	var bs []json.RawMessage
	json.Unmarshal([]byte(js), &bs)
	return bs
}

type section struct {
	anchor   string // the H1's block ID, or the first block when the notebook doesn't start with an H1
	title    string
	headings []string
}

func sections(bs []model.Block) []section {
	var out []section
	for _, b := range bs {
		h, isHeading := heading(b)
		if len(out) == 0 || isHeading && h.Level == 1 {
			out = append(out, section{anchor: b.ID})
		}
		s := &out[len(out)-1]
		switch {
		case isHeading && h.Level == 1:
			s.title = plain(h.Text)
		case isHeading:
			s.headings = append(s.headings, plain(h.Text))
		}
		for _, k := range b.Children() {
			if h, ok := heading(k); ok {
				s.headings = append(s.headings, plain(h.Text))
			}
		}
	}
	return out
}

func heading(b model.Block) (model.Heading, bool) {
	var h model.Heading
	if b.Type != "heading" || json.Unmarshal(b.Data, &h) != nil {
		return h, false
	}
	return h, true
}

func plain(s string) string {
	var b strings.Builder
	for _, sp := range model.ParseInline(s) {
		b.WriteString(sp.Text)
	}
	return b.String()
}

// norm makes "2. **Complexity** & edge cases" compare as "complexity & edge cases".
func norm(s string) string {
	return strings.ToLower(strings.TrimLeftFunc(s, func(r rune) bool { return !unicode.IsLetter(r) }))
}

func has(headings []string, want string) bool {
	for _, h := range headings {
		if strings.HasPrefix(norm(h), norm(want)) {
			return true
		}
	}
	return false
}

// Check lists sections that have some, but not all, of a required group's headings
// (a section with none is an intro or another kind of page and is left alone), and
// notebooks longer than MaxPages. page maps a block ID to its page.
func (t Template) Check(bs []model.Block, pageCount int, page func(id string) int) []layout.Issue {
	var is []layout.Issue
	for _, s := range sections(bs) {
		var best []string
		bestHits := 0
		for _, g := range t.Required {
			var missing []string
			for _, want := range g {
				if !has(s.headings, want) {
					missing = append(missing, want)
				}
			}
			if hits := len(g) - len(missing); hits > bestHits {
				best, bestHits = missing, hits
			}
		}
		if len(best) == 0 {
			continue
		}
		title := s.title
		if title == "" {
			title = "the start of the notebook"
		}
		is = append(is, layout.Issue{
			Check:   "missing_section",
			Page:    page(s.anchor),
			Blocks:  []string{s.anchor},
			Problem: fmt.Sprintf("section %q (%s template) has no %s heading", title, t.ID, quoteAll(best)),
			Fix:     fmt.Sprintf("add H2 %s with what the %s template puts there (get_authoring_guide template=%s), or rename the heading that holds it", quoteAll(best), t.ID, t.ID),
		})
	}
	if t.MaxPages > 0 && pageCount > t.MaxPages {
		is = append(is, layout.Issue{
			Check:   "template_too_long",
			Page:    pageCount,
			Problem: fmt.Sprintf("a %s is at most %d page(s); this notebook has %d", strings.ToLower(t.Name), t.MaxPages, pageCount),
			Fix:     "cut the least important area, turn prose into table rows, or pair small areas in two_column",
		})
	}
	return is
}

func quoteAll(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = strconv.Quote(s)
	}
	return strings.Join(q, ", ")
}
