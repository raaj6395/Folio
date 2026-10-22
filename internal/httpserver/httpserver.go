package httpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/account"
	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/mcpserver"
	"github.com/raaj6395/folio/internal/secret"
	"github.com/raaj6395/folio/internal/store"
	"github.com/raaj6395/folio/internal/viewer"
)

type Options struct {
	Home      string
	PublicURL string
	Public    bool
	Auth      bool
}

func Handler(st *store.Store, lb *layout.Builder, o Options) http.Handler {
	localUser := store.LocalUser
	if o.Auth {
		localUser = ""
	}
	srv := mcpserver.New(st, lb, o.Home, o.PublicURL, localUser)
	var mcpHandler http.Handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true, DisableLocalhostProtection: o.Public})

	var site *http.ServeMux
	if o.Auth {
		accounts := account.New(st, o.PublicURL)
		site = viewer.New(st, lb, accounts.User, true)
		accounts.Register(site)
		mcpHandler = bearer(st, mcpHandler)
	} else {
		site = viewer.New(st, lb, viewer.Local, false)
	}

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
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
	mux.Handle("/", securityHeaders(http.NewCrossOriginProtection().Handler(site)))
	return mux
}

func bearer(st *store.Store, next http.Handler) http.Handler {
	failures := account.NewLimiter(30, time.Minute)
	verify := func(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
		u, _, err := st.TokenUser(secret.Hash(token))
		if err != nil {
			failures.Allow(account.ClientIP(r))
			return nil, fmt.Errorf("%w: invalid or revoked Folio token; create a new one on the Connect Claude page", auth.ErrInvalidToken)
		}
		return &auth.TokenInfo{UserID: u.ID}, nil
	}
	protected := auth.RequireBearerToken(verify, &auth.RequireBearerTokenOptions{AllowMissingExpiration: true})(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failures.Exceeded(account.ClientIP(r)) {
			http.Error(w, "too many failed token attempts; wait a minute", http.StatusTooManyRequests)
			return
		}
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="folio"`)
			http.Error(w, "missing Folio token: create one on the Connect Claude page, then add it as an \"Authorization: Bearer <token>\" header", http.StatusUnauthorized)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'; form-action 'self'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
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
