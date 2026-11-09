package apiaccess

import (
	"net/url"
	"strings"
	"testing"
)

func TestPolicyCheck(t *testing.T) {
	local := Rule{"allow", []string{"127.0.0.1"}, []string{"GET"}}
	wild := Rule{"allow", []string{"*.example.com"}, []string{"*"}}
	noDelete := Rule{"deny", []string{"*"}, []string{"DELETE"}}
	cases := []struct {
		name   string
		rules  []Rule
		method string
		url    string
		want   string // "" allowed, else substring of the error
	}{
		{"empty allows all", nil, "DELETE", "https://anything.test/x", ""},
		{"allow match", []Rule{local}, "GET", "http://127.0.0.1:1234/echo", ""},
		{"method not listed", []Rule{local}, "POST", "http://127.0.0.1:1234/echo", "not allowed by the API access rules"},
		{"error lists allow rules", []Rule{local, wild}, "POST", "http://127.0.0.1:1234/echo", "Allowed: GET on 127.0.0.1; any method on *.example.com"},
		{"error hides path and query", []Rule{local}, "POST", "http://127.0.0.1:1234/secret?token=abc", "POST http://127.0.0.1:1234 is not"},
		{"wildcard matches subdomain", []Rule{wild}, "PUT", "https://a.b.example.com/", ""},
		{"wildcard skips apex", []Rule{wild}, "GET", "https://example.com/", "not allowed"},
		{"wildcard skips suffix lookalike", []Rule{wild}, "GET", "https://badexample.com/", "not allowed"},
		{"deny beats allow", []Rule{local, wild, noDelete}, "DELETE", "https://api.example.com/x", `denied by the API access rule "deny DELETE on *"`},
		{"deny leaves other methods", []Rule{wild, noDelete}, "GET", "https://api.example.com/x", ""},
		{"deny-only refuses all", []Rule{noDelete}, "GET", "https://api.example.com/x", "Allowed: nothing"},
		{"any host token", []Rule{{"allow", []string{"*"}, []string{"GET", "HEAD"}}}, "HEAD", "https://x.test/", ""},
		{"port pinned matches", []Rule{{"allow", []string{"127.0.0.1:8080"}, []string{"*"}}}, "GET", "http://127.0.0.1:8080/", ""},
		{"port pinned mismatch", []Rule{{"allow", []string{"127.0.0.1:8080"}, []string{"*"}}}, "GET", "http://127.0.0.1:9090/", "not allowed"},
		{"no port matches any port", []Rule{{"allow", []string{"127.0.0.1"}, []string{"*"}}}, "GET", "http://127.0.0.1:9090/", ""},
		{"default https port", []Rule{{"allow", []string{"api.test:443"}, []string{"*"}}}, "GET", "https://api.test/", ""},
		{"idna host", []Rule{{"allow", []string{"bücher.example"}, []string{"*"}}}, "GET", "https://xn--bcher-kva.example/", ""},
		{"ipv6 literal", []Rule{{"allow", []string{"[::1]:80"}, []string{"*"}}}, "GET", "http://[::1]/", ""},
		{"case insensitive host", []Rule{{"allow", []string{"API.Test"}, []string{"*"}}}, "GET", "https://api.TEST/", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Compile(c.rules)
			if err != nil {
				t.Fatal(err)
			}
			u, _ := url.Parse(c.url)
			err = p.Check(c.method, u)
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("want allowed, got %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
			if err != nil && strings.Contains(err.Error(), "token=abc") {
				t.Fatalf("error leaks query: %v", err)
			}
		})
	}
}

func TestCompileRejects(t *testing.T) {
	ok := Rule{"allow", []string{"a.test"}, []string{"GET"}}
	many := func(n int) []string {
		s := make([]string, n)
		for i := range s {
			s[i] = "a.test"
		}
		return s
	}
	cases := []struct {
		name  string
		rules []Rule
		want  string
	}{
		{"bad effect", []Rule{{"maybe", ok.Hosts, ok.Methods}}, "rule 1: effect"},
		{"http prefix", []Rule{ok, {"allow", []string{"http://a.test"}, []string{"GET"}}}, `rule 2: host "http://a.test"`},
		{"https prefix", []Rule{{"allow", []string{"https://a.test"}, []string{"GET"}}}, "host"},
		{"path", []Rule{{"allow", []string{"a/b"}, []string{"GET"}}}, `host "a/b"`},
		{"empty hosts", []Rule{{"allow", nil, []string{"GET"}}}, "hosts"},
		{"too many hosts", []Rule{{"allow", many(21), []string{"GET"}}}, "hosts"},
		{"empty methods", []Rule{{"allow", ok.Hosts, nil}}, "methods"},
		{"lowercase method", []Rule{{"allow", ok.Hosts, []string{"get"}}}, `method "get"`},
		{"duplicate method", []Rule{{"allow", ok.Hosts, []string{"GET", "GET"}}}, "listed twice"},
		{"star mixed", []Rule{{"allow", ok.Hosts, []string{"*", "GET"}}}, `method "*"`},
		{"too many rules", func() []Rule {
			r := make([]Rule, 51)
			for i := range r {
				r[i] = ok
			}
			return r
		}(), "at most 50"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Compile(c.rules); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}
