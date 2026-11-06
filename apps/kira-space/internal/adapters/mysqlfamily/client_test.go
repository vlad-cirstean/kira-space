// client_test.go is a pure unit test (no container) for BuildConfig's URI-mode query-string
// handling — a connection URI's query parameters
// used to be copied verbatim into mysql.Config.Params, which go-sql-driver executes as a literal
// unescaped `SET <k> = <v>` statement at connect time.
package mysqlfamily_test

import (
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/mysqlfamily"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func noopProfile() mysqlfamily.Profile {
	return mysqlfamily.Profile{Kind: "mysql", ServerLabel: "MySQL", ApplyEngineOptions: func(*mysql.Config, model.ResolvedConnectionConfig, mysqlfamily.LogFunc) {}}
}

func uriCfg(uri string) model.ResolvedConnectionConfig {
	return model.ResolvedConnectionConfig{ID: "conn-1", Mode: "uri", URI: &uri}
}

// TestBuildConfig_URIQueryParamsNeverReachDriverParams is the regression test: before the fix,
// every query parameter on a mysql:// URI (sslmode included) was copied into mc.Params, which the
// driver concatenates into a `SET ...` statement and executes verbatim at connect time.
func TestBuildConfig_URIQueryParamsNeverReachDriverParams(t *testing.T) {
	var warnings []string
	log := func(level, message string) {
		if level == "warn" {
			warnings = append(warnings, message)
		}
	}

	mc, err := mysqlfamily.BuildConfig(uriCfg("mysql://u:p@host:3306/db?sslmode=require&x=1&sql_mode=''"), "", noopProfile(), log)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}

	if len(mc.Params) != 0 {
		t.Fatalf("mc.Params must stay empty (the driver executes it as SQL at connect time), got %#v", mc.Params)
	}

	foundWarnOnX, foundWarnOnSQLMode := false, false
	for _, w := range warnings {
		if strings.Contains(w, `"x"`) {
			foundWarnOnX = true
		}
		if strings.Contains(w, "sql_mode") {
			foundWarnOnSQLMode = true
		}
	}
	if !foundWarnOnX || !foundWarnOnSQLMode {
		t.Fatalf("expected a warn log naming each dropped unrecognized option, got %v", warnings)
	}
}

// TestBuildConfig_URISslmodeStillAppliesTLS is the other half: sslmode must keep working as a
// real TLS setting once it stops being executed as SQL.
func TestBuildConfig_URISslmodeStillAppliesTLS(t *testing.T) {
	mc, err := mysqlfamily.BuildConfig(uriCfg("mysql://u:p@host:3306/db?sslmode=verify-full"), "", noopProfile(), func(string, string) {})
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if mc.TLSConfig == "" {
		t.Fatal("expected sslmode=verify-full (from the URI) to register a TLSConfig name")
	}
}

// TestBuildConfig_TLSConfigNameStableAcrossConnectionIDs is the regression test for the
// registry leak: BuildConfig used to register the driver's process-global TLS config under
// "kira-"+cfg.ID, so every distinct connection id minted its own entry — never deregistered on
// disconnect or delete — that accumulated for the life of the process. Two connections with
// different ids but the same effective TLS settings (same host, same sslmode) must now register
// under the *same* name, proving the registration is keyed on the settings rather than the id and
// is therefore idempotent instead of unbounded.
func TestBuildConfig_TLSConfigNameStableAcrossConnectionIDs(t *testing.T) {
	cfgA := model.ResolvedConnectionConfig{ID: "conn-aaaa", Mode: "uri", URI: ptr("mysql://u:p@same-host:3306/db?sslmode=verify-full")}
	cfgB := model.ResolvedConnectionConfig{ID: "conn-bbbb", Mode: "uri", URI: ptr("mysql://u:p@same-host:3306/db?sslmode=verify-full")}

	mcA, err := mysqlfamily.BuildConfig(cfgA, "", noopProfile(), func(string, string) {})
	if err != nil {
		t.Fatalf("BuildConfig(cfgA): %v", err)
	}
	mcB, err := mysqlfamily.BuildConfig(cfgB, "", noopProfile(), func(string, string) {})
	if err != nil {
		t.Fatalf("BuildConfig(cfgB): %v", err)
	}

	if mcA.TLSConfig != mcB.TLSConfig {
		t.Fatalf("two connections to the same host under sslmode=verify-full registered different TLS config names (%q vs %q) — the registry leaks one entry per connection id", mcA.TLSConfig, mcB.TLSConfig)
	}
	if strings.Contains(mcA.TLSConfig, "conn-aaaa") || strings.Contains(mcA.TLSConfig, "conn-bbbb") {
		t.Fatalf("TLS config name %q still derives from the connection id", mcA.TLSConfig)
	}

	// Same check for require/prefer's fixed, shared name.
	reqA := model.ResolvedConnectionConfig{ID: "conn-cccc", Mode: "uri", URI: ptr("mysql://u:p@host-a:3306/db?sslmode=require")}
	reqB := model.ResolvedConnectionConfig{ID: "conn-dddd", Mode: "uri", URI: ptr("mysql://u:p@host-b:3306/db?sslmode=require")}
	mcReqA, err := mysqlfamily.BuildConfig(reqA, "", noopProfile(), func(string, string) {})
	if err != nil {
		t.Fatalf("BuildConfig(reqA): %v", err)
	}
	mcReqB, err := mysqlfamily.BuildConfig(reqB, "", noopProfile(), func(string, string) {})
	if err != nil {
		t.Fatalf("BuildConfig(reqB): %v", err)
	}
	if mcReqA.TLSConfig != mcReqB.TLSConfig {
		t.Fatalf("two require-mode connections (different ids and different hosts, same settings shape) registered different TLS config names (%q vs %q)", mcReqA.TLSConfig, mcReqB.TLSConfig)
	}
}

func ptr(s string) *string { return &s }

// TestBuildConfig_URIUnknownSslmodeStillRejected preserves the existing "fail loudly on a typo"
// behaviour for the one option this adapter does understand.
func TestBuildConfig_URIUnknownSslmodeStillRejected(t *testing.T) {
	_, err := mysqlfamily.BuildConfig(uriCfg("mysql://u:p@host:3306/db?sslmode=bogus"), "", noopProfile(), func(string, string) {})
	if err == nil {
		t.Fatal("expected an error for an unrecognized sslmode")
	}
}
