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
		`{"type":"table","columns":["Method","Safe"],"rows":[["GET","yes"],["POST","no"]],"caption":"x"}`,
		`{"type":"qa","question":"Q?","answer":"A","hide_answer":true}`,
		`{"type":"fill_in","prompt":"Explain","lines":3}`,
		`{"type":"spacer","size":"medium"}`,
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
		{`{"type":"table","columns":["Method","Idempotent","Safe"],"rows":[["a","b","c"],["a","b","c"],["a","b","c","d"]]}`, "row 2 has 4 cells, expected 3 (columns: Method, Idempotent, Safe)"},
		{`{"type":"table","columns":["a"],"rows":[["x"]]}`, "columns has 1 entries, must be 2-6"},
		{`{"type":"table","columns":["a","b"],"rows":[` + strings.TrimSuffix(strings.Repeat(`["x","y"],`, 26), ",") + `]}`, "26 rows, max 25"},
		{`{"type":"qa","question":"q","answer":""}`, `field "answer" is required`},
		{`{"type":"fill_in","prompt":"p","lines":16}`, "lines must be 1-15, got 16"},
		{`{"type":"spacer","size":"huge"}`, `size must be "small", "medium" or "large"`},
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

func TestTableAlignment(t *testing.T) {
	align := func(src string) []string {
		t.Helper()
		b, err := Parse(json.RawMessage(src))
		if err != nil {
			t.Fatal(err)
		}
		return b.Doc()["align"].([]string)
	}
	got := align(`{"type":"table","columns":["Plan","Price","Share","Notes"],"rows":[["Free","$0","12%","—"],["Pro","$1,200","3.5%","**best**"],["Team","-","40 %","x"]]}`)
	if strings.Join(got, ",") != "left,right,right,left" {
		t.Fatalf("default alignment: %v", got)
	}
	got = align(`{"type":"table","columns":["a","b"],"rows":[["1","2"]],"align":["center","left"]}`)
	if strings.Join(got, ",") != "center,left" {
		t.Fatalf("explicit alignment: %v", got)
	}
	for src, want := range map[string]string{
		`{"type":"table","columns":["a","b"],"rows":[["1","2"]],"align":["left"]}`:          "align has 1 entries, expected one per column (2)",
		`{"type":"table","columns":["a","b"],"rows":[["1","2"]],"align":["left","middle"]}`: `align[1] must be "left", "center" or "right", got "middle"`,
	} {
		if _, err := Parse(json.RawMessage(src)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%s): got %v, want %q", src, err, want)
		}
	}
}

func TestHeadingInsideTwoColumn(t *testing.T) {
	ok := `{"type":"two_column","left":[{"type":"heading","level":3,"text":"Pros"},{"type":"paragraph","text":"fast"}],"right":[{"type":"paragraph","text":"x"}]}`
	if _, err := Parse(json.RawMessage(ok)); err != nil {
		t.Fatalf("H3 in two_column: %v", err)
	}
	bad := `{"type":"two_column","left":[{"type":"heading","level":2,"text":"Pros"}],"right":[{"type":"paragraph","text":"x"}]}`
	if _, err := Parse(json.RawMessage(bad)); err == nil || !strings.Contains(err.Error(), "only level 3 headings are allowed inside two_column") {
		t.Fatalf("H2 in two_column: %v", err)
	}
}
