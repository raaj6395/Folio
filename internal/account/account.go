package account

import (
	_ "embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/raaj6395/folio/internal/secret"
	"github.com/raaj6395/folio/internal/store"
)

const (
	cookieName = "folio_session"
	sessionTTL = 30 * 24 * time.Hour
)

//go:embed page.html
var pageHTML string

var pageTmpl = template.Must(template.New("page").Parse(pageHTML))

type Accounts struct {
	st        *store.Store
	publicURL string
	signup    *Limiter
	login     *Limiter
	tokens    *Limiter
}

func New(st *store.Store, publicURL string) *Accounts {
	return &Accounts{
		st:        st,
		publicURL: strings.TrimSuffix(publicURL, "/"),
		signup:    NewLimiter(5, time.Hour),
		login:     NewLimiter(10, 15*time.Minute),
		tokens:    NewLimiter(20, time.Hour),
	}
}

func (a *Accounts) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /signup", a.form("signup"))
	mux.HandleFunc("POST /signup", a.doSignup)
	mux.HandleFunc("GET /login", a.form("login"))
	mux.HandleFunc("POST /login", a.doLogin)
	mux.HandleFunc("POST /logout", a.doLogout)
	mux.HandleFunc("GET /connect", a.connect)
	mux.HandleFunc("POST /connect/tokens", a.createToken)
	mux.HandleFunc("POST /connect/tokens/{id}/revoke", a.revokeToken)
}

func (a *Accounts) User(r *http.Request) (string, string) {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return "", ""
	}
	u, err := a.st.SessionUser(secret.Hash(c.Value))
	if err != nil {
		return "", ""
	}
	return u.ID, u.Email
}

type page struct {
	Title, Page, Error, Next, EmailInput, Email string
	NewToken, MCPURL                            string
	Tokens                                      []store.Token
}

func render(w http.ResponseWriter, status int, p page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := pageTmpl.Execute(w, p); err != nil {
		slog.Error("account page", "err", err)
	}
}

func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

func (a *Accounts) form(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if id, _ := a.User(r); id != "" {
			http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusSeeOther)
			return
		}
		title := map[string]string{"login": "Log in", "signup": "Sign up"}[kind]
		render(w, http.StatusOK, page{Title: title, Page: kind, Next: safeNext(r.URL.Query().Get("next"))})
	}
}

func (a *Accounts) startSession(w http.ResponseWriter, r *http.Request, userID string) error {
	plain, hash := secret.NewSessionID()
	if err := a.st.CreateSession(userID, hash, sessionTTL); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: plain, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
	return nil
}

func (a *Accounts) doSignup(w http.ResponseWriter, r *http.Request) {
	email, password, next := strings.TrimSpace(r.FormValue("email")), r.FormValue("password"), safeNext(r.FormValue("next"))
	fail := func(status int, msg string) {
		render(w, status, page{Title: "Sign up", Page: "signup", Error: msg, Next: next, EmailInput: email})
	}
	if os.Getenv("FOLIO_SIGNUP") == "closed" {
		fail(http.StatusForbidden, "Sign-ups are closed right now.")
		return
	}
	if !a.signup.Allow(ClientIP(r)) {
		fail(http.StatusTooManyRequests, "Too many sign-ups from your network. Try again later.")
		return
	}
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || !strings.Contains(email[strings.Index(email, "@"):], ".") {
		fail(http.StatusBadRequest, "Enter a valid email address.")
		return
	}
	if len(password) < 10 || len(password) > 256 {
		fail(http.StatusBadRequest, "Password must be 10 to 256 characters.")
		return
	}
	u, err := a.st.CreateUser(email, secret.HashPassword(password))
	if errors.Is(err, store.ErrEmailTaken) {
		fail(http.StatusConflict, "That email already has an account. Log in instead.")
		return
	}
	if err != nil {
		slog.Error("signup", "err", err)
		fail(http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}
	slog.Info("user signed up", "user", u.ID)
	if err := a.startSession(w, r, u.ID); err != nil {
		slog.Error("session", "err", err)
	}
	if next == "/" {
		next = "/connect"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *Accounts) doLogin(w http.ResponseWriter, r *http.Request) {
	email, password, next := strings.TrimSpace(r.FormValue("email")), r.FormValue("password"), safeNext(r.FormValue("next"))
	fail := func(status int, msg string) {
		render(w, status, page{Title: "Log in", Page: "login", Error: msg, Next: next, EmailInput: email})
	}
	if !a.login.Allow(ClientIP(r) + "|" + store.NormalizeEmail(email)) {
		fail(http.StatusTooManyRequests, "Too many attempts. Wait a few minutes and try again.")
		return
	}
	u, err := a.st.UserByEmail(email)
	if err != nil || u.Disabled {
		secret.CheckPassword("!", password)
		fail(http.StatusUnauthorized, "Invalid email or password.")
		return
	}
	if !secret.CheckPassword(u.PasswordHash, password) {
		fail(http.StatusUnauthorized, "Invalid email or password.")
		return
	}
	if err := a.startSession(w, r, u.ID); err != nil {
		slog.Error("session", "err", err)
		fail(http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *Accounts) doLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		a.st.DeleteSession(secret.Hash(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (a *Accounts) loggedIn(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	id, email := a.User(r)
	if id == "" {
		http.Redirect(w, r, "/login?next=/connect", http.StatusSeeOther)
		return "", "", false
	}
	return id, email, true
}

func (a *Accounts) connectPage(w http.ResponseWriter, userID, email, newToken string) {
	tokens, err := a.st.Tokens(userID)
	if err != nil {
		slog.Error("tokens", "err", err)
	}
	render(w, http.StatusOK, page{Title: "Connect Claude", Page: "connect", Email: email, Tokens: tokens, NewToken: newToken, MCPURL: a.publicURL + "/mcp"})
}

func (a *Accounts) connect(w http.ResponseWriter, r *http.Request) {
	if id, email, ok := a.loggedIn(w, r); ok {
		a.connectPage(w, id, email, "")
	}
}

func (a *Accounts) createToken(w http.ResponseWriter, r *http.Request) {
	id, email, ok := a.loggedIn(w, r)
	if !ok {
		return
	}
	if !a.tokens.Allow(id) {
		http.Error(w, "too many tokens created recently; try again later", http.StatusTooManyRequests)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = "Claude " + time.Now().Format("2006-01-02")
	}
	if len(name) > 60 {
		name = name[:60]
	}
	plain, hash, last4 := secret.NewToken()
	if _, err := a.st.CreateToken(id, name, hash, last4); err != nil {
		slog.Error("create token", "err", err)
		http.Error(w, "could not create token", http.StatusInternalServerError)
		return
	}
	a.connectPage(w, id, email, plain)
}

func (a *Accounts) revokeToken(w http.ResponseWriter, r *http.Request) {
	id, _, ok := a.loggedIn(w, r)
	if !ok {
		return
	}
	if err := a.st.RevokeToken(id, r.PathValue("id")); err != nil && !errors.Is(err, store.ErrNotFound) {
		slog.Error("revoke token", "err", err)
	}
	http.Redirect(w, r, "/connect", http.StatusSeeOther)
}
