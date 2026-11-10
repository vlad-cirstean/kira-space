package apiaccess

import (
	"net/url"
	"strings"
	"testing"
)

func TestPolicyDecide(t *testing.T) {
	local := Rule{"allow", []string{"127.0.0.1"}, []string{"GET"}}
	wild := Rule{"allow", []string{"*.example.com"}, []string{"*"}}
	noDelete := Rule{"deny", []string{"*"}, []string{"DELETE"}}
	const (
		allowed   = "allowed"
		unmatched = "unmatched"
	)
	cases := []struct {
		name    string
		rules   []Rule
		def     string // "" is the zero policy, which asks
		method  string
		url     string
		verdict string // allowed, unmatched, or a substring of the error
	}{
		{"no rules ask", nil, "", "DELETE", "https://anything.test/x", unmatched},
		{"no rules ask explicit", nil, "ask", "DELETE", "https://anything.test/x", unmatched},
		{"no rules deny", nil, "deny", "GET", "https://anything.test/x", "no rule matches it"},
		{"no rules deny names nothing allowed", nil, "deny", "GET", "https://anything.test/x", "Allowed: nothing, there is no allow rule"},
		{"deny-only ask passes other methods", []Rule{noDelete}, "ask", "GET", "https://api.example.com/x", unmatched},
		{"deny-only ask refuses match", []Rule{noDelete}, "ask", "DELETE", "https://api.example.com/x", "denied by the API access rule"},
		{"deny-only deny refuses", []Rule{noDelete}, "deny", "GET", "https://api.example.com/x", "no rule matches"},
		{"allow match ask", []Rule{local}, "ask", "GET", "http://127.0.0.1:1234/echo", allowed},
		{"allow match deny", []Rule{local}, "deny", "GET", "http://127.0.0.1:1234/echo", allowed},
		{"unmatched ask", []Rule{local}, "ask", "POST", "http://127.0.0.1:1234/echo", unmatched},
		{"method not listed deny", []Rule{local}, "deny", "POST", "http://127.0.0.1:1234/echo", "not allowed by the API access rules"},
		{"error lists allow rules", []Rule{local, wild}, "deny", "POST", "http://127.0.0.1:1234/echo", "Allowed: GET on 127.0.0.1; any method on *.example.com"},
		{"error hides path and query", []Rule{local}, "deny", "POST", "http://127.0.0.1:1234/secret?token=abc", "POST http://127.0.0.1:1234 is not"},
		{"wildcard matches subdomain", []Rule{wild}, "deny", "PUT", "https://a.b.example.com/", allowed},
		{"wildcard skips apex", []Rule{wild}, "deny", "GET", "https://example.com/", "not allowed"},
		{"wildcard skips suffix lookalike", []Rule{wild}, "deny", "GET", "https://badexample.com/", "not allowed"},
		{"deny beats allow deny default", []Rule{local, wild, noDelete}, "deny", "DELETE", "https://api.example.com/x", `denied by the API access rule "deny DELETE on *"`},
		{"deny beats allow ask default", []Rule{local, wild, noDelete}, "ask", "DELETE", "https://api.example.com/x", `denied by the API access rule "deny DELETE on *"`},
		{"deny leaves other methods", []Rule{wild, noDelete}, "deny", "GET", "https://api.example.com/x", allowed},
		{"deny-only deny names nothing allowed", []Rule{noDelete}, "deny", "GET", "https://api.example.com/x", "Allowed: nothing"},
		{"any host token", []Rule{{"allow", []string{"*"}, []string{"GET", "HEAD"}}}, "deny", "HEAD", "https://x.test/", allowed},
		{"port pinned matches", []Rule{{"allow", []string{"127.0.0.1:8080"}, []string{"*"}}}, "deny", "GET", "http://127.0.0.1:8080/", allowed},
		{"port pinned mismatch", []Rule{{"allow", []string{"127.0.0.1:8080"}, []string{"*"}}}, "deny", "GET", "http://127.0.0.1:9090/", "not allowed"},
		{"localhost port matches", []Rule{{"allow", []string{"localhost:3000"}, []string{"*"}}}, "deny", "GET", "http://localhost:3000/", allowed},
		{"localhost other port", []Rule{{"allow", []string{"localhost:3000"}, []string{"*"}}}, "deny", "GET", "http://localhost:3001/", "not allowed"},
		{"no port matches any port", []Rule{{"allow", []string{"127.0.0.1"}, []string{"*"}}}, "deny", "GET", "http://127.0.0.1:9090/", allowed},
		{"default https port", []Rule{{"allow", []string{"api.test:443"}, []string{"*"}}}, "deny", "GET", "https://api.test/", allowed},
		{"idna host", []Rule{{"allow", []string{"bücher.example"}, []string{"*"}}}, "deny", "GET", "https://xn--bcher-kva.example/", allowed},
		{"ipv6 literal", []Rule{{"allow", []string{"[::1]:80"}, []string{"*"}}}, "deny", "GET", "http://[::1]/", allowed},
		{"case insensitive host", []Rule{{"allow", []string{"API.Test"}, []string{"*"}}}, "deny", "GET", "https://api.TEST/", allowed},
		{"unknown default fails closed", []Rule{local}, "bogus", "POST", "http://127.0.0.1:1234/echo", "not allowed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Compile(c.rules)
			if err != nil {
				t.Fatal(err)
			}
			if c.def != "" {
				p = p.WithDefault(c.def)
			}
			u, _ := url.Parse(c.url)
			v, err := p.Decide(c.method, u)
			switch c.verdict {
			case allowed, unmatched:
				if err != nil {
					t.Fatalf("want %s, got %v", c.verdict, err)
				}
				if (v == VerdictAllowed) != (c.verdict == allowed) {
					t.Fatalf("verdict = %v, want %s", v, c.verdict)
				}
			default:
				if err == nil || !strings.Contains(err.Error(), c.verdict) {
					t.Fatalf("want error containing %q, got %v", c.verdict, err)
				}
			}
			if err != nil && strings.Contains(err.Error(), "token=abc") {
				t.Fatalf("error leaks query: %v", err)
			}
		})
	}
}

func TestPolicyAllowsAll(t *testing.T) {
	deny := Rule{"deny", []string{"*"}, []string{"DELETE"}}
	for _, c := range []struct {
		rules []Rule
		def   string
		want  bool
	}{
		{nil, "ask", true},
		{nil, "deny", false},
		{[]Rule{deny}, "ask", false},
		{[]Rule{deny}, "deny", false},
	} {
		p, err := Compile(c.rules)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.WithDefault(c.def).AllowsAll(); got != c.want {
			t.Errorf("rules %d default %s: AllowsAll = %v, want %v", len(c.rules), c.def, got, c.want)
		}
	}
	if !(Policy{}).AllowsAll() {
		t.Error("zero policy must allow all")
	}
}

func TestPolicyCheckHop(t *testing.T) {
	local := Rule{"allow", []string{"127.0.0.1"}, []string{"GET"}}
	noDelete := Rule{"deny", []string{"127.0.0.1"}, []string{"DELETE"}}
	cases := []struct {
		name   string
		rules  []Rule
		def    string
		method string
		url    string
		want   string // "" follows
	}{
		{"no rules ask follows any host", nil, "ask", "GET", "https://anywhere.test/x", ""},
		{"no rules deny refuses", nil, "deny", "GET", "https://anywhere.test/x", "a redirect must go to a host an allow rule matches"},
		{"ask allow match follows", []Rule{local}, "ask", "GET", "http://127.0.0.1:9/x", ""},
		{"ask unmatched host refused", []Rule{local}, "ask", "GET", "https://other.test/x", "a redirect must go to a host an allow rule matches"},
		{"ask unmatched method refused", []Rule{local}, "ask", "POST", "http://127.0.0.1:9/x", "a redirect must go to a host an allow rule matches"},
		{"ask deny match refused", []Rule{local, noDelete}, "ask", "DELETE", "http://127.0.0.1:9/x", "denied by the API access rule"},
		{"deny unmatched refused", []Rule{local}, "deny", "GET", "https://other.test/x", "a redirect must go to a host an allow rule matches"},
		{"deny allow match follows", []Rule{local}, "deny", "GET", "http://127.0.0.1:9/x", ""},
		{"hop error hides path and query", []Rule{local}, "ask", "GET", "https://other.test/secret?token=abc", "GET https://other.test matches no allow rule"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Compile(c.rules)
			if err != nil {
				t.Fatal(err)
			}
			u, _ := url.Parse(c.url)
			err = p.WithDefault(c.def).CheckHop(c.method, u)
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("want follow, got %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
			if err != nil && (strings.Contains(err.Error(), "token=abc") || strings.Contains(err.Error(), "/secret")) {
				t.Fatalf("error leaks path or query: %v", err)
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
