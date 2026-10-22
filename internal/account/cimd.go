package account

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/raaj6395/folio/internal/store"
)

const cimdTTL = time.Hour

func cimdHTTPClient(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("refusing to connect to %s", host)
			}
			if !allowPrivate && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast()) {
				return fmt.Errorf("refusing to fetch client metadata from a private address (%s)", ip)
			}
			return nil
		},
	}
	tr := &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second, Proxy: nil}
	return &http.Client{Transport: tr, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func isURLClientID(id string) bool { return strings.HasPrefix(id, "https://") }

func (a *Accounts) resolveClient(ctx context.Context, clientID string) (store.OAuthClient, error) {
	if clientID == "" {
		return store.OAuthClient{}, errors.New("client_id is missing")
	}
	if !isURLClientID(clientID) {
		c, err := a.st.OAuthClient(clientID)
		if err != nil || c.Kind != "dcr" {
			return c, errors.New("unknown client_id")
		}
		return c, nil
	}
	if c, err := a.st.OAuthClient(clientID); err == nil && c.Kind == "cimd" && time.Now().Before(c.ExpiresAt) {
		return c, nil
	}
	u, err := url.Parse(clientID)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Path == "" || u.Path == "/" || u.Fragment != "" || u.User != nil {
		return store.OAuthClient{}, errors.New("client_id must be an https URL with a path")
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", clientID, nil)
	req.Header.Set("Accept", "application/json")
	resp, err := a.cimd.Do(req)
	if err != nil {
		return store.OAuthClient{}, fmt.Errorf("could not fetch client metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return store.OAuthClient{}, fmt.Errorf("client metadata returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10+1))
	if err != nil || len(body) > 64<<10 {
		return store.OAuthClient{}, errors.New("client metadata is too large or unreadable")
	}
	var doc struct {
		ClientID                string   `json:"client_id"`
		ClientName              string   `json:"client_name"`
		RedirectURIs            []string `json:"redirect_uris"`
		TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return store.OAuthClient{}, errors.New("client metadata is not valid JSON")
	}
	if doc.ClientID != clientID {
		return store.OAuthClient{}, errors.New("client metadata client_id does not match its URL")
	}
	if len(doc.RedirectURIs) == 0 || strings.TrimSpace(doc.ClientName) == "" {
		return store.OAuthClient{}, errors.New("client metadata needs client_name and redirect_uris")
	}
	if m := doc.TokenEndpointAuthMethod; m != "" && m != "none" {
		return store.OAuthClient{}, fmt.Errorf("token_endpoint_auth_method %q is not supported (only public clients)", m)
	}
	for _, r := range doc.RedirectURIs {
		if !validRedirect(r) {
			return store.OAuthClient{}, fmt.Errorf("redirect_uri %q is not allowed", r)
		}
		if ru, _ := url.Parse(r); !isLoopback(ru) && (ru.Scheme != u.Scheme || !strings.EqualFold(ru.Host, u.Host)) {
			return store.OAuthClient{}, fmt.Errorf("redirect_uri %q must be on the same origin as the client_id URL", r)
		}
	}
	name := doc.ClientName
	if len(name) > 80 {
		name = name[:80]
	}
	c := store.OAuthClient{ID: clientID, Name: name, RedirectURIs: doc.RedirectURIs, Kind: "cimd", ExpiresAt: time.Now().Add(cimdTTL)}
	return c, a.st.SaveOAuthClient(c)
}

func isLoopback(u *url.URL) bool {
	h := u.Hostname()
	return u.Scheme == "http" && (h == "localhost" || h == "127.0.0.1" || h == "::1")
}

func redirectAllowed(registered []string, requested string) bool {
	req, err := url.Parse(requested)
	if err != nil || req.Fragment != "" || req.User != nil {
		return false
	}
	for _, r := range registered {
		if r == requested {
			return true
		}
		reg, err := url.Parse(r)
		if err != nil || !isLoopback(reg) || !isLoopback(req) {
			continue
		}
		if reg.Hostname() == req.Hostname() && reg.Path == req.Path && reg.RawQuery == req.RawQuery {
			return true
		}
	}
	return false
}

func onlyLoopback(uris []string) bool {
	for _, r := range uris {
		if u, err := url.Parse(r); err != nil || !isLoopback(u) {
			return false
		}
	}
	return len(uris) > 0
}

func validRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.User != nil || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		h := u.Hostname()
		return h == "localhost" || h == "127.0.0.1" || h == "::1"
	}
	return false
}
