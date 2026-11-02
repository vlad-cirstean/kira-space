// P5 D18: the shared corpus, Go side. packages/api-core/src/http/substitute.ts's own resolve() runs the
// identical cases (tests/unit/http-substitution.spec.ts) — a case added to testdata/
// substitution.json must pass on both sides or one of them fails.
package apivars_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

type corpusCase struct {
	Name     string            `json:"name"`
	Template string            `json:"template"`
	Values   map[string]string `json:"values"`
	Secrets  []string          `json:"secrets"`
	Want     string            `json:"want"`
	Refs     []struct {
		Name     string   `json:"name"`
		Kind     string   `json:"kind"`
		Pipeline []string `json:"pipeline"`
	} `json:"refs"`
}

func loadCorpus(t *testing.T) []corpusCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "substitution.json"))
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var cases []corpusCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("decode corpus: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("corpus is empty")
	}
	return cases
}

func TestResolveAgainstTheSharedCorpus(t *testing.T) {
	for _, c := range loadCorpus(t) {
		t.Run(c.Name, func(t *testing.T) {
			result := apivars.Resolve(c.Template, c.Values, c.Secrets)
			if result.Text != c.Want {
				t.Errorf("Text = %q, want %q", result.Text, c.Want)
			}
			if len(result.Refs) != len(c.Refs) {
				t.Fatalf("Refs = %+v, want %+v", result.Refs, c.Refs)
			}
			for i, ref := range result.Refs {
				want := c.Refs[i]
				if ref.Name != want.Name || string(ref.Kind) != want.Kind {
					t.Errorf("Refs[%d] = {%s %s}, want {%s %s}", i, ref.Name, ref.Kind, want.Name, want.Kind)
				}
				// P17 D4: Pipeline is nil (omitted from the JSON) for every case that predates the
				// pipe grammar — normalize both sides to a same-length comparison so a corpus entry
				// with no "pipeline" key (unmarshalled as a nil slice) compares equal to Go's own nil.
				if len(ref.Pipeline) != len(want.Pipeline) {
					t.Errorf("Refs[%d].Pipeline = %v, want %v", i, ref.Pipeline, want.Pipeline)
					continue
				}
				for j, step := range ref.Pipeline {
					if step != want.Pipeline[j] {
						t.Errorf("Refs[%d].Pipeline[%d] = %q, want %q", i, j, step, want.Pipeline[j])
					}
				}
			}
		})
	}
}

// strPtr is repos_test's own helper (storage/repos/ops_test.go), restated here — F2's three-state
// VariablesRepo.Upsert (P108 Part 3) needs it for every secret seeded below.
func strPtr(s string) *string { return &s }

// newResolveService opens a real (tmpfile-backed) SQLite database through the real migrations —
// mirrors repos_test's own newVariablesRepo (variables_test.go); duplicated rather than imported
// since that helper lives in an internal _test.go file of a different package. Auth is nil: none of
// ResolveRequest's own path ever calls it (only Reveal/RevealHistory do). The returned
// *repos.VariablesRepo is the exact instance the Service resolves secrets through — seeding via it
// directly (rather than a second, independent instance) is what makes SecretsFor see the row.
func newResolveService(t *testing.T) (*apivars.Service, *repos.VariablesRepo, *repos.CollectionsRepo) {
	t.Helper()
	t.Setenv("KIRA_INSECURE_SECRETS", "1")
	t.Setenv("KIRA_HOME", t.TempDir())
	db, err := storage.Open()
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	variablesRepo := repos.NewVariables(db.DB, secrets.New())
	return apivars.New(variablesRepo, nil), variablesRepo, &repos.CollectionsRepo{DB: db.DB}
}

// findUsed is resolve_test.go's own small lookup over ResolveRequest/Resolver.Used()'s
// []UsedSecret (P17 D9) — a test still wants "is this name in there, and what did it render to",
// without caring about entry order.
func findUsed(used []apivars.UsedSecret, name string) (apivars.UsedSecret, bool) {
	for _, u := range used {
		if u.Name == name {
			return u, true
		}
	}
	return apivars.UsedSecret{}, false
}

// P9 §6.2, widened by P17 D9: ResolveRequest's fourth return is exactly the secret spans it
// actually substituted — the (Rendered, Placeholder) pairs P9 D6's masking replacer needs — and
// empty when the request references none.
func TestResolveRequestReturnsExactlyTheSecretsItSubstituted(t *testing.T) {
	svc, variablesRepo, collections := newResolveService(t)
	c, err := collections.CreateCollection("Orders")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	t.Run("substitutes and reports exactly what it used", func(t *testing.T) {
		if _, err := variablesRepo.Upsert(model.VariableScopeCollection, c.ID, "", "token", strPtr("sekret-value"), true, ""); err != nil {
			t.Fatalf("Upsert: %v", err)
		}

		url, headers, body, used, err := svc.ResolveRequest(
			"https://api.example.com/orders",
			[]httpclient.Header{{Name: "Authorization", Value: "Bearer {{token}}"}},
			httpclient.Body{Mode: "none"},
			c.ID, "",
		)
		if err != nil {
			t.Fatalf("ResolveRequest: %v", err)
		}
		if url != "https://api.example.com/orders" {
			t.Errorf("url = %q, unexpectedly changed", url)
		}
		if len(headers) != 1 || headers[0].Value != "Bearer sekret-value" {
			t.Fatalf("headers = %+v, want Authorization: Bearer sekret-value", headers)
		}
		if body.Mode != "none" {
			t.Errorf("body.Mode = %q, want none", body.Mode)
		}
		got, ok := findUsed(used, "token")
		if len(used) != 1 || !ok || got.Rendered != "sekret-value" || got.Placeholder != "{{token}}" {
			t.Fatalf("used = %+v, want one entry {token, sekret-value, {{token}}}", used)
		}
	})

	t.Run("empty when the request references no secret", func(t *testing.T) {
		_, _, _, used, err := svc.ResolveRequest(
			"https://api.example.com/orders?x={{plain}}",
			nil, httpclient.Body{Mode: "none"}, c.ID, "",
		)
		if err != nil {
			t.Fatalf("ResolveRequest: %v", err)
		}
		if len(used) != 0 {
			t.Fatalf("used = %+v, want empty", used)
		}
	})

	// A reference that never resolves — a stale id, a typo, or a secret whose decrypt failed — is
	// left verbatim in the URL by design. Resolver.Text/URLText always call Resolve with
	// secretNames nil, so a name that turns out not to be any of this scope's secrets falls into
	// the exact same branch a decrypt failure would — reproduced here with a plain unresolvable
	// name, since the fixture already has a space in it (a real Postman variable-name shape,
	// substitute.ts's own doc). The raw space used to reach net/http's own request line unescaped,
	// which the *server* rejects with a 400 before ever reading a single header — driven through a
	// real httpclient.Send against a real httptest server, not just url.Parse (which tolerates
	// the raw space and never errors, so it alone would not have caught this).
	t.Run("finding 6: an unresolved reference with a space in its name doesn't break the request line", func(t *testing.T) {
		var gotRequestURI string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotRequestURI = r.RequestURI
			w.WriteHeader(http.StatusTeapot)
		}))
		defer srv.Close()

		resolvedURL, _, _, _, err := svc.ResolveRequest(
			srv.URL+"/orders?a={{base url}}&b=2",
			nil, httpclient.Body{Mode: "none"}, c.ID, "",
		)
		if err != nil {
			t.Fatalf("ResolveRequest: %v", err)
		}
		if strings.Contains(resolvedURL, "{{base url}}") {
			t.Fatalf("resolvedURL = %q still has a raw space inside the unresolved reference", resolvedURL)
		}
		if !strings.Contains(resolvedURL, "{{base%20url}}") {
			t.Fatalf("resolvedURL = %q, want the reference recognisable as {{base%%20url}}", resolvedURL)
		}

		resp, err := httpclient.Send(context.Background(), httpclient.Request{Method: "GET", URL: resolvedURL}, httpclient.Options{})
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		// The server received and parsed a well-formed request line — "the reference
		// itself isn't found by the server" (it's an unresolved {{...}} token, not this app's
		// concern) shows up as *this handler's own* status, never a client-side 400 from a
		// malformed request line the server bounced before routing it anywhere.
		if resp.Status != http.StatusTeapot {
			t.Errorf("resp.Status = %d, want %d (the handler's own answer) — the request line reached the server intact", resp.Status, http.StatusTeapot)
		}
		if !strings.Contains(gotRequestURI, "b=2") {
			t.Errorf("server saw RequestURI = %q, lost the second, unrelated query param", gotRequestURI)
		}
	})

	// A piped secret's UsedSecret.Rendered is the *transformed* form, not the
	// plaintext — the property bridge/http.go's secretReplacer is built on. This is the Go-level
	// proof that the resolver itself hands out the right text; bridge/http_test.go proves the
	// masking replacer built from it.
	t.Run("a piped secret's Rendered is the transformed form, not the plaintext", func(t *testing.T) {
		if _, err := variablesRepo.Upsert(model.VariableScopeCollection, c.ID, "", "piped", strPtr("hunter2"), true, ""); err != nil {
			t.Fatalf("Upsert: %v", err)
		}

		_, headers, _, used, err := svc.ResolveRequest(
			"https://api.example.com/orders",
			[]httpclient.Header{{Name: "X-Signed", Value: "{{piped | base64}}"}},
			httpclient.Body{Mode: "none"}, c.ID, "",
		)
		if err != nil {
			t.Fatalf("ResolveRequest: %v", err)
		}
		wantB64 := base64.StdEncoding.EncodeToString([]byte("hunter2"))
		if len(headers) != 1 || headers[0].Value != wantB64 {
			t.Fatalf("headers = %+v, want X-Signed: %s", headers, wantB64)
		}
		got, ok := findUsed(used, "piped")
		if !ok || got.Rendered != wantB64 || got.Placeholder != "{{piped | base64}}" {
			t.Fatalf("used = %+v, want one entry {piped, %s, {{piped | base64}}}", used, wantB64)
		}
		if strings.Contains(got.Rendered, "hunter2") {
			t.Fatalf("Rendered = %q must never contain the plaintext", got.Rendered)
		}
	})

	// D9(d): a secret used both plainly and piped in one request produces two entries, one per
	// distinct (Name, Placeholder) — the case a name-keyed model could not represent at all.
	t.Run("a secret used both plainly and piped records two distinct entries", func(t *testing.T) {
		if _, err := variablesRepo.Upsert(model.VariableScopeCollection, c.ID, "", "dual", strPtr("s3cr3t"), true, ""); err != nil {
			t.Fatalf("Upsert: %v", err)
		}

		_, headers, _, used, err := svc.ResolveRequest(
			"https://api.example.com/orders",
			[]httpclient.Header{
				{Name: "X-Plain", Value: "{{dual}}"},
				{Name: "X-Signed", Value: "{{dual | base64}}"},
			},
			httpclient.Body{Mode: "none"}, c.ID, "",
		)
		if err != nil {
			t.Fatalf("ResolveRequest: %v", err)
		}
		wantB64 := base64.StdEncoding.EncodeToString([]byte("s3cr3t"))
		if len(headers) != 2 || headers[0].Value != "s3cr3t" || headers[1].Value != wantB64 {
			t.Fatalf("headers = %+v, want plain s3cr3t and base64 %s", headers, wantB64)
		}
		var dualEntries []apivars.UsedSecret
		for _, u := range used {
			if u.Name == "dual" {
				dualEntries = append(dualEntries, u)
			}
		}
		if len(dualEntries) != 2 {
			t.Fatalf("dualEntries = %+v, want exactly 2 (one per distinct placeholder)", dualEntries)
		}
	})

	// P17 D11: `|` joins the URL-unsafe set — an unresolved piped reference in the URL must not
	// put a raw `|` into the request line, exactly as finding 6's own space did.
	t.Run("D11: an unresolved piped reference in the URL is percent-encoded, pipe included", func(t *testing.T) {
		resolvedURL, _, _, _, err := svc.ResolveRequest(
			"https://api.example.com/orders?a={{missing | upper}}&b=2",
			nil, httpclient.Body{Mode: "none"}, c.ID, "",
		)
		if err != nil {
			t.Fatalf("ResolveRequest: %v", err)
		}
		if strings.Contains(resolvedURL, "{{missing | upper}}") {
			t.Fatalf("resolvedURL = %q still has a raw pipe/space inside the unresolved reference", resolvedURL)
		}
		if !strings.Contains(resolvedURL, "{{missing%20%7C%20upper}}") {
			t.Fatalf("resolvedURL = %q, want the reference recognisable as {{missing%%20%%7C%%20upper}}", resolvedURL)
		}
		if _, err := url.Parse(resolvedURL); err != nil {
			t.Fatalf("url.Parse(%q): %v", resolvedURL, err)
		}
	})
}

func TestNamesFindsEveryDistinctReferenceOnce(t *testing.T) {
	got := apivars.Names("{{a}}/{{b}}?x={{a}}&y={{ c }}")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("Names = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("Names[%d] = %q, want %q", i, got[i], name)
		}
	}
}
