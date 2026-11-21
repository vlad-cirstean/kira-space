package agentapi

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apiaccess"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
)

type seed struct {
	name, value, origin string
	kind                keptvars.SourceKind
}

func inputSvc(t *testing.T, seeds ...seed) *Service {
	t.Helper()
	st := keptvars.New(keptvars.Options{})
	t.Cleanup(st.Close)
	for _, s := range seeds {
		_, err := st.Put(keptvars.Put{
			Scope: "external", Name: s.name, Value: json.RawMessage(s.value), Secret: s.kind == keptvars.SourceUser,
			Origin: s.origin, Source: keptvars.Source{Kind: s.kind},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return &Service{Kept: st}
}

func withInputs(in Input, decls ...InputDecl) Input {
	in.Inputs = decls
	return in
}

var tokenDecl = InputDecl{Name: "token", Label: "Access token"}

const originA, originB = "https://a.example", "https://b.example"

func TestInputDecisionTable(t *testing.T) {
	user := func(origin string) seed { return seed{"token", `"V"`, origin, keptvars.SourceUser} }
	script := seed{"token", `"V"`, "", keptvars.SourceScript}
	ref := func(host string) Input { return get("https://" + host + "/p?t={{kept.token}}") }
	unref := func(host string) Input { return get("https://" + host + "/p") }
	cases := []struct {
		name  string
		seeds []seed
		in    Input
		state InputState
		err   string
	}{
		{"none referenced", nil, ref("a.example"), InputNeeded, ""},
		{"none unreferenced", nil, unref("a.example"), "", "declared but not used"},
		{"user same origin referenced", []seed{user(originA)}, ref("a.example"), InputStored, ""},
		{"user same origin default port", []seed{user(originA)}, ref("A.example:443"), InputStored, ""},
		{"user same origin unreferenced", []seed{user(originA)}, unref("a.example"), "", "declared but not used"},
		{"user other origin referenced", []seed{user(originA)}, ref("b.example"), "", "supplied for https://a.example; choose another input name for https://b.example"},
		{"user other origin unreferenced", []seed{user(originA)}, unref("b.example"), "", "supplied for https://a.example"},
		{"script entry referenced", []seed{script}, ref("a.example"), "", "made by a script; choose another input name"},
		{"script entry unreferenced", []seed{script}, unref("a.example"), "", "made by a script"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := inputSvc(t, c.seeds...).Plan(withInputs(c.in, tokenDecl))
			if c.err != "" {
				if err == nil || !strings.Contains(err.Error(), c.err) {
					t.Fatalf("err = %v, want %q", err, c.err)
				}
				return
			}
			if err != nil || len(p.Inputs) != 1 || p.Inputs[0].State != c.state || p.Inputs[0].Label != "Access token" {
				t.Fatalf("inputs = %+v, err %v, want state %s", p.Inputs, err, c.state)
			}
			if (c.state == InputNeeded) != (p.MissingInputs() == 1) || (c.state == InputNeeded) != p.Kept[0].Pending {
				t.Fatalf("missing = %d, kept = %+v", p.MissingInputs(), p.Kept)
			}
		})
	}
}

func TestInputReadByScriptCountsAsUsed(t *testing.T) {
	in := withInputs(unrefGet(), tokenDecl)
	in.ScriptReads = []string{"token"}
	p, err := inputSvc(t).Plan(in)
	if err != nil || p.MissingInputs() != 1 {
		t.Fatalf("plan: %v, missing %d", err, p.MissingInputs())
	}
}

func unrefGet() Input { return get("https://a.example/p") }

func TestUndeclaredReferenceToUserEntry(t *testing.T) {
	svc := inputSvc(t, seed{"token", `"V"`, originA, keptvars.SourceUser})
	p, err := svc.Plan(get("https://a.example/p?t={{kept.token}}"))
	if err != nil || !p.UsesUserValue() || p.Kept[0].Pending {
		t.Fatalf("same origin: %v %+v", err, p.Kept)
	}
	if _, err := svc.Plan(get("https://b.example/p?t={{kept.token}}")); err == nil || !strings.Contains(err.Error(), "supplied for https://a.example") {
		t.Fatalf("other origin: %v", err)
	}
	if _, err := svc.Plan(get("https://a.example/p?t={{kept.nope}}")); err == nil || !strings.HasSuffix(err.Error(), "declare it in inputs") {
		t.Fatalf("absent: %v", err)
	}
}

func TestPendingInputPositions(t *testing.T) {
	svc := inputSvc(t)
	hdr := Input{Method: "GET", URL: "https://a.example/", Scope: "external", Headers: []NameValue{{Name: "Authorization", Value: "Bearer {{kept.token}}"}}}
	jsonBody := Input{Method: "POST", URL: "https://a.example/", Scope: "external", ContentType: "application/json", Body: `{"t": {{kept.token}}}`}
	quoted := Input{Method: "POST", URL: "https://a.example/", Scope: "external", ContentType: "application/json", Body: `{"t": "{{kept.token}}"}`}
	for name, c := range map[string]struct {
		in  Input
		err string
	}{
		"authority":  {get("https://{{kept.token}}.example/"), "scheme, host or port"},
		"header":     {hdr, ""},
		"json body":  {jsonBody, ""},
		"json quote": {quoted, "without quotes"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Plan(withInputs(c.in, tokenDecl))
			if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
				t.Fatalf("err = %v, want %q", err, c.err)
			}
		})
	}
}

func TestInputDeclarationRules(t *testing.T) {
	svc := inputSvc(t)
	in := get("https://a.example/{{kept.a}}{{kept.b}}{{kept.c}}{{kept.d}}{{kept.e}}")
	var five []InputDecl
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		five = append(five, InputDecl{Name: n, Label: n})
	}
	long := strings.Repeat("x", 81)
	cases := []struct {
		name  string
		decls []InputDecl
		err   string
	}{
		{"five", five, "at most 4 inputs"},
		{"duplicate", []InputDecl{{Name: "a", Label: "A"}, {Name: "a", Label: "B"}}, "declared twice"},
		{"bad name", []InputDecl{{Name: "1a", Label: "A"}}, "kept variable name"},
		{"empty label", []InputDecl{{Name: "a", Label: "  "}}, "label of 1 to 80 characters"},
		{"long label", []InputDecl{{Name: "a", Label: long}}, "label of 1 to 80 characters"},
		{"multiline label", []InputDecl{{Name: "a", Label: "x\ny"}}, "on one line"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := svc.Plan(withInputs(in, c.decls...)); err == nil || !strings.Contains(err.Error(), c.err) {
				t.Fatalf("err = %v, want %q", err, c.err)
			}
		})
	}
}

func TestInputRoomOverScopeCap(t *testing.T) {
	st := keptvars.New(keptvars.Options{})
	t.Cleanup(st.Close)
	for i := 0; i < keptvars.MaxScopeEntries; i++ {
		name := "v" + strings.Repeat("a", i%30) + string(rune('A'+i/30))
		if _, err := st.Put(keptvars.Put{Scope: "external", Name: name, Value: json.RawMessage(`1`), Source: keptvars.Source{Kind: keptvars.SourceScript}}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := (&Service{Kept: st}).Plan(withInputs(get("https://a.example/{{kept.token}}"), tokenDecl))
	if err != keptvars.ErrScopeFull {
		t.Fatalf("err = %v", err)
	}
}

func TestSendRefusesMissingInput(t *testing.T) {
	svc := inputSvc(t)
	if _, err := svc.Send(context.Background(), withInputs(get("https://a.example/{{kept.token}}"), tokenDecl), nil); err != errInputMissing {
		t.Fatalf("err = %v", err)
	}
}

func TestUserValueMaskedInEveryForm(t *testing.T) {
	svc := inputSvc(t, seed{"token", `"p@ss w0rd/1"`, originA, keptvars.SourceUser})
	m := newMask(&apifiles.Opened{}, nil, svc.keptSecrets(), nil)
	for _, echo := range []string{"p@ss w0rd/1", "p%40ss+w0rd%2F1", "p@ss%20w0rd%2F1", "cEBzcyB3MHJkLzE="} {
		if got := m.text("x " + echo + " y"); got != "x {{kept.token}} y" {
			t.Fatalf("echo %q -> %q", echo, got)
		}
	}
}

func TestRedirectGuardBindsUserValueToOrigin(t *testing.T) {
	origin, _ := url.Parse("https://a.example/start")
	hop := func(s string) *url.URL { u, _ := url.Parse(s); return u }
	bound := redirectGuard("GET", nil, apiaccess.Policy{}, origin, false, originA)
	if bound == nil {
		t.Fatal("a user value needs a guard")
	}
	for _, off := range []string{"https://b.example/x", "http://a.example/x", "https://a.example:8443/x"} {
		if err := bound(hop(off)); err == nil || !strings.Contains(err.Error(), "bound to "+originA) {
			t.Fatalf("hop %s: %v", off, err)
		}
	}
	if err := bound(hop("https://A.example:443/next")); err != nil {
		t.Fatalf("same origin hop: %v", err)
	}
	if redirectGuard("GET", nil, apiaccess.Policy{}, origin, false, "") != nil {
		t.Fatal("without a user value and credentials an allow-all policy needs no guard")
	}
}
