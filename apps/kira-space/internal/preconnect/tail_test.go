package preconnect

import (
	"strings"
	"testing"
)

func TestTailTracker(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
		want   string
	}{
		{"single line", []string{"boom\n"}, "boom"},
		{"line split across chunks", []string{"bo", "om\n"}, "boom"},
		{"several lines in one chunk", []string{"first\nsecond\nthird\n"}, "third"},
		{"trailing newline then new content is not reused as a stale prefix", []string{"first\n", "second"}, "second"},
		{"blank lines ignored", []string{"real\n\n\n"}, "real"},
		{"unterminated remainder counts as the tail", []string{"partial line, no newline yet"}, "partial line, no newline yet"},
		{"carriage return newline", []string{"one\r\ntwo\r\n"}, "two"},
		{"truncated over 200 chars", []string{strings.Repeat("x", 250) + "\n"}, strings.Repeat("x", 200)},
		// A bare '\r' (a progress-bar rewrite) is not a line
		// separator on its own (splitLines only treats '\r\n' as one) — several of these, each
		// re-writing the same visual line, must still resolve to a stable, correctly-truncated
		// value() answer, not something that depends on how the underlying reads happened to be
		// chunked.
		{"bare carriage returns are not line separators", []string{"10%\r50%\r100%"}, "10%\r50%\r100%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tr tailTracker
			for _, c := range tt.chunks {
				tr.push(c)
			}
			if got := tr.value(); got != tt.want {
				t.Errorf("value() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTailTracker_CarryNeverGrowsPastTheCap is the regression:
// push used to grow carry without any bound at all for a sidecar whose stderr carries no newline
// for a long stretch (a bare-\r progress bar, or any tool streaming without newlines) —
// stderrTailMax was applied only in value(), at read time, bounding nothing that was retained.
// This pushes many chunks with no newline anywhere in them (well past stderrTailMax in
// total) and asserts carry itself never exceeds the cap after any single push — it fails against
// the pre-fix tailTracker, whose carry would grow to the full, unbounded total instead.
func TestTailTracker_CarryNeverGrowsPastTheCap(t *testing.T) {
	var tr tailTracker
	chunk := strings.Repeat("progress...", 50) // 550 bytes, no '\r' or '\n' at all
	for i := 0; i < 20; i++ {
		tr.push(chunk)
		if len(tr.carry) > stderrTailMax {
			t.Fatalf("after push %d: len(carry) = %d, want at most stderrTailMax (%d)", i, len(tr.carry), stderrTailMax)
		}
	}
	// value() must still report a stable, correctly-truncated answer: the first stderrTailMax
	// bytes of the (in principle unbounded) stream, exactly what an uncapped tracker would also
	// have reported for value() — the cap changes retention, never the observable answer.
	want := strings.Repeat(chunk, 20)[:stderrTailMax]
	if got := tr.value(); got != want {
		t.Errorf("value() = %q, want %q", got, want)
	}
}

// TestTailTracker_LongUnterminatedLineEventuallyTerminatedStillTruncatesToTheOriginalPrefix proves
// capCarry's own correctness claim: capping carry to its first stderrTailMax bytes (not the last)
// must never change what a *terminated* line eventually reports through value(), because value()
// only ever returns a prefix of `last` in the first place.
func TestTailTracker_LongUnterminatedLineEventuallyTerminatedStillTruncatesToTheOriginalPrefix(t *testing.T) {
	var tr tailTracker
	first := strings.Repeat("a", 150)
	second := strings.Repeat("b", 150) // pushes carry well past stderrTailMax before any newline
	tr.push(first)
	tr.push(second)
	tr.push("\n")

	want := (first + second)[:stderrTailMax]
	if got := tr.value(); got != want {
		t.Errorf("value() = %q, want %q (the first stderrTailMax bytes of the true, uncapped line)", got, want)
	}
}
