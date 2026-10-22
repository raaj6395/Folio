package httpserver

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/store"
)

func pkce() (verifier, challenge string) {
	verifier = strings.Repeat("v3rifier-", 6)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func jsonBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]any
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not JSON (%d): %s", resp.StatusCode, b)
	}
	return m
}

func TestOAuthFlowLikeClaude(t *testing.T) {
	eachStore(t, func(t *testing.T, st *store.Store) {
		var clientURL, redirect, codeClientURL string
		meta := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/oauth/claude.json":
				json.NewEncoder(w).Encode(map[string]any{"client_id": clientURL, "client_name": "Claude", "redirect_uris": []string{redirect}, "token_endpoint_auth_method": "none"})
			case "/oauth/claude-code.json":
				json.NewEncoder(w).Encode(map[string]any{"client_id": codeClientURL, "client_name": "Claude Code", "redirect_uris": []string{"http://localhost/callback", "http://127.0.0.1/callback"}, "token_endpoint_auth_method": "none"})
			case "/oauth/elsewhere.json":
				json.NewEncoder(w).Encode(map[string]any{"client_id": "https://" + r.Host + "/oauth/elsewhere.json", "client_name": "Sneaky", "redirect_uris": []string{"https://attacker.example/cb"}})
			case "/oauth/liar.json":
				json.NewEncoder(w).Encode(map[string]any{"client_id": "https://someone-else.example/c.json", "client_name": "Liar", "redirect_uris": []string{redirect}})
			default:
				http.NotFound(w, r)
			}
		}))
		defer meta.Close()
		clientURL = meta.URL + "/oauth/claude.json"
		codeClientURL = meta.URL + "/oauth/claude-code.json"
		redirect = meta.URL + "/api/mcp/auth_callback"

		home := t.TempDir()
		lb, _ := layout.New(home)
		srv := httptest.NewServer(nil)
		defer srv.Close()
		srv.Config.Handler = Handler(st, lb, Options{Home: home, PublicURL: srv.URL, Auth: true, ClientHTTP: meta.Client()})
		s := &site{t, srv}

		resp, _ := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader("{}"))
		wantMeta := srv.URL + "/.well-known/oauth-protected-resource/mcp"
		if resp.StatusCode != 401 || !strings.Contains(resp.Header.Get("WWW-Authenticate"), `resource_metadata="`+wantMeta+`"`) {
			t.Fatalf("401 challenge: %d %q", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
		}
		for _, p := range []string{"/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource"} {
			resp, _ := http.Get(srv.URL + p)
			prm := jsonBody(t, resp)
			if prm["resource"] != srv.URL+"/mcp" || fmt.Sprint(prm["authorization_servers"]) != "["+srv.URL+"]" {
				t.Fatalf("%s: %v", p, prm)
			}
		}
		resp, _ = http.Get(srv.URL + "/.well-known/oauth-authorization-server")
		as := jsonBody(t, resp)
		if as["issuer"] != srv.URL || as["client_id_metadata_document_supported"] != true || fmt.Sprint(as["code_challenge_methods_supported"]) != "[S256]" || as["authorization_response_iss_parameter_supported"] != true {
			t.Fatalf("AS metadata: %v", as)
		}

		alice := s.browser()
		alice.signupAndToken("alice@example.com")
		alice.c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		verifier, challenge := pkce()
		authz := func(clientID, redirectURI string) string {
			return "/oauth/authorize?" + url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirectURI},
				"code_challenge": {challenge}, "code_challenge_method": {"S256"}, "state": {"st8"}, "resource": {srv.URL + "/mcp"}}.Encode()
		}

		anon := s.browser()
		anon.c.CheckRedirect = alice.c.CheckRedirect
		if resp, _ := anon.get(authz(clientURL, redirect)); resp.StatusCode != 303 || !strings.HasPrefix(resp.Header.Get("Location"), "/login?next=%2Foauth%2Fauthorize") {
			t.Fatalf("logged-out authorize: %d %s", resp.StatusCode, resp.Header.Get("Location"))
		}
		if resp, body := alice.get(authz(clientURL, "https://evil.example/cb")); resp.StatusCode != 400 || resp.Header.Get("Location") != "" || !strings.Contains(body, "register") {
			t.Fatalf("unregistered redirect must not redirect: %d", resp.StatusCode)
		}
		if resp, body := alice.get(authz(meta.URL+"/oauth/liar.json", redirect)); resp.StatusCode != 400 || !strings.Contains(body, "does not match its URL") {
			t.Fatalf("mismatched client document: %d", resp.StatusCode)
		}

		if resp, body := alice.get(authz(meta.URL+"/oauth/elsewhere.json", "https://attacker.example/cb")); resp.StatusCode != 400 || !strings.Contains(body, "same origin") {
			t.Fatalf("CIMD redirect on another origin must be refused: %d", resp.StatusCode)
		}
		if resp, body := alice.get(authz(codeClientURL, "http://localhost:51234/callback")); resp.StatusCode != 200 || !strings.Contains(body, "your own computer") {
			t.Fatalf("Claude Code loopback redirect with a port must be accepted with a warning: %d %.200s", resp.StatusCode, body)
		}
		if resp, _ := alice.get(authz(codeClientURL, "http://localhost:51234/other")); resp.StatusCode != 400 {
			t.Fatalf("loopback redirect with another path must be refused: %d", resp.StatusCode)
		}

		getCode := func(clientID string) string {
			t.Helper()
			resp, body := alice.get(authz(clientID, redirect))
			if resp.StatusCode != 200 || !strings.Contains(body, "Allow access?") {
				t.Fatalf("consent page: %d %.200s", resp.StatusCode, body)
			}
			form := url.Values{}
			for _, m := range regexp.MustCompile(`name="([a-z_]+)" value="([^"]*)"`).FindAllStringSubmatch(body, -1) {
				form.Set(m[1], strings.ReplaceAll(m[2], "&amp;", "&"))
			}
			form.Set("decision", "allow")
			resp, _ = alice.post("/oauth/authorize", form)
			loc, _ := url.Parse(resp.Header.Get("Location"))
			if resp.StatusCode != 302 || loc.Scheme+"://"+loc.Host+loc.Path != redirect || loc.Query().Get("state") != "st8" || loc.Query().Get("iss") != srv.URL || loc.Query().Get("code") == "" {
				t.Fatalf("consent redirect: %d %s", resp.StatusCode, loc)
			}
			return loc.Query().Get("code")
		}
		exchange := func(form url.Values) (int, map[string]any) {
			resp, _ := http.PostForm(srv.URL+"/oauth/token", form)
			return resp.StatusCode, jsonBody(t, resp)
		}
		codeForm := func(code, clientID, v string) url.Values {
			return url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID}, "redirect_uri": {redirect}, "code_verifier": {v}, "resource": {srv.URL + "/mcp"}}
		}

		bad := getCode(clientURL)
		if st, body := exchange(codeForm(bad, clientURL, strings.Repeat("x", 50))); st != 400 || body["error"] != "invalid_grant" {
			t.Fatalf("wrong PKCE verifier: %d %v", st, body)
		}

		code := getCode(clientURL)
		status, tok := exchange(codeForm(code, clientURL, verifier))
		if status != 200 || tok["token_type"] != "Bearer" || tok["refresh_token"] == nil {
			t.Fatalf("token: %d %v", status, tok)
		}
		if st, body := exchange(codeForm(code, clientURL, verifier)); st != 400 || body["error"] != "invalid_grant" {
			t.Fatalf("replayed code: %d %v", st, body)
		}

		use := func(access string) error {
			cs, err := s.claude(access)
			if err != nil {
				return err
			}
			defer cs.Close()
			_, text := callTool(t, cs, "create_notebook", map[string]any{"title": "via OAuth"})
			if !strings.Contains(text, "via OAuth") {
				return fmt.Errorf("tool call failed: %s", text)
			}
			return nil
		}
		if err := use(tok["access_token"].(string)); err != nil {
			t.Fatalf("access token on /mcp: %v", err)
		}
		if _, body := alice.get("/"); !strings.Contains(body, "via OAuth") {
			t.Fatal("notebook made through OAuth should belong to alice")
		}

		oldRefresh := tok["refresh_token"].(string)
		status, tok2 := exchange(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {oldRefresh}, "client_id": {clientURL}})
		if status != 200 || tok2["refresh_token"] == oldRefresh {
			t.Fatalf("refresh: %d %v", status, tok2)
		}
		if st, _ := exchange(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {oldRefresh}, "client_id": {clientURL}}); st != 400 {
			t.Fatal("old refresh token must stop working after rotation")
		}
		if err := use(tok2["access_token"].(string)); err != nil {
			t.Fatalf("refreshed access token: %v", err)
		}

		_, page := alice.get("/connect")
		grant := regexp.MustCompile(`/connect/apps/(g_[a-z2-7]+)/revoke`).FindStringSubmatch(page)
		if grant == nil || !strings.Contains(page, "Claude") {
			t.Fatalf("connect page should list the Claude app: %.300s", page)
		}
		alice.post("/connect/apps/"+grant[1]+"/revoke", nil)
		if err := use(tok2["access_token"].(string)); err == nil {
			t.Fatal("access token still works after disconnect")
		}
		if st, _ := exchange(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok2["refresh_token"].(string)}, "client_id": {clientURL}}); st != 400 {
			t.Fatal("refresh still works after disconnect")
		}

		resp, _ = http.Post(srv.URL+"/oauth/register", "application/json", strings.NewReader(`{"client_name":"DCR app","redirect_uris":["`+redirect+`"]}`))
		reg := jsonBody(t, resp)
		if resp.StatusCode != 201 || reg["token_endpoint_auth_method"] != "none" {
			t.Fatalf("register: %d %v", resp.StatusCode, reg)
		}
		dcrCode := getCode(reg["client_id"].(string))
		if st, body := exchange(codeForm(dcrCode, reg["client_id"].(string), verifier)); st != 200 {
			t.Fatalf("DCR client token: %d %v", st, body)
		}
		resp, _ = http.Post(srv.URL+"/oauth/register", "application/json", strings.NewReader(`{"redirect_uris":["http://evil.example/cb"]}`))
		if resp.StatusCode != 400 {
			t.Fatalf("plain-http non-local redirect must be refused: %d", resp.StatusCode)
		}
	})
}
