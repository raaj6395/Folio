package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raaj6395/folio/internal/store"
)

func TestCIMDRefusesPrivateAddresses(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{}")) }))
	defer srv.Close()
	_, err := cimdHTTPClient(false).Get(srv.URL + "/client.json")
	if err == nil || !strings.Contains(err.Error(), "private address") {
		t.Fatalf("loopback fetch must be refused, got %v", err)
	}
}

func TestRedirectMatching(t *testing.T) {
	reg := []string{"http://localhost/callback", "http://127.0.0.1/callback", "https://claude.ai/api/mcp/auth_callback"}
	for uri, want := range map[string]bool{
		"https://claude.ai/api/mcp/auth_callback":      true,
		"https://claude.ai/api/mcp/auth_callback?x=1":  false,
		"https://claude.ai:8443/api/mcp/auth_callback": false,
		"http://localhost:3118/callback":               true,
		"http://127.0.0.1:50000/callback":              true,
		"http://localhost:3118/other":                  false,
		"http://evil.example:3118/callback":            false,
		"http://localhost:3118/callback#frag":          false,
	} {
		if got := redirectAllowed(reg, uri); got != want {
			t.Errorf("redirectAllowed(%q) = %v, want %v", uri, got, want)
		}
	}
}

func TestRealClaudeCodeMetadata(t *testing.T) {
	if os.Getenv("FOLIO_NET_TEST") == "" {
		t.Skip("set FOLIO_NET_TEST=1 to fetch Claude Code's real client metadata document")
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "f.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := New(st, "https://folio.example", nil)
	c, err := a.resolveClient(context.Background(), "https://claude.ai/oauth/claude-code-client-metadata")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Claude Code" || !redirectAllowed(c.RedirectURIs, "http://localhost:45678/callback") {
		t.Fatalf("%+v", c)
	}
}
