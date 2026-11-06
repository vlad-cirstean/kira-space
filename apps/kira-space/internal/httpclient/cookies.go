package httpclient

import (
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/net/publicsuffix"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient/cookiejar"
)

// Cookie is one cookie, sent or received (P90 item 1/2). Expires is RFC3339 or "" for a session
// cookie; SameSite is "", "lax", "strict" or "none".
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Expires  string `json:"expires"`
	MaxAge   int    `json:"maxAge"`
	Secure   bool   `json:"secure"`
	HttpOnly bool   `json:"httpOnly"`
	SameSite string `json:"sameSite"`
	// Hop is the timeline hop index this cookie was sent on or received from (0 for the first).
	Hop int `json:"hop"`
}

// jarMu guards sharedJar itself (not its own internal state, which cookiejar.Jar already
// synchronises) — ClearJar replaces the variable wholesale, and a concurrent send may be reading
// it via jarFor at the same instant.
var (
	jarMu     sync.RWMutex
	sharedJar = mustJar()
)

// mustJar builds a fresh, empty, publicsuffix-backed jar. publicsuffix.List is not optional:
// cookiejar.New(nil) accepts a Set-Cookie for Domain=com and replays it to every .com host (the
// stdlib's own documented warning). golang.org/x/net is already in go.sum as an indirect
// dependency; this promotes it to direct. BSD-3-Clause, fully open source.
func mustJar() *cookiejar.Jar {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		// cookiejar.New's only error path is a nil Options, which cannot happen here.
		panic(fmt.Sprintf("httpclient: cookie jar: %v", err))
	}
	return jar
}

// currentJar reads sharedJar under jarMu — the one accessor jarFor/JarCookies/DeleteJarCookie all
// go through, so ClearJar's wholesale replacement is never read half-torn.
func currentJar() *cookiejar.Jar {
	jarMu.RLock()
	defer jarMu.RUnlock()
	return sharedJar
}

// jarFor picks the jar one send uses — sharedJar: one app-wide, in-memory
// jar for the process's lifetime, host-keyed by the stdlib, so two environments pointing at
// different hosts can never share a session cookie; two environments pointing at the *same* host
// deliberately do, which is what a cookie jar means. Not persisted to SQLite, not per-tab, not
// per-environment.
func jarFor(r resolved) http.CookieJar {
	switch {
	case !r.useJar:
		return nil
	case r.ephemeral:
		// P71 incognito: cookies still work across this send's own redirect chain, nothing is
		// retained afterwards — a throwaway jar, never the shared one.
		return mustJar()
	default:
		return currentJar()
	}
}

// JarCookies lists what the shared jar would send to rawURL right now — Cookies tab, request
// mode (P90 item 2), each entry with its own domain, path and attributes. Hop and MaxAge stay
// zero: the jar stores neither.
func JarCookies(rawURL string) ([]Cookie, error) {
	u, err := resolveURL(rawURL)
	if err != nil {
		return nil, err
	}
	entries := currentJar().Entries(u)
	out := make([]Cookie, 0, len(entries))
	for _, e := range entries {
		c := Cookie{
			Name:     e.Name,
			Value:    e.Value,
			Domain:   e.Domain,
			Path:     e.Path,
			Secure:   e.Secure,
			HttpOnly: e.HttpOnly,
			SameSite: entrySameSite(e.SameSite),
		}
		if e.Persistent {
			c.Expires = e.Expires.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, c)
	}
	return out, nil
}

// entrySameSite maps the jar's stored attribute ("SameSite=Lax" etc.) to Cookie.SameSite.
func entrySameSite(s string) string {
	switch s {
	case "SameSite=Strict":
		return "strict"
	case "SameSite=Lax":
		return "lax"
	case "SameSite=None":
		return "none"
	default:
		return ""
	}
}

// DeleteJarCookie removes the one jar entry with exactly this domain, path and name — the three
// fields JarCookies reported, so two same-name cookies on one URL are removed independently. A
// miss (already gone) is not an error.
func DeleteJarCookie(domain, path, name string) {
	currentJar().Delete(domain, path, name)
}

// ClearJar replaces the shared jar wholesale — cookiejar has no Clear.
func ClearJar() {
	jarMu.Lock()
	defer jarMu.Unlock()
	sharedJar = mustJar()
}

// sentCookiesFromHeaderValue parses one Cookie header's value as WroteHeaderField (timeline.go)
// captured it — the jar injects Cookie inside net/http, after CheckRedirect, so there is nowhere
// else to read what actually went out.
func sentCookiesFromHeaderValue(value string, hop int) []Cookie {
	req := &http.Request{Header: http.Header{"Cookie": {value}}}
	raw := req.Cookies()
	out := make([]Cookie, 0, len(raw))
	for _, c := range raw {
		out = append(out, Cookie{Name: c.Name, Value: c.Value, Hop: hop})
	}
	return out
}

// receivedCookiesFromHeader parses every Set-Cookie in h — the stdlib's own parser, nothing
// hand-rolled.
func receivedCookiesFromHeader(h http.Header, hop int) []Cookie {
	resp := &http.Response{Header: h}
	raw := resp.Cookies()
	out := make([]Cookie, 0, len(raw))
	for _, c := range raw {
		out = append(out, Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  formatCookieExpires(c),
			MaxAge:   c.MaxAge,
			Secure:   c.Secure,
			HttpOnly: c.HttpOnly,
			SameSite: sameSiteString(c.SameSite),
			Hop:      hop,
		})
	}
	return out
}

func sameSiteString(s http.SameSite) string {
	switch s {
	case http.SameSiteLaxMode:
		return "lax"
	case http.SameSiteStrictMode:
		return "strict"
	case http.SameSiteNoneMode:
		return "none"
	default:
		return ""
	}
}

func formatCookieExpires(c *http.Cookie) string {
	if c.Expires.IsZero() {
		return ""
	}
	return c.Expires.UTC().Format("2006-01-02T15:04:05Z07:00")
}
