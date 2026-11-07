package notetemplates

import (
	"strings"
	"testing"

	"github.com/raaj6395/folio/internal/model"
)

func blocks(t *testing.T, js string) []model.Block {
	t.Helper()
	tpl := Template{Body: "## Skeleton\n```json\n" + js + "\n```"}
	bs, err := model.ParseAll(tpl.Skeleton())
	if err != nil {
		t.Fatal(err)
	}
	for i := range bs {
		bs[i].ID = model.NewID("b_")
	}
	return bs
}

func onePage(string) int { return 1 }

// Every template teaches Claude only valid blocks, and its own example passes its own check.
func TestSkeletonsAreValidAndComplete(t *testing.T) {
	for _, tpl := range All {
		raws := tpl.Skeleton()
		if len(raws) == 0 {
			t.Errorf("%s: no skeleton", tpl.ID)
			continue
		}
		bs, err := model.ParseAll(raws)
		if err != nil {
			t.Errorf("%s skeleton: %v", tpl.ID, err)
			continue
		}
		if is := tpl.Check(bs, 1, onePage); len(is) != 0 {
			t.Errorf("%s skeleton fails its own check: %+v", tpl.ID, is)
		}
		if tpl.Checklist() == "" || strings.Contains(tpl.Checklist(), "##") {
			t.Errorf("%s: checklist %q", tpl.ID, tpl.Checklist())
		}
		if !strings.Contains(Index(), "- "+tpl.ID+": ") || !strings.HasPrefix(tpl.Guide(), "# Template: "+tpl.Name) {
			t.Errorf("%s missing from the index or guide", tpl.ID)
		}
	}
	if order[len(order)-1] != "topic" || Label("topic") != "" || Label("dsa_problem") != "DSA" || Label("nope") != "" {
		t.Fatal("topic is the unlabelled default, offered last")
	}
}

func TestCheck(t *testing.T) {
	dsa, _ := ByID("dsa_problem")
	bs := blocks(t, `[
		{"type":"heading","level":1,"text":"Intro to sliding window"},
		{"type":"paragraph","text":"No required headings here, so it's not a problem page."},
		{"type":"heading","level":1,"text":"1. Two Sum"},
		{"type":"heading","level":2,"text":"**1. Problem** statement"},
		{"type":"two_column","left":[{"type":"heading","level":3,"text":"Approach"},{"type":"paragraph","text":"x"}],"right":[{"type":"paragraph","text":"y"}]},
		{"type":"heading","level":2,"text":"time & space"},
		{"type":"heading","level":1,"text":"2. Three Sum"},
		{"type":"heading","level":2,"text":"Problem"},
		{"type":"heading","level":2,"text":"Approach"},
		{"type":"heading","level":2,"text":"Code"},
		{"type":"heading","level":2,"text":"COMPLEXITY and edge cases"}
	]`)
	is := dsa.Check(bs, 3, onePage)
	if len(is) != 1 || is[0].Check != "missing_section" || is[0].Blocks[0] != bs[2].ID {
		t.Fatalf("only Two Sum is incomplete: %+v", is)
	}
	if !strings.Contains(is[0].Problem, `"Code", "Complexity"`) || !strings.Contains(is[0].Fix, "template=dsa_problem") {
		t.Fatalf("issue names what's missing and where the blueprint is: %+v", is[0])
	}

	sd, _ := ByID("system_design")
	bs = blocks(t, `[
		{"type":"heading","level":1,"text":"Chat: Architecture"},
		{"type":"heading","level":2,"text":"Components"},
		{"type":"heading","level":1,"text":"Chat: Deep dive"},
		{"type":"heading","level":2,"text":"Trade-offs"}
	]`)
	if is := sd.Check(bs, 2, onePage); len(is) != 1 || !strings.Contains(is[0].Problem, `"Data model"`) || is[0].Blocks[0] != bs[0].ID {
		t.Fatalf("each part is checked against its own group: %+v", is)
	}

	cs, _ := ByID("cheat_sheet")
	if is := cs.Check(nil, 3, onePage); len(is) != 1 || is[0].Check != "template_too_long" || is[0].Page != 3 {
		t.Fatalf("cheat sheet over 2 pages: %+v", is)
	}
	topic, _ := ByID("topic")
	if is := topic.Check(bs, 40, onePage); len(is) != 0 {
		t.Fatalf("topic checks nothing: %+v", is)
	}
}
