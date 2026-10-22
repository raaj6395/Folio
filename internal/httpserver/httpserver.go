package httpserver

import (
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/mcpserver"
	"github.com/raaj6395/folio/internal/store"
	"github.com/raaj6395/folio/internal/viewer"
)

func Handler(st *store.Store, lb *layout.Builder, home, publicURL string, public bool) http.Handler {
	srv := mcpserver.New(st, lb, home, publicURL)
	mux := http.NewServeMux()
	opts := &mcp.StreamableHTTPOptions{Stateless: true, DisableLocalhostProtection: public}
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, opts))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := st.Ping(); err != nil {
			http.Error(w, "database unreachable: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, "folio ok (%s)", st.Backend())
		if c := os.Getenv("RENDER_GIT_COMMIT"); len(c) >= 7 {
			fmt.Fprintf(w, " commit %s", c[:7])
		}
	})
	mux.Handle("/", viewer.New(st, lb))
	return mux
}

func IsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
