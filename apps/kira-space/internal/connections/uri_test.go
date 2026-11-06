package connections

import (
	"testing"

	"github.com/kirathecat/kira-studio/internal/testx"
)

func strPtr(s string) *string { return &s }

// derefOrNil is testx.DerefOrNil (P107 I2-28).
var derefOrNil = testx.DerefOrNil

// TestURIHasAmbiguousPassword: findAuthority ends the authority at the first
// /, ? or # after "://", so an unencoded one of those inside a password truncates the detected
// authority before the real '@' — userinfoPassword then reports no password at all, and
// foldPassword would inject a second one. This is the guard that rejects that shape at
// validateMode instead.
func TestURIHasAmbiguousPassword(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want bool
	}{
		{"raw slash in password", "postgres://u:pa/ss@h/db", true},
		{"raw question mark in password", "postgres://u:pa?ss@h/db", true},
		{"raw hash in password", "postgres://u:pa#ss@h/db", true},
		{"IPv6 host, later @ in query", "postgres://[::1]:5432/db?application_name=me@corp", false},
		{"userinfo plus IPv6 host, later @ in query", "postgres://user@[::1]:5432/db?options=a@b", false},
		{"password plus host, later @ in query", "postgres://u:p@h:5432/db?options=a@b", false},
		{"well-formed, no delimiter before the real @", "postgres://u:pass@h/db", false},
		{"well-formed with query string", "postgres://u:pass@h/db?sslmode=require", false},
		{"percent-encoded slash in password", "mysql://root:my%2Fpass@h/db", false},
		{"host:port, no userinfo at all", "postgres://h:5432/db", false},
		{"host:port with an unrelated later @ in the path", "postgres://h:5432/db@extra", false},
		{"no scheme separator", "not a uri", false},
		{"no delimiter after the authority at all", "postgres://u:pa/ss@h", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := uriHasAmbiguousPassword(tt.uri); got != tt.want {
				t.Errorf("uriHasAmbiguousPassword(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}

func TestURIPasswordRoundTripSurvivesSpecialCharacters(t *testing.T) {
	// A password containing '@' and ':' must survive an inject-then-read round trip: inject
	// percent-encodes it, so the '@'/':' inside it never gets mistaken for userinfo/authority
	// delimiters when read back.
	const original = "p@ss:w0rd"
	uri := injectURIPassword("postgresql://u@h/db", strPtr(original))
	got := userinfoPassword(uri)
	if got == nil || *got != original {
		t.Errorf("round-tripped password = %v, want %q", derefOrNil(got), original)
	}
}
