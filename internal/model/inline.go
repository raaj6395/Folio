package model

import "strings"

type Span struct {
	Text      string `json:"t"`
	Bold      bool   `json:"b,omitempty"`
	Italic    bool   `json:"i,omitempty"`
	Code      bool   `json:"c,omitempty"`
	Highlight bool   `json:"h,omitempty"`
	Link      string `json:"l,omitempty"`
}

func ParseInline(s string) []Span {
	var out []Span
	parseInline(s, Span{}, &out)
	if out == nil {
		out = []Span{}
	}
	return out
}

func emit(out *[]Span, style Span, text string) {
	if text == "" {
		return
	}
	if n := len(*out); n > 0 {
		last := &(*out)[n-1]
		if last.Bold == style.Bold && last.Italic == style.Italic && last.Code == style.Code && last.Highlight == style.Highlight && last.Link == style.Link {
			last.Text += text
			return
		}
	}
	style.Text = text
	*out = append(*out, style)
}

func singleStar(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] != '*' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '*' {
			i++
			continue
		}
		return i
	}
	return -1
}

func parseInline(s string, st Span, out *[]Span) {
	var buf strings.Builder
	flush := func() { emit(out, st, buf.String()); buf.Reset() }
	for i := 0; i < len(s); {
		rest := s[i:]
		switch {
		case rest[0] == '`':
			if j := strings.IndexByte(rest[1:], '`'); j > 0 {
				flush()
				c := st
				c.Code = true
				emit(out, c, rest[1:1+j])
				i += j + 2
				continue
			}
		case strings.HasPrefix(rest, "**"):
			if j := strings.Index(rest[2:], "**"); j > 0 {
				flush()
				c := st
				c.Bold = true
				parseInline(rest[2:2+j], c, out)
				i += j + 4
				continue
			}
		case strings.HasPrefix(rest, "=="):
			if j := strings.Index(rest[2:], "=="); j > 0 {
				flush()
				c := st
				c.Highlight = true
				parseInline(rest[2:2+j], c, out)
				i += j + 4
				continue
			}
		case rest[0] == '*':
			if j := singleStar(rest[1:]); j > 0 {
				flush()
				c := st
				c.Italic = true
				parseInline(rest[1:1+j], c, out)
				i += j + 2
				continue
			}
		case rest[0] == '[':
			if j := strings.Index(rest, "]("); j > 1 {
				if k := strings.IndexByte(rest[j+2:], ')'); k > 0 {
					flush()
					c := st
					if u := rest[j+2 : j+2+k]; safeURL(u) {
						c.Link = u
					}
					parseInline(rest[1:j], c, out)
					i += j + 3 + k
					continue
				}
			}
		}
		buf.WriteByte(s[i])
		i++
	}
	flush()
}

func safeURL(u string) bool {
	l := strings.ToLower(u)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "mailto:")
}
