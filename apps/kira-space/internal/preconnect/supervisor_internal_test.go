package preconnect

import (
	"strings"
	"testing"
	"time"
)

// Lowers settleWindow/killGrace (D9) so the sidecar and kill-escalation tests in
// supervisor_test.go don't each cost 2s of wall clock. This file is internal (package
// preconnect, not preconnect_test) purely to reach these unexported vars — it registers no
// tests of its own.
func init() {
	settleWindow = 80 * time.Millisecond
	killGrace = 120 * time.Millisecond
}

// TestWithAugmentedPath_NeverLeadsWithAnEmptyElement: a leading empty PATH element is `.` to
// /bin/sh, and cmd.Dir is the user's own home directory — an arbitrary file dropped there (named
// kubectl, aws, psql, ...) would be executed in preference to nothing. Covers both ways
// withAugmentedPath used to produce one: no PATH= entry at all, and a PATH= entry present
// but empty.
func TestWithAugmentedPath_NeverLeadsWithAnEmptyElement(t *testing.T) {
	assertNoLeadingColon := func(t *testing.T, env []string) {
		t.Helper()
		for _, kv := range env {
			if !strings.HasPrefix(kv, "PATH=") {
				continue
			}
			value := strings.TrimPrefix(kv, "PATH=")
			if strings.HasPrefix(value, ":") {
				t.Fatalf("PATH value %q starts with a leading empty element (`.` to /bin/sh)", value)
			}
		}
	}

	t.Run("no PATH entry at all", func(t *testing.T) {
		out := withAugmentedPath([]string{"HOME=/home/x"})
		assertNoLeadingColon(t, out)
	})

	t.Run("PATH entry present but empty", func(t *testing.T) {
		out := withAugmentedPath([]string{"PATH=", "HOME=/home/x"})
		assertNoLeadingColon(t, out)
	})

	t.Run("PATH entry present and non-empty is appended to, not replaced", func(t *testing.T) {
		out := withAugmentedPath([]string{"PATH=/usr/bin:/bin"})
		found := false
		for _, kv := range out {
			if kv == "PATH=/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected the existing PATH to be extended, got %v", out)
		}
	})
}
