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
	Home       string
	PublicURL  string
	Public     bool
	Auth       bool
	ClientHTTP *http.Client
}

func Handler(st *store.Store, lb *layout.Builder, o Options) http.Handler {
	localUser := store.LocalUser
	if o.Auth {
		localUser = ""
	}
	srv := mcpserver.New(st, lb, o.Home, o.PublicURL, localUser)
	var mcpHandler http.Handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true, DisableLocalhostProtection: o.Public})

	mux := http.NewServeMux()
	var site *http.ServeMux
	if o.Auth {
		accounts := account.New(st, o.PublicURL, o.ClientHTTP)
		site = viewer.New(st, lb, accounts.User, true)
		accounts.Register(site)
		accounts.RegisterOAuth(site, mux)
		mcpHandler = bearer(st, accounts.ResourceMetadataURL(), mcpHandler)
	} else {
		site = viewer.New(st, lb, viewer.Local, false)
	}

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

func bearer(st *store.Store, metadataURL string, next http.Handler) http.Handler {
	failures := account.NewLimiter(30, time.Minute)
	verify := func(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
		hash := secret.Hash(token)
		u, _, err := st.TokenUser(hash)
		if err != nil {
			failures.Allow(account.ClientIP(r))
			return nil, fmt.Errorf("%w: invalid, expired or revoked Folio token", auth.ErrInvalidToken)
		}
		return &auth.TokenInfo{UserID: u.ID, Expiration: st.TokenExpiry(hash)}, nil
	}
	protected := auth.RequireBearerToken(verify, &auth.RequireBearerTokenOptions{AllowMissingExpiration: true, ResourceMetadataURL: metadataURL})(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failures.Exceeded(account.ClientIP(r)) {
			http.Error(w, "too many failed token attempts; wait a minute", http.StatusTooManyRequests)
			return
		}
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+metadataURL+`"`)
			http.Error(w, "sign-in required: connect Folio in Claude (it will open the Folio login), or send a personal token as \"Authorization: Bearer <token>\"", http.StatusUnauthorized)
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
		h.Set("Content-Security-Policy", account.CSP("'self'"))
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
