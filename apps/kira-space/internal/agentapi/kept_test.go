package agentapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
)

func keptSvc(t *testing.T, values map[string]string) *Service {
	t.Helper()
	st := keptvars.New(keptvars.Options{})
	t.Cleanup(st.Close)
	for name, v := range values {
		if _, err := st.Put(keptvars.Put{Scope: "external", Name: name, Value: json.RawMessage(v), Source: keptvars.Source{Kind: keptvars.SourceScript}}); err != nil {
			t.Fatal(err)
		}
	}
	return &Service{Kept: st}
}

func get(url string) Input { return Input{Method: "GET", URL: url, Scope: "external"} }

func build(t *testing.T, s *Service, in Input) (httpclient.Request, error) {
	t.Helper()
	p, err := s.plan(in, true)
	if err != nil {
		return httpclient.Request{}, err
	}
	req, _, err := buildRequest(p, &apifiles.Opened{})
	return req, err
}

func TestKeptInsertionByPositionAndType(t *testing.T) {
	vals := map[string]string{
		"s": `"a b/c&d"`, "n": `12`, "big": `12345678901234567890`, "t": `true`, "nl": `null`,
		"arr": `[1,"x"]`, "obj": `{"k":[2]}`,
	}
	svc := keptSvc(t, vals)
	cases := []struct {
		name string
		in   Input
		url  string
		body string
	}{
		{"path string escaped", get("http://h/p/{{kept.s}}"), "http://h/p/a%20b%2Fc&d", ""},
		{"query string escaped", get("http://h/p?q={{kept.s}}"), "http://h/p?q=a+b%2Fc%26d", ""},
		{"path number", get("http://h/p/{{kept.n}}"), "http://h/p/12", ""},
		{"path big integer exact", get("http://h/p/{{kept.big}}"), "http://h/p/12345678901234567890", ""},
		{"path boolean", get("http://h/p/{{kept.t}}"), "http://h/p/true", ""},
		{"path null empty", get("http://h/p/{{kept.nl}}x"), "http://h/p/x", ""},
		{"agent query entry", Input{Method: "GET", URL: "http://h/p", Scope: "external", Query: []NameValue{{Name: "q", Value: "{{kept.s}}"}}}, "http://h/p?q=a+b%2Fc%26d", ""},
		{"json body array", Input{Method: "POST", URL: "http://h/p", Scope: "external", ContentType: "application/json", Body: `{"ids": {{kept.arr}}, "n": {{kept.n}}, "o": {{kept.obj}}, "s": {{kept.s}}, "z": {{kept.nl}}}`}, "http://h/p", `{"ids": [1,"x"], "n": 12, "o": {"k":[2]}, "s": "a b/c&d", "z": null}`},
		{"json body big integer", Input{Method: "POST", URL: "http://h/p", Scope: "external", ContentType: "application/vnd.api+JSON", Body: `[{{kept.big}}]`}, "http://h/p", `[12345678901234567890]`},
		{"raw body string raw", Input{Method: "POST", URL: "http://h/p", Scope: "external", ContentType: "text/plain", Body: `v={{kept.s}}`}, "http://h/p", `v=a b/c&d`},
		{"raw body list as json", Input{Method: "POST", URL: "http://h/p", Scope: "external", ContentType: "text/plain", Body: `v={{kept.arr}} {{kept.n}} {{kept.t}}`}, "http://h/p", `v=[1,"x"] 12 true`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, err := build(t, svc, c.in)
			if err != nil {
				t.Fatal(err)
			}
			if req.URL != c.url || req.Body.Raw != c.body {
				t.Fatalf("got url %q body %q, want url %q body %q", req.URL, req.Body.Raw, c.url, c.body)
			}
		})
	}
}

func TestKeptFormAndHeader(t *testing.T) {
	svc := keptSvc(t, map[string]string{"n": `7`, "s": `"v"`})
	in := Input{Method: "POST", URL: "http://h/p", Scope: "external", Headers: []NameValue{{Name: "X-A", Value: "id={{kept.n}}"}}, Form: []NameValue{{Name: "k{{kept.s}}", Value: "{{kept.n}}"}}}
	req, err := build(t, svc, in)
	if err != nil {
		t.Fatal(err)
	}
	if req.Headers[0].Value != "id=7" || req.Body.URLEncoded[0].Name != "kv" || req.Body.URLEncoded[0].Value != "7" {
		t.Fatalf("got %+v", req)
	}
}

func TestKeptRefusals(t *testing.T) {
	vals := map[string]string{"h": `"evil.test"`, "p": `8080`, "arr": `[1]`, "nl": `"a\nb"`, "x": `"q"`}
	for i := 0; i < 17; i++ {
		vals["v"+string(rune('a'+i))] = `1`
	}
	svc := keptSvc(t, vals)
	many := ""
	for i := 0; i < 17; i++ {
		many += "{{kept.v" + string(rune('a'+i)) + "}}"
	}
	cases := []struct {
		name string
		in   Input
		want string
	}{
		{"unknown", get("http://h/{{kept.nope}}"), `no kept variable "nope"`},
		{"other scope", Input{Method: "GET", URL: "http://h/{{kept.x}}", Scope: "session:1"}, `no kept variable "x"`},
		{"host", get("https://{{kept.h}}/"), "scheme, host or port"},
		{"port", get("https://a.com:{{kept.p}}/"), "scheme, host or port"},
		{"scheme", get("{{kept.h}}://a.com/"), "scheme, host or port"},
		{"userinfo", get("https://u:{{kept.x}}@a.com/"), "scheme, host or port"},
		{"pipeline", get("http://h/{{kept.x | base64}}"), "no transforms"},
		{"list in url", get("http://h/{{kept.arr}}"), "is a list"},
		{"list in header", Input{Method: "GET", URL: "http://h/", Scope: "external", Headers: []NameValue{{Name: "X", Value: "{{kept.arr}}"}}}, "is a list"},
		{"line break in header", Input{Method: "GET", URL: "http://h/", Scope: "external", Headers: []NameValue{{Name: "X", Value: "{{kept.nl}}"}}}, "line break"},
		{"quoted in json", Input{Method: "POST", URL: "http://h/", Scope: "external", ContentType: "application/json", Body: `{"a": "{{kept.x}}"}`}, "without quotes"},
		{"quoted after escaped quote", Input{Method: "POST", URL: "http://h/", Scope: "external", ContentType: "application/json", Body: `{"a": "a\"{{kept.x}}"}`}, "without quotes"},
		{"invalid json template", Input{Method: "POST", URL: "http://h/", Scope: "external", ContentType: "application/json", Body: `{"a": {{kept.x}} {{kept.x}}}`}, "not valid JSON"},
		{"seventeen names", get("http://h/?" + many), "at most 16"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.Plan(c.in)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
		})
	}
}

func TestKeptValueIsNeverRescanned(t *testing.T) {
	svc := keptSvc(t, map[string]string{"x": `"{{token}}"`})
	p, err := svc.plan(Input{Method: "POST", URL: "http://h/p", Scope: "external", Body: `{{kept.x}}`}, true)
	if err != nil {
		t.Fatal(err)
	}
	// A token that rescanned the kept value would try to open this unset secret and fail.
	p.creds = []credential{{name: "token", secretName: "dev.token"}}
	req, _, err := buildRequest(p, &apifiles.Opened{})
	if err != nil || req.Body.Raw != "{{token}}" {
		t.Fatalf("body %q, err %v, want the span sent literally", req.Body.Raw, err)
	}
}

func TestKeptSecretValueIsMasked(t *testing.T) {
	svc := keptSvc(t, map[string]string{"s": `"hunter2"`})
	p, err := svc.plan(Input{Method: "GET", URL: "http://h/{{kept.s}}", Scope: "external"}, true)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range p.keptValues {
		v.secret = true
		p.keptValues[k] = v
	}
	m := newMask(&apifiles.Opened{}, nil, p.secretKeptTexts())
	if got := m.text(`{"echo":"hunter2"}`); strings.Contains(got, "hunter2") {
		t.Fatalf("masked = %s", got)
	}
}

func TestKeptPlanCarriesNoValueAndSitesFollowText(t *testing.T) {
	svc := keptSvc(t, map[string]string{"ids": `[101,102]`, "region": `"eu"`})
	p, err := svc.Plan(Input{Method: "POST", URL: "http://h/é/{{kept.region}}", Scope: "external", ContentType: "application/json", Body: `{"ids": {{kept.ids}}}`})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Kept) != 2 || p.keptValues != nil {
		t.Fatalf("kept = %+v, values held = %v", p.Kept, p.keptValues != nil)
	}
	site := p.Kept[0].Sites[0]
	if p.Kept[0].Name != "region" || site.Field != "url" || string([]rune(p.URL)[site.From:site.To]) != "{{kept.region}}" {
		t.Fatalf("site = %+v in %q", site, p.URL)
	}
	if b := p.Kept[1].Sites[0]; b.Field != "body" || p.Body[b.From:b.To] != "{{kept.ids}}" {
		t.Fatalf("body site = %+v", b)
	}
}

func TestSendRefusesAChangedKeptValueButNotSameRequest(t *testing.T) {
	svc := keptSvc(t, map[string]string{"x": `1`})
	in := get("http://h/{{kept.x}}")
	approved, err := svc.Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	same, _ := svc.Plan(in)
	if !same.sameRequest(approved) || same.checkKeptCurrent(approved) != nil {
		t.Fatal("an unchanged plan must pass")
	}
	st := svc.Kept.(*keptvars.Store)
	if _, err := st.Put(keptvars.Put{Scope: "external", Name: "x", Value: json.RawMessage(`2`), Source: keptvars.Source{Kind: keptvars.SourceScript}}); err != nil {
		t.Fatal(err)
	}
	moved, _ := svc.Plan(in)
	if !moved.sameRequest(approved) {
		t.Fatal("sameRequest compares the template, not the value")
	}
	if err := moved.checkKeptCurrent(approved); err == nil || !strings.Contains(err.Error(), "changed while waiting") {
		t.Fatalf("err = %v, want a changed refusal", err)
	}
}
