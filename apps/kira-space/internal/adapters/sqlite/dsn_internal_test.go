// dsn_internal_test.go: buildDSN builds the
// SQLite URI filename by plain string concatenation ("file:" + path + "?" + query). SQLite's own
// URI parser splits on the *first* `?` (and treats `#` as starting a fragment), so a path
// containing either character truncates the filename and lets the remainder be read as query
// parameters — exactly where mode=ro/_query_only=1 (this adapter's whole read-only enforcement)
// live. A `database` value like `/data/x.db?mode=rwc&_query_only=0` would quietly defeat the
// read-only toggle with the UI still showing the connection as read-only.
package sqlite

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func strp(s string) *string { return &s }

func TestResolveFilePath_RejectsQueryAndFragmentMetacharacters(t *testing.T) {
	tests := []struct {
		name string
		cfg  model.ResolvedConnectionConfig
	}{
		{
			name: "fields mode: a ? would let the remainder override mode=ro",
			cfg:  model.ResolvedConnectionConfig{Mode: "fields", Database: strp("/data/x.db?mode=rwc&_query_only=0")},
		},
		{
			name: "fields mode: a # starts a URI fragment",
			cfg:  model.ResolvedConnectionConfig{Mode: "fields", Database: strp("/data/x.db#fragment")},
		},
		{
			name: "uri mode: the same metacharacter reaches resolveFilePath after url.Parse strips the leading slash",
			cfg:  model.ResolvedConnectionConfig{Mode: "uri", URI: strp("sqlite:///data/x.db%3Fmode=rwc")},
		},
		{
			// F12a: SQLite's own URI filename parsing percent-decodes the path component, so a raw
			// "%" could open a different file than the one assertFileExists just confirmed exists —
			// e.g. "%2F"/"%20" decoding into a path separator or space that changes which file SQLite
			// actually opens.
			name: "fields mode: a raw % could decode into a different path once SQLite opens it",
			cfg:  model.ResolvedConnectionConfig{Mode: "fields", Database: strp("/data/x%2Fdb")},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveFilePath(tc.cfg)
			if err == nil {
				t.Fatalf("resolveFilePath: want an error rejecting the metacharacter, got none")
			}
			var ae *adapters.Error
			if !asAdapterError(err, &ae) {
				t.Fatalf("err = %v (%T), want an *adapters.Error", err, err)
			}
			if ae.Code != adapters.CodeConnect {
				t.Errorf("Code = %q, want %q", ae.Code, adapters.CodeConnect)
			}
		})
	}
}

func TestResolveFilePath_OrdinaryPathsAreUnaffected(t *testing.T) {
	path, err := resolveFilePath(model.ResolvedConnectionConfig{Mode: "fields", Database: strp("/data/normal.db")})
	if err != nil {
		t.Fatalf("resolveFilePath: %v", err)
	}
	if path != "/data/normal.db" {
		t.Errorf("path = %q, want %q", path, "/data/normal.db")
	}
}

// TestBuildDSN_WithoutTheGuardAQueryCharacterWouldSplitBeforeTheRealMode is P21 round 3 finding 11
// pinned at the string level: buildDSN splices `path` directly ahead of its own `?query`, so a
// path already containing `?mode=rwc` produces a DSN whose *first* `?` — the one SQLite's URI
// parser actually honours — introduces the attacker-supplied `mode=rwc` before this function's own
// `mode=ro`/`_query_only=1` ever appear. This is the underlying string-splicing hazard
// rejectDSNMetacharacters (exercised via resolveFilePath above) exists to keep out of buildDSN
// altogether — pinned directly here so a future change to either function can't silently drop it.
func TestBuildDSN_WithoutTheGuardAQueryCharacterWouldSplitBeforeTheRealMode(t *testing.T) {
	dsn := buildDSN("/data/x.db?mode=rwc&_query_only=0", true)
	firstQuery := strings.Index(dsn, "?")
	if firstQuery < 0 {
		t.Fatalf("dsn = %q, want at least one \"?\"", dsn)
	}
	// SQLite's URI parser reads everything after the *first* "?" as the query string — which here
	// is exactly the attacker-supplied "mode=rwc&_query_only=0", not this function's own
	// mode=ro/_query_only=1 further down the string.
	firstQueryString := dsn[firstQuery+1:]
	if !strings.HasPrefix(firstQueryString, "mode=rwc") {
		t.Fatalf("the query string SQLite would actually parse = %q, want it to start with the attacker-supplied mode=rwc — this is the hazard the guard prevents ever reaching buildDSN", firstQueryString)
	}
}

func asAdapterError(err error, target **adapters.Error) bool {
	ae, ok := err.(*adapters.Error)
	if !ok {
		return false
	}
	*target = ae
	return true
}
