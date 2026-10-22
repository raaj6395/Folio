package account

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/secret"
	"github.com/raaj6395/folio/internal/store"
)

const (
	accessTTL = time.Hour
	codeTTL   = 10 * time.Minute
)

func (a *Accounts) ResourceURL() string         { return a.publicURL + "/mcp" }
func (a *Accounts) ResourceMetadataURL() string { return a.publicURL + "/.well-known/oauth-protected-resource/mcp" }

func (a *Accounts) RegisterOAuth(site, open *http.ServeMux) {
	prm := auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               a.ResourceURL(),
		AuthorizationServers:   []string{a.publicURL},
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "Folio",
	})
	open.Handle("/.well-known/oauth-protected-resource", prm)
	open.Handle("/.well-known/oauth-protected-resource/mcp", prm)
	open.HandleFunc("/.well-known/oauth-authorization-server", a.serverMetadata)
	open.HandleFunc("POST /oauth/register", a.register)
	open.HandleFunc("POST /oauth/token", a.token)
	site.HandleFunc("GET /oauth/authorize", a.authorize)
	site.HandleFunc("POST /oauth/authorize", a.authorize)
	site.HandleFunc("POST /connect/apps/{id}/revoke", a.revokeGrant)
}

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, MCP-Protocol-Version")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	cors(w)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func oauthError(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

func (a *Accounts) serverMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		cors(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                         a.publicURL,
		"authorization_endpoint":                         a.publicURL + "/oauth/authorize",
		"token_endpoint":                                 a.publicURL + "/oauth/token",
		"registration_endpoint":                          a.publicURL + "/oauth/register",
		"response_types_supported":                       []string{"code"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"client_id_metadata_document_supported":          true,
		"authorization_response_iss_parameter_supported": true,
		"service_documentation":                          a.publicURL + "/connect",
	})
}

func (a *Accounts) register(w http.ResponseWriter, r *http.Request) {
	if !a.registrations.Allow(ClientIP(r)) {
		oauthError(w, http.StatusTooManyRequests, "invalid_request", "too many registrations; try again later")
		return
	}
	var meta oauthex.ClientRegistrationMetadata
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&meta); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "body must be JSON client metadata")
		return
	}
	if len(meta.RedirectURIs) == 0 || len(meta.RedirectURIs) > 10 {
		oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "1 to 10 redirect_uris are required")
		return
	}
	for _, u := range meta.RedirectURIs {
		if !validRedirect(u) {
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris must be https (or http on localhost): "+u)
			return
		}
	}
	name := strings.TrimSpace(meta.ClientName)
	if name == "" {
		name = "MCP client"
	}
	if len(name) > 80 {
		name = name[:80]
	}
	c := store.OAuthClient{ID: model.NewID("fc_") + model.NewID(""), Name: name, RedirectURIs: meta.RedirectURIs, Kind: "dcr"}
	if err := a.st.SaveOAuthClient(c); err != nil {
		slog.Error("register client", "err", err)
		oauthError(w, http.StatusInternalServerError, "server_error", "could not register client")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  c.ID,
		"client_id_issued_at":        time.Now().Unix(),
		"client_name":                c.Name,
		"redirect_uris":              c.RedirectURIs,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
}

func (a *Accounts) validResource(res string) bool {
	if res == "" {
		return true
	}
	norm := func(s string) string {
		u, err := url.Parse(strings.TrimSuffix(s, "/"))
		if err != nil {
			return ""
		}
		u.Scheme, u.Host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
		return u.String()
	}
	got := norm(res)
	return got != "" && (got == norm(a.ResourceURL()) || got == norm(a.publicURL))
}

type authzRequest struct {
	ClientID, RedirectURI, State, Challenge, Method, Resource, Scope, ResponseType string
}

func (a *Accounts) redirectWith(w http.ResponseWriter, r *http.Request, redirectURI string, params map[string]string) {
	u, _ := url.Parse(redirectURI)
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	q.Set("iss", a.publicURL)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (a *Accounts) authorize(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	q := authzRequest{
		ClientID: r.Form.Get("client_id"), RedirectURI: r.Form.Get("redirect_uri"), State: r.Form.Get("state"),
		Challenge: r.Form.Get("code_challenge"), Method: r.Form.Get("code_challenge_method"), Resource: r.Form.Get("resource"),
		Scope: r.Form.Get("scope"), ResponseType: r.Form.Get("response_type"),
	}
	fail := func(msg string) {
		render(w, http.StatusBadRequest, page{Title: "Can't connect", Page: "oauth_error", Error: msg})
	}
	client, err := a.resolveClient(r.Context(), q.ClientID)
	if err != nil {
		fail("This app couldn't be verified: " + err.Error())
		return
	}
	if q.RedirectURI == "" && len(client.RedirectURIs) == 1 {
		q.RedirectURI = client.RedirectURIs[0]
	}
	if !redirectAllowed(client.RedirectURIs, q.RedirectURI) {
		fail("The app asked to return to an address it didn't register.")
		return
	}
	bounce := func(code, desc string) {
		a.redirectWith(w, r, q.RedirectURI, map[string]string{"error": code, "error_description": desc, "state": q.State})
	}
	if q.ResponseType != "code" {
		bounce("unsupported_response_type", "only response_type=code is supported")
		return
	}
	if q.Challenge == "" || q.Method != "S256" {
		bounce("invalid_request", "PKCE with code_challenge_method=S256 is required")
		return
	}
	if !a.validResource(q.Resource) {
		bounce("invalid_target", "resource must be "+a.ResourceURL())
		return
	}
	userID, email := a.User(r)
	if userID == "" {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path+"?"+r.Form.Encode()), http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodGet {
		appHost := "unverified app"
		if isURLClientID(client.ID) {
			appHost = hostOf(client.ID)
		}
		render(w, http.StatusOK, page{Title: "Allow access", Page: "consent", Email: email, ClientName: client.Name, ClientHost: appHost,
			RedirectHost: hostOf(q.RedirectURI), LoopbackOnly: onlyLoopback(client.RedirectURIs), Form: r.Form})
		return
	}
	if r.Form.Get("decision") != "allow" {
		bounce("access_denied", "the user declined")
		return
	}
	code, hash, _ := secret.NewOpaque("folio_code_")
	resource := q.Resource
	if resource == "" {
		resource = a.ResourceURL()
	}
	if err := a.st.CreateCode(hash, store.Code{ClientID: client.ID, UserID: userID, RedirectURI: q.RedirectURI, Challenge: q.Challenge, Resource: resource}, codeTTL); err != nil {
		slog.Error("create code", "err", err)
		bounce("server_error", "could not create authorization code")
		return
	}
	slog.Info("oauth consent granted", "user", userID, "client", client.ID)
	a.redirectWith(w, r, q.RedirectURI, map[string]string{"code": code, "state": q.State})
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Host
	}
	return ""
}

func (a *Accounts) issue(w http.ResponseWriter, userID, grantID, refresh string) {
	access, hash, last4 := secret.NewOpaque("folio_oat_")
	if _, err := a.st.CreateAccessToken(userID, grantID, hash, last4, accessTTL); err != nil {
		slog.Error("access token", "err", err)
		oauthError(w, http.StatusInternalServerError, "server_error", "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    int(accessTTL.Seconds()),
		"refresh_token": refresh,
	})
}

func (a *Accounts) token(w http.ResponseWriter, r *http.Request) {
	if !a.tokenCalls.Allow(ClientIP(r)) {
		oauthError(w, http.StatusTooManyRequests, "invalid_request", "too many token requests")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "form body required")
		return
	}
	clientID := r.PostForm.Get("client_id")
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		code, err := a.st.ConsumeCode(secret.Hash(r.PostForm.Get("code")))
		if errors.Is(err, store.ErrNotFound) {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "authorization code is invalid, expired or already used")
			return
		}
		if err != nil {
			oauthError(w, http.StatusInternalServerError, "server_error", "could not read code")
			return
		}
		if code.ClientID != clientID || code.RedirectURI != r.PostForm.Get("redirect_uri") {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "client_id or redirect_uri does not match the authorization request")
			return
		}
		if !secret.PKCEMatches(r.PostForm.Get("code_verifier"), code.Challenge) {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "code_verifier does not match code_challenge")
			return
		}
		if res := r.PostForm.Get("resource"); res != "" && !a.validResource(res) {
			oauthError(w, http.StatusBadRequest, "invalid_target", "resource must be "+a.ResourceURL())
			return
		}
		name := code.ClientID
		if c, err := a.st.OAuthClient(code.ClientID); err == nil {
			name = c.Name
		}
		refresh, refreshHash, _ := secret.NewOpaque("folio_ort_")
		g, err := a.st.CreateGrant(code.UserID, code.ClientID, name, refreshHash)
		if err != nil {
			slog.Error("grant", "err", err)
			oauthError(w, http.StatusInternalServerError, "server_error", "could not create grant")
			return
		}
		a.issue(w, code.UserID, g.ID, refresh)
	case "refresh_token":
		refresh, refreshHash, _ := secret.NewOpaque("folio_ort_")
		userID, grantID, err := a.st.RotateRefresh(secret.Hash(r.PostForm.Get("refresh_token")), refreshHash, clientID)
		if err != nil {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "refresh token is invalid or revoked")
			return
		}
		a.issue(w, userID, grantID, refresh)
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "use authorization_code or refresh_token")
	}
}

func (a *Accounts) revokeGrant(w http.ResponseWriter, r *http.Request) {
	id, _, ok := a.loggedIn(w, r)
	if !ok {
		return
	}
	if err := a.st.RevokeGrant(id, r.PathValue("id")); err != nil && !errors.Is(err, store.ErrNotFound) {
		slog.Error("revoke grant", "err", err)
	}
	http.Redirect(w, r, "/connect", http.StatusSeeOther)
}
