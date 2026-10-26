package httpserver

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/raaj6395/folio/internal/store"
)

func TestFoldersFromClaudeAndIsolation(t *testing.T) {
	eachStore(t, func(t *testing.T, st *store.Store) {
		s := newSite(t, st)
		alice, bob := s.browser(), s.browser()
		a, err := s.claude(alice.signupAndToken("alice@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		defer a.Close()
		b, err := s.claude(bob.signupAndToken("bob@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		defer b.Close()

		res, text := callTool(t, a, "create_notebook", map[string]any{"title": "TCP", "folder": "Semester 1/Networks"})
		var nb struct{ ID, Folder string }
		js, _ := json.Marshal(res.StructuredContent)
		json.Unmarshal(js, &nb)
		if res.IsError || nb.Folder != "Semester 1/Networks" {
			t.Fatalf("create in folder: %s", text)
		}
		res, _ = callTool(t, a, "create_notebook", map[string]any{"title": "UDP", "folder": "semester 1 / networks"})
		if js, _ := json.Marshal(res.StructuredContent); !strings.Contains(string(js), `"folder":"Semester 1/Networks"`) {
			t.Errorf("folder path must be reused whatever the case: %s", js)
		}
		callTool(t, a, "create_notebook", map[string]any{"title": "Loose"})
		if _, text := callTool(t, a, "list_folders", nil); !strings.Contains(text, `"notebook_count":2,"path":"Semester 1/Networks"`) {
			t.Errorf("list_folders: %s", text)
		}
		if _, text := callTool(t, a, "list_notebooks", map[string]any{"folder": "Semester 1"}); !strings.Contains(text, "TCP") || strings.Contains(text, "Loose") {
			t.Errorf("list_notebooks filter: %s", text)
		}
		if res, text := callTool(t, a, "create_notebook", map[string]any{"title": "x", "folder": "a/b/c/d"}); !res.IsError || !strings.Contains(text, "3 levels") {
			t.Errorf("too deep: %s", text)
		}
		if res, _ := callTool(t, a, "move_notebook", map[string]any{"notebook_id": nb.ID, "folder": ""}); res.IsError {
			t.Error("move to top level")
		}

		// Bob can't reach Alice's folders from Claude or the web.
		if _, text := callTool(t, b, "list_folders", nil); strings.Contains(text, "Semester") {
			t.Errorf("bob sees alice's folders: %s", text)
		}
		callTool(t, b, "move_notebook", map[string]any{"notebook_id": nb.ID, "folder": "pwned"})
		if _, text := callTool(t, b, "list_folders", nil); strings.Contains(text, "pwned") {
			t.Error("a failed move must not create folders")
		}
		aliceNB := nb.ID
		folders, _ := st.Folders(mustUser(t, st, "alice@example.com"))
		fid := folders[0].ID
		res, _ = callTool(t, b, "create_notebook", map[string]any{"title": "Bob's"})
		js, _ = json.Marshal(res.StructuredContent)
		json.Unmarshal(js, &nb)
		for path, form := range map[string]url.Values{
			"/f/" + fid + "/rename":       {"name": {"pwned"}},
			"/f/" + fid + "/delete":       nil,
			"/n/" + nb.ID + "/move":       {"folder_id": {fid}},
			"/folders":                    {"name": {"x"}, "parent_id": {fid}},
			"/f/" + fid + "/move":         {"parent_id": {""}},
			"/f/" + fid + "/color":        {"color": {"red"}},
			"/n/" + aliceNB + "/favorite": {"on": {"1"}},
			"/notebooks/move":             {"id": {nb.ID}, "folder_id": {fid}},
		} {
			if resp, _ := bob.post(path, form); resp.StatusCode != 404 {
				t.Errorf("bob POST %s: %d", path, resp.StatusCode)
			}
		}
		if resp, body := bob.get("/f/" + fid); resp.StatusCode != 404 || strings.Contains(body, "Semester") {
			t.Errorf("bob GET alice's folder: %d", resp.StatusCode)
		}
		bob.post("/notebooks/move", url.Values{"id": {aliceNB}, "folder_id": {""}})
		if got, _ := st.Notebook(mustUser(t, st, "alice@example.com"), aliceNB); got.Favorite {
			t.Error("bob starred alice's notebook")
		}
		if after, _ := st.Folders(mustUser(t, st, "alice@example.com")); len(after) != len(folders) || after[0].Name != "Semester 1" || after[0].Color != "" {
			t.Error("alice's folders changed")
		}
	})
}

func mustUser(t *testing.T, st *store.Store, email string) string {
	t.Helper()
	u, err := st.UserByEmail(email)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}
