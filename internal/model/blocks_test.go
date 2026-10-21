package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	for _, s := range []string{
		`{"type":"heading","level":2,"text":"HTTP **methods**"}`,
		`{"type":"paragraph","text":"hello"}`,
		`{"type":"bullets","items":[{"text":"a","children":[{"text":"b","children":[{"text":"c"}]}]}]}`,
		`{"type":"numbered","items":[{"text":"one"}]}`,
		`{"type":"callout","variant":"warning","title":"Careful","body":["x","y"]}`,
		`{"type":"code","language":"go","code":"fmt.Println(1)\n"}`,
		`{"type":"divider"}`,
		`{"type":"page_break","id":"b_ignored1"}`,
	} {
		if _, err := Parse(json.RawMessage(s)); err != nil {
			t.Errorf("Parse(%s): %v", s, err)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"type":"nope"}`, `unknown or missing type "nope"`},
		{`{"text":"x"}`, `unknown or missing type ""`},
		{`{"type":"heading","level":4,"text":"x"}`, "level must be 1, 2 or 3, got 4"},
		{`{"type":"heading","level":1,"text":"x","size":3}`, `unknown field "size"; allowed fields: level, text`},
		{`{"type":"heading","level":"1","text":"x"}`, `field "level" must be an integer, got string`},
		{`{"type":"paragraph","text":""}`, `field "text" is required`},
		{`{"type":"paragraph","text":"` + strings.Repeat("a", 2001) + `"}`, "2001 chars, max 2000"},
		{`{"type":"bullets","items":[{"text":"a","children":[{"text":"b","children":[{"text":"c","children":[{"text":"d"}]}]}]}]}`, "item 0.0.0.0 is nested 4 levels deep, max 3"},
		{`{"type":"bullets","items":[` + strings.TrimSuffix(strings.Repeat(`{"text":"x"},`, 41), ",") + `]}`, "41 items in total, max 40"},
		{`{"type":"callout","variant":"info","body":["x"]}`, "variant must be one of note, tip, warning, definition, key_point"},
		{`{"type":"callout","variant":"note","body":"x"}`, `field "body" must be a list of string, got string`},
		{`{"type":"code","language":"go","code":"` + strings.Repeat(`x\n`, 81) + `"}`, "81 lines, max 80"},
		{`{"type":"divider","text":"x"}`, "takes no fields besides type"},
	}
	for _, c := range cases {
		_, err := Parse(json.RawMessage(c.in))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Parse(%.60s): got %v, want error containing %q", c.in, err, c.want)
		}
	}
}

func TestParseAllIsAtomicAndIndexed(t *testing.T) {
	raws := []json.RawMessage{json.RawMessage(`{"type":"divider"}`), json.RawMessage(`{"type":"heading","level":9,"text":"x"}`)}
	blocks, err := ParseAll(raws)
	if blocks != nil || err == nil || !strings.HasPrefix(err.Error(), "block[1] (heading): level") {
		t.Fatalf("got %v, %v", blocks, err)
	}
}

func TestBlockMarshalFlat(t *testing.T) {
	b, _ := Parse(json.RawMessage(`{"type":"paragraph","text":"hi"}`))
	b.ID = "b_abc"
	out, _ := json.Marshal(b)
	if string(out) != `{"id":"b_abc","type":"paragraph","text":"hi"}` {
		t.Fatal(string(out))
	}
}
