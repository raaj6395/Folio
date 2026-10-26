package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/store"
)

type site struct {
	t   *testing.T
	srv *httptest.Server
}

func newSite(t *testing.T, st *store.Store) *site {
	home := t.TempDir()
	lb, err := layout.New(home)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(nil)
	t.Cleanup(srv.Close)
	srv.Config.Handler = Handler(st, lb, Options{Home: home, PublicURL: srv.URL, Auth: true})
	return &site{t, srv}
}

type browser struct {
	t *testing.T
	c *http.Client
	s *site
}

func (s *site) browser() *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{s.t, &http.Client{Jar: jar}, s}
}

func (b *browser) post(path string, form url.Values) (*http.Response, string) {
	b.t.Helper()
	resp, err := b.c.PostForm(b.s.srv.URL+path, form)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func (b *browser) get(path string) (*http.Response, string) {
	b.t.Helper()
	resp, err := b.c.Get(b.s.srv.URL + path)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

var tokenRE = regexp.MustCompile(`folio_pat_[a-z2-7]{40,}`)

func (b *browser) signupAndToken(email string) string {
	b.t.Helper()
	resp, body := b.post("/signup", url.Values{"email": {email}, "password": {"correct horse battery"}})
	if resp.StatusCode != 200 || resp.Request.URL.Path != "/connect" {
		b.t.Fatalf("signup ended at %s (%d): %.200s", resp.Request.URL.Path, resp.StatusCode, body)
	}
	_, body = b.post("/connect/tokens", url.Values{"name": {"test"}})
	tok := tokenRE.FindString(body)
	if tok == "" || !strings.Contains(body, "--header \"Authorization: Bearer "+tok+"\"") {
		b.t.Fatalf("token not shown with command: %.300s", body)
	}
	return tok
}

type bearerRT struct{ token string }

func (r bearerRT) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func (s *site) claude(token string) (*mcp.ClientSession, error) {
	return mcp.NewClient(&mcp.Implementation{Name: "claude", Version: "0"}, nil).Connect(context.Background(),
		&mcp.StreamableClientTransport{Endpoint: s.srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerRT{token}}, MaxRetries: -1}, nil)
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, string) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	text := ""
	if len(res.Content) > 0 {
		if tc, ok := res.Content[0].(*mcp.TextContent); ok {
			text = tc.Text
		}
	}
	return res, text
}

func eachStore(t *testing.T, fn func(t *testing.T, st *store.Store)) {
	t.Run("sqlite", func(t *testing.T) {
		st, err := store.Open(filepath.Join(t.TempDir(), "folio.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		fn(t, st)
	})
	t.Run("postgres", func(t *testing.T) { fn(t, store.OpenTestPostgres(t)) })
}

func TestAccountsTokensAndIsolation(t *testing.T) {
	eachStore(t, func(t *testing.T, st *store.Store) {
		var logs bytes.Buffer
		old := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
		defer slog.SetDefault(old)

		s := newSite(t, st)
		alice, bob := s.browser(), s.browser()
		aliceTok := alice.signupAndToken("alice@example.com")
		bobTok := bob.signupAndToken("bob@example.com")

		if _, err := s.claude(""); err == nil {
			t.Fatal("MCP without a token must fail")
		}
		if _, err := s.claude("folio_pat_" + strings.Repeat("a", 52)); err == nil {
			t.Fatal("MCP with an unknown token must fail")
		}

		a, err := s.claude(aliceTok)
		if err != nil {
			t.Fatal(err)
		}
		defer a.Close()
		res, text := callTool(t, a, "create_notebook", map[string]any{"title": "Alice secret"})
		if res.IsError {
			t.Fatal(text)
		}
		var nb struct{ ID string }
		js, _ := json.Marshal(res.StructuredContent)
		json.Unmarshal(js, &nb)
		res, text = callTool(t, a, "append_blocks", map[string]any{"notebook_id": nb.ID, "blocks": []map[string]any{{"type": "paragraph", "text": "top secret"}}})
		if res.IsError {
			t.Fatal(text)
		}
		var appended struct {
			ChangedBlockIDs []string `json:"changed_block_ids"`
		}
		js, _ = json.Marshal(res.StructuredContent)
		json.Unmarshal(js, &appended)
		blockID := appended.ChangedBlockIDs[0]

		b, err := s.claude(bobTok)
		if err != nil {
			t.Fatal(err)
		}
		defer b.Close()
		if _, text := callTool(t, b, "list_notebooks", nil); strings.Contains(text, nb.ID) || strings.Contains(text, "Alice") {
			t.Fatalf("bob lists alice's notebook: %s", text)
		}

		attacks := map[string]map[string]any{
			"get_outline":             {},
			"move_notebook":           {"folder": "pwned"},
			"read_blocks":             {"block_ids": []string{blockID}},
			"get_layout_info":         {},
			"render_pages":            {"pages": []int{1}},
			"export_pdf":              {},
			"get_viewer_url":          {},
			"append_blocks":           {"blocks": []map[string]any{{"type": "divider"}}},
			"insert_blocks":           {"after_block_id": "start", "blocks": []map[string]any{{"type": "divider"}}},
			"update_block":            {"block_id": blockID, "block": map[string]any{"type": "paragraph", "text": "pwned"}},
			"delete_blocks":           {"block_ids": []string{blockID}},
			"move_blocks":             {"block_ids": []string{blockID}, "after_block_id": "start"},
			"set_theme":               {"theme": "clean"},
			"rename_notebook":         {"title": "pwned"},
			"delete_notebook":         {"confirm": true},
			"list_versions":           {},
			"undo":                    {},
			"restore_version":         {"version": 1},
			"check_layout":            {},
			"get_notebook_guidelines": {},
			"set_notebook_guidelines": {"text": "pwned"},
		}
		tools, err := b.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, tool := range tools.Tools {
			schema, _ := json.Marshal(tool.InputSchema)
			if strings.Contains(string(schema), `"notebook_id"`) {
				if _, ok := attacks[tool.Name]; !ok {
					t.Errorf("tool %s takes notebook_id but is missing from the isolation matrix", tool.Name)
				}
			}
		}
		for name, args := range attacks {
			args["notebook_id"] = nb.ID
			res, text := callTool(t, b, name, args)
			if !res.IsError || !strings.Contains(text, "not found") || strings.Contains(text, "secret") {
				t.Errorf("%s as bob on alice's notebook: isError=%v text=%.120s", name, res.IsError, text)
			}
		}
		for _, action := range []string{"rename", "delete", "restore"} {
			if resp, body := bob.post("/n/"+nb.ID+"/"+action, url.Values{"title": {"pwned"}}); resp.StatusCode != 404 || strings.Contains(body, "Alice") {
				t.Errorf("bob POST %s on alice's notebook: %d", action, resp.StatusCode)
			}
		}
		if _, text := callTool(t, a, "get_outline", map[string]any{"notebook_id": nb.ID}); !strings.Contains(text, "top secret") || !strings.Contains(text, "Alice secret") {
			t.Fatalf("alice's notebook was changed by bob's attacks: %s", text)
		}

		for _, path := range []string{"/n/" + nb.ID, "/n/" + nb.ID + "/pdf", "/n/" + nb.ID + "/page/1.png", "/n/" + nb.ID + "/thumb.png"} {
			if resp, body := bob.get(path); resp.StatusCode != 404 || strings.Contains(body, "Alice") {
				t.Errorf("bob GET %s: %d %.80s", path, resp.StatusCode, body)
			}
		}
		if resp, body := alice.get("/n/" + nb.ID); resp.StatusCode != 200 || !strings.Contains(body, "Alice secret") {
			t.Errorf("alice viewer: %d", resp.StatusCode)
		}
		if resp, _ := alice.get("/n/" + nb.ID + "/page/1.png"); resp.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("a page image requested without its current fingerprint must not be cached: %q", resp.Header.Get("Cache-Control"))
		}
		if _, body := bob.get("/"); strings.Contains(body, "Alice") {
			t.Error("bob's dashboard shows alice's notebook")
		}

		anon := s.browser()
		anon.c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		if resp, _ := anon.get("/n/" + nb.ID); resp.StatusCode != 303 || !strings.HasPrefix(resp.Header.Get("Location"), "/login?next=") {
			t.Errorf("anonymous viewer: %d %s", resp.StatusCode, resp.Header.Get("Location"))
		}
		if resp, _ := anon.post("/login", url.Values{"email": {"alice@example.com"}, "password": {"correct horse battery"}, "next": {"//evil.example"}}); resp.Header.Get("Location") != "/" {
			t.Errorf("open redirect: %s", resp.Header.Get("Location"))
		}
		if resp, body := anon.post("/login", url.Values{"email": {"alice@example.com"}, "password": {"wrong password!"}}); resp.StatusCode != 401 || !strings.Contains(body, "Invalid email or password") {
			t.Errorf("bad password: %d", resp.StatusCode)
		}
		if resp, body := anon.post("/login", url.Values{"email": {"nobody@example.com"}, "password": {"whatever123"}}); resp.StatusCode != 401 || !strings.Contains(body, "Invalid email or password") {
			t.Errorf("unknown email must look identical: %d", resp.StatusCode)
		}

		req, _ := http.NewRequest("POST", s.srv.URL+"/connect/tokens", strings.NewReader("name=x"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "https://evil.example")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		if resp, _ := alice.c.Do(req); resp.StatusCode != http.StatusForbidden {
			t.Errorf("cross-site POST: %d, want 403", resp.StatusCode)
		}

		_, page := alice.get("/connect")
		id := regexp.MustCompile(`/connect/tokens/(t_[a-z2-7]+)/revoke`).FindStringSubmatch(page)
		if id == nil || strings.Contains(page, aliceTok) {
			t.Fatalf("connect page must list the token but never show it again")
		}
		alice.post("/connect/tokens/"+id[1]+"/revoke", nil)
		if _, err := s.claude(aliceTok); err == nil {
			t.Error("revoked token still works")
		}

		alice.post("/logout", nil)
		alice.c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		if resp, _ := alice.get("/connect"); resp.StatusCode != 303 {
			t.Errorf("after logout /connect: %d", resp.StatusCode)
		}

		for _, tok := range []string{aliceTok, bobTok} {
			if strings.Contains(logs.String(), tok) {
				t.Fatal("a token appeared in the logs")
			}
		}
	})
}
