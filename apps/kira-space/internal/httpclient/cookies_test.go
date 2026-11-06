package httpclient

import (
	"net/http"
	"net/url"
	"testing"
)

func freshJar(t *testing.T) {
	t.Helper()
	old := currentJar()
	ClearJar()
	t.Cleanup(func() { jarMu.Lock(); sharedJar = old; jarMu.Unlock() })
}

// TestJarCookiesCarriesDomainPathAndAttributes guards P175 D2: the listing reports each entry's
// own domain, path and attributes, the ids DeleteJarCookie takes.
func TestJarCookiesCarriesDomainPathAndAttributes(t *testing.T) {
	freshJar(t)
	setU, _ := url.Parse("https://api.example.com/login")
	currentJar().SetCookies(setU, []*http.Cookie{
		{Name: "sid", Value: "1", Domain: "example.com", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 3600},
	})
	got, err := JarCookies("https://api.example.com/x")
	if err != nil || len(got) != 1 {
		t.Fatalf("JarCookies = %+v, %v", got, err)
	}
	c := got[0]
	if c.Domain != "example.com" || c.Path != "/" || !c.Secure || !c.HttpOnly || c.SameSite != "lax" || c.Expires == "" {
		t.Fatalf("cookie = %+v", c)
	}
}

// TestDeleteJarCookieIsExact guards P168 Part 6 F12: two same-name cookies matching one URL are
// removed independently, and a parent-domain cookie goes by its own domain.
func TestDeleteJarCookieIsExact(t *testing.T) {
	freshJar(t)
	setU, _ := url.Parse("https://api.example.com/a/b")
	currentJar().SetCookies(setU, []*http.Cookie{
		{Name: "sid", Value: "root", Path: "/"},
		{Name: "sid", Value: "deep", Path: "/a"},
		{Name: "sid", Value: "parent", Domain: "example.com", Path: "/"},
	})
	target := "https://api.example.com/a/b"
	DeleteJarCookie("api.example.com", "/a", "sid")
	got, _ := JarCookies(target)
	if len(got) != 2 {
		t.Fatalf("after deleting /a: %+v", got)
	}
	DeleteJarCookie("example.com", "/", "sid")
	got, _ = JarCookies(target)
	if len(got) != 1 || got[0].Value != "root" {
		t.Fatalf("after deleting parent domain: %+v", got)
	}
}
