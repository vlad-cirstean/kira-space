package apifiles

import (
	"net/url"
	"testing"
)

func TestHostRuleAllows(t *testing.T) {
	cases := []struct {
		rule, url string
		want      bool
	}{
		{"api.example.com", "https://api.example.com/x", true},
		{"API.Example.com", "https://api.EXAMPLE.com/x", true},
		{"api.example.com", "https://other.example.com/x", false},
		{"api.example.com", "https://api.example.com.evil.io/x", false},
		{"api.example.com", "https://sub.api.example.com/x", false},
		{"*.example.com", "https://a.example.com", true},
		{"*.example.com", "https://a.b.example.com", true},
		{"*.example.com", "https://example.com", false},
		{"*.example.com", "https://badexample.com", false},
		{"*.example.com", "https://a.example.com.evil.io", false},
		{"api.example.com:8443", "https://api.example.com:8443/", true},
		{"api.example.com:8443", "https://api.example.com/", false},
		{"api.example.com:443", "https://api.example.com/", true},
		{"api.example.com", "https://api.example.com:9999/", true},
		{"api.example.com", "http://api.example.com/", false},
		{"http://api.example.com", "http://api.example.com/", true},
		{"http://api.example.com:8080", "http://api.example.com:8080/", true},
		{"http://api.example.com:8080", "http://api.example.com/", false},
		{"127.0.0.1", "http://127.0.0.1:8080/", true},
		{"127.0.0.1", "http://localhost:8080/", false},
		{"localhost", "http://localhost/", true},
		{"[::1]:8080", "http://[::1]:8080/x", true},
		{"[::1]", "http://[::1]:9/x", true},
		{"bücher.example", "https://xn--bcher-kva.example/", true},
		{"xn--bcher-kva.example", "https://bücher.example/", true},
		{"api.example.com", "ftp://api.example.com/", false},
	}
	for _, c := range cases {
		r, err := ParseHostRule(c.rule)
		if err != nil {
			t.Fatalf("parse %q: %v", c.rule, err)
		}
		u, err := url.Parse(c.url)
		if err != nil {
			t.Fatalf("url %q: %v", c.url, err)
		}
		if got := r.Allows(u); got != c.want {
			t.Errorf("%q allows %q = %v, want %v", c.rule, c.url, got, c.want)
		}
	}
}

func TestParseHostRuleRejects(t *testing.T) {
	for _, s := range []string{"", "*", "*.", "a.com/x", "a.com?x", "u@a.com", "https://a.com", "a.com:0", "a.com:99999", "a.com:", "*.127.0.0.1", "a b.com", "::1", "[::1"} {
		if _, err := ParseHostRule(s); err == nil {
			t.Errorf("%q parsed, want an error", s)
		}
	}
}
