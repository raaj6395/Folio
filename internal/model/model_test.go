package model

import (
	"math/rand"
	"reflect"
	"slices"
	"sort"
	"testing"
)

func TestKeyBetweenRandomInserts(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	keys := []string{}
	for n := 0; n < 2000; n++ {
		i := r.Intn(len(keys) + 1)
		a, b := "", ""
		if i > 0 {
			a = keys[i-1]
		}
		if i < len(keys) {
			b = keys[i]
		}
		k := KeyBetween(a, b)
		if !(a < k && (b == "" || k < b)) || k[len(k)-1] == '0' {
			t.Fatalf("KeyBetween(%q,%q)=%q out of order", a, b, k)
		}
		keys = slices.Insert(keys, i, k)
	}
	if !sort.StringsAreSorted(keys) {
		t.Fatal("keys not sorted")
	}
}

func TestKeysAtStart(t *testing.T) {
	b := "i"
	for n := 0; n < 200; n++ {
		k := KeyBetween("", b)
		if !(k < b) || k == "" {
			t.Fatalf("KeyBetween(\"\",%q)=%q", b, k)
		}
		b = k
	}
}

func TestParseInline(t *testing.T) {
	cases := []struct {
		in   string
		want []Span
	}{
		{"plain", []Span{{Text: "plain"}}},
		{"a **b** c", []Span{{Text: "a "}, {Text: "b", Bold: true}, {Text: " c"}}},
		{"*i* and `x*y`", []Span{{Text: "i", Italic: true}, {Text: " and "}, {Text: "x*y", Code: true}}},
		{"**b *bi* b**", []Span{{Text: "b ", Bold: true}, {Text: "bi", Bold: true, Italic: true}, {Text: " b", Bold: true}}},
		{"**b *bi***", []Span{{Text: "b *bi", Bold: true}, {Text: "*"}}},
		{"==hi== [go](https://go.dev)", []Span{{Text: "hi", Highlight: true}, {Text: " "}, {Text: "go", Link: "https://go.dev"}}},
		{"unclosed **bold and `code and [link](x", []Span{{Text: "unclosed **bold and `code and [link](x"}}},
		{"2 * 3 * 4", []Span{{Text: "2 "}, {Text: " 3 ", Italic: true}, {Text: " 4"}}},
		{"****", []Span{{Text: "****"}}},
		{"[x](javascript:alert(1)) [m](mailto:a@b.c)", []Span{{Text: "x) "}, {Text: "m", Link: "mailto:a@b.c"}}},
		{"", []Span{}},
	}
	for _, c := range cases {
		if got := ParseInline(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseInline(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestAppendKeysStayShort(t *testing.T) {
	k := ""
	for n := 0; n < 300; n++ {
		next := KeyBetween(k, "")
		if next <= k {
			t.Fatalf("append key %q not after %q", next, k)
		}
		k = next
	}
	if len(k) > 10 {
		t.Fatalf("key after 300 appends is %d chars: %q", len(k), k)
	}
}
