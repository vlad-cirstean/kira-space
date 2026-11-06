// matrix.go is the harness for the complete permutation suite — auth/config cases,
// plus the Scenario/Then seam populated with real functional coverage per adapter.
// Case/Outcome/RunMatrix live here, once, so a per-adapter authmatrix_test.go only ever declares
// its own table.
package testsupport

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

const matrixEnv = "KIRA_TEST_MATRIX"

// FixturePassword is the shared default password every container fixture's baked-in principal
// uses (postgres.go, mariadb.go, mysql.go, mongo.go, clickhouse.go, redis.go, kafka_sasl.go) —
// exported so a per-adapter authmatrix_test.go that needs the real credential (rather than an
// intentionally-wrong one) references this instead of re-typing the literal, the exact drift this
// constant exists to prevent. Deliberately not "kira": that used to be the shared default, and
// "kira" is a substring of several fixture identifiers a driver can legitimately echo in error
// text — the database names kira_test/kira_analytics/kira_admin, kafka's own "kira-studio" client
// id and "kira-sasl-test-cluster" cluster id — so a passing case's incidental error text could
// satisfy this file's own password-leak assertion below by accident, on a message that never
// echoed the password. No case triggers that today (every failing case already overrides
// the password to something else distinctive), but the hazard is real for future additions.
// A fixture's own password must never collide as a substring with any other fixture identifier.
const FixturePassword = "kira-fixture-pw"

// RequireMatrix skips unless the complete suite was explicitly asked for. Call it before starting
// any fixture (StartX), not just before RunMatrix — an ordinary `go test ./...` must never pay for
// a container this tier alone needs.
func RequireMatrix(t *testing.T) {
	t.Helper()
	if os.Getenv(matrixEnv) != "1" {
		t.Skip("set " + matrixEnv + "=1 to run the permutation matrix (scripts/test-matrix.sh)")
	}
}

// Principal is one server-side identity a case needs, created inside the adapter's already-running
// shared container (one container per adapter, roles created at runtime inside it) and
// torn down with the test that asked for it. f is the adapter's own fixture pointer (e.g.
// *RedisFixture) — untyped because each adapter's Setup closure is written against its own
// concrete fixture type; RunMatrix never inspects it, only passes it through.
type Principal struct {
	Name  string
	Setup func(t *testing.T, f any)
}

// ClearAwsEnv unsets AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/AWS_SESSION_TOKEN/AWS_PROFILE for one
// subtest, auto-restored (s3's and sqs's own authmatrix_test.go, byte-identical).
// This sandbox's own outbound proxy injects placeholder AWS_* credentials (CLAUDE.md), which would
// otherwise mask the "no credentials anywhere" case the transcript needs.
var ClearAwsEnv = &Principal{
	Name: "clear ambient AWS env",
	Setup: func(t *testing.T, _ any) {
		t.Helper()
		for _, k := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE"} {
			old, had := os.LookupEnv(k)
			_ = os.Unsetenv(k)
			t.Cleanup(func() {
				if had {
					_ = os.Setenv(k, old)
				}
			})
		}
	},
}

// SeedAmbientAwsEnv is ClearAwsEnv's mirror (same byte-identical pair): guarantees
// AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY are ambiently present for one subtest (t.Setenv,
// auto-restored). Some fields-mode cases need ambient credentials to exist somewhere in the
// environment for LoadDefaultConfig to find (a lone URI key never becomes a static credential,
// config.go's own both-or-nothing) — this sandbox's own outbound proxy happens to inject
// placeholder AWS_* env, which is what let those cases pass in-sandbox, but a GitHub Actions
// runner has no such proxy and fails with a real EC2-IMDS lookup error instead. Setting the env
// directly makes the case pass on its own merits everywhere (LocalStack accepts any nonempty
// static credentials unconditionally, measured, so the exact values don't matter).
var SeedAmbientAwsEnv = &Principal{
	Name: "seed ambient AWS env",
	Setup: func(t *testing.T, _ any) {
		t.Helper()
		t.Setenv("AWS_ACCESS_KEY_ID", LocalStackStaticAccessKey)
		t.Setenv("AWS_SECRET_ACCESS_KEY", LocalStackStaticSecret)
	},
}

// Outcome is what Connect must produce. Exactly one of Succeed and a Code assertion is meaningful:
// when Succeed, FailWith/NotCode are ignored; when not, FailWith asserts the exact code and NotCode
// asserts the failure is anything *but* that code (the "a permission refusal must not read as
// E_AUTH" shape, which recurs in nearly every adapter's own matrix).
type Outcome struct {
	Succeed  bool
	FailWith adapters.ErrorCode
	NotCode  adapters.ErrorCode
	Details  map[string]string // asserted as a subset of ConnectInfo.Details
}

// Case is one connection configuration plus the connect outcome it must produce, plus whatever
// Scenarios a later phase attaches once a connection of that shape comes up.
type Case struct {
	Name      string
	Principal *Principal // nil = the fixture's own base config needs no extra principal
	Config    func(base model.ResolvedConnectionConfig) model.ResolvedConnectionConfig
	Expect    Outcome
	Then      []Scenario // empty in P25; P26 populated it per adapter for a Succeed case
}

// Scenario is one thing to do with a connection that came up — the extension point P26 populated
// per adapter with real functional coverage (load/write/delete/filter/DDL) atop this same harness.
type Scenario struct {
	Name     string
	Requires func(adapters.Caps) bool // skip where the adapter does not claim the capability
	Run      func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig)
}

// runMatrixCase runs one Case: Principal.Setup, build the config, Connect, assert Outcome, run
// each Scenario whose Requires passes, Disconnect. Called once per case inside RunMatrix's own
// t.Run. Deliberately does NOT call t.Helper(): verified empirically that marking
// this function a helper collapses every one of its own assertions' reported line to the single
// generic "runMatrixCase(...)" call site inside RunMatrix's loop, losing exactly which check
// failed; leaving it unmarked keeps each t.Errorf/t.Fatalf attributed to its own line here.
func runMatrixCase(t *testing.T, kind string, fixture any, deps adapters.Deps, base model.ResolvedConnectionConfig, c Case) {
	if c.Principal != nil {
		c.Principal.Setup(t, fixture)
	}
	cfg := base
	if c.Config != nil {
		cfg = c.Config(base)
	}

	a, err := adapters.CreateAdapter(kind, deps)
	if err != nil {
		t.Fatalf("CreateAdapter(%s): %v", kind, err)
	}

	info, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("matrix-"+kind))

	if c.Expect.Succeed {
		if err != nil {
			t.Fatalf("Connect: want success, got %v", err)
		}
		t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
		for k, want := range c.Expect.Details {
			if got := info.Details[k]; got != want {
				t.Errorf("Details[%s] = %q, want %q", k, got, want)
			}
		}
		for _, s := range c.Then {
			if s.Requires != nil && !s.Requires(a.Caps()) {
				continue
			}
			t.Run(s.Name, func(t *testing.T) { s.Run(t, a, cfg) })
		}
		return
	}

	if err == nil {
		t.Fatal("Connect: want an error, got nil")
	}
	// P29 F6: the op log persists this exact error text to disk for
	// advanced.opLogRetentionDays — verify a failed connect never echoes the password
	// back, in either config shape. The failure message deliberately does not print the
	// error itself, so a real leak doesn't also land in CI logs.
	if p := cfg.Password; p != nil && *p != "" && strings.Contains(err.Error(), *p) {
		t.Error("Connect error text contains the connection password verbatim")
	}
	for _, secret := range secretsFromURI(cfg.URI) {
		if strings.Contains(err.Error(), secret) {
			t.Error("Connect error text contains a URI-embedded secret verbatim")
		}
	}
	code, _ := adapters.CodeOf(err)
	if c.Expect.FailWith != "" && code != c.Expect.FailWith {
		t.Errorf("code = %v, want %v (err: %v)", code, c.Expect.FailWith, err)
	}
	if c.Expect.NotCode != "" && code == c.Expect.NotCode {
		t.Errorf("code = %v, must not be %v (err: %v)", code, c.Expect.NotCode, err)
	}
}

// RunMatrix drives every case as a subtest: RequireMatrix, then runMatrixCase per case. kind is
// the adapter's own registry name ("redis", "mongodb", ...); fixture is passed straight through to
// each case's own Principal.Setup, untouched.
func RunMatrix(t *testing.T, kind string, fixture any, base model.ResolvedConnectionConfig, cases []Case) {
	t.Helper()
	RequireMatrix(t)
	deps := adapters.Deps{Log: func(level, message string) {}}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) { runMatrixCase(t, kind, fixture, deps, base, c) })
	}
}

// uriSecretQueryKeys mirrors connfiles secretOptionKeys (a local copy: importing that
// package here would invert the dependency).
var uriSecretQueryKeys = map[string]bool{
	"password": true, "sslpassword": true, "tlscertificatekeyfilepassword": true, "proxypassword": true,
}

// secretsFromURI extracts a uri-mode case's embedded secrets: the userinfo password plus the
// decoded value of each secret query pair (P181). A uri-mode Case carries them inside cfg.URI
// rather than cfg.Password, and the leak assertion needs every one.
func secretsFromURI(uri *string) []string {
	if uri == nil {
		return nil
	}
	idx := strings.Index(*uri, "://")
	if idx < 0 {
		return nil
	}
	rest := (*uri)[idx+3:]
	authority, query := rest, ""
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		authority = rest[:i]
		if q := strings.IndexByte(rest[i:], '?'); q >= 0 {
			query = rest[i+q+1:]
			if h := strings.IndexByte(query, '#'); h >= 0 {
				query = query[:h]
			}
		}
	}

	var out []string
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		if colon := strings.IndexByte(authority[:at], ':'); colon >= 0 && colon+1 < at {
			out = append(out, authority[colon+1:at])
		}
	}
	for _, pair := range strings.Split(query, "&") {
		key, raw, _ := strings.Cut(pair, "=")
		if !uriSecretQueryKeys[strings.ToLower(key)] || raw == "" {
			continue
		}
		value, err := url.QueryUnescape(raw)
		if err != nil {
			value = raw
		}
		out = append(out, value)
	}
	return out
}

// RunScenarios applies the same Requires gate RunMatrix does, outside a matrix table, so a
// scenario written once against a live (already-connected) adapter backs both the general
// (Tier-1, ungated) suite and the complete (Tier-2, KIRA_TEST_MATRIX-gated) one instead of being
// written twice. Unlike RunMatrix, this does not call RequireMatrix itself — a Tier-1 caller must
// run unconditionally, and a Tier-2 caller already ran it via RunMatrix/RequireMatrix before a
// is connected.
func RunScenarios(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig, scenarios ...Scenario) {
	t.Helper()
	for _, s := range scenarios {
		if s.Requires != nil && !s.Requires(a.Caps()) {
			continue
		}
		t.Run(s.Name, func(t *testing.T) { s.Run(t, a, cfg) })
	}
}
