package preconnect

import "strings"

// stderrTailMax mirrors preconnect.ts's STDERR_TAIL_MAX.
const stderrTailMax = 200

// tailTracker is a literal port of preconnect.ts's makeTailTracker (`:59-75`). Chunks from a pipe
// never line up with newlines — a single line can arrive split across several reads, or several
// lines can arrive in one. carry holds only the unterminated remainder after the last newline
// seen so far; without tracking that separately from last, a line already completed by a
// previous chunk's trailing newline would get silently reused as a prefix for the next chunk's
// content instead of being replaced by it. The zero value is ready to use.
type tailTracker struct {
	carry string
	last  string
}

// For a sidecar whose stderr carries no newline at all for a
// long stretch — a progress bar rewriting its line with a bare '\r' (curl, rsync, docker pull, and
// many cloud-proxy CLIs all do this), or any tool that streams without newlines — push used to
// grow carry without any cap: stderrTailMax was applied only in value(), at read time, bounding
// nothing that was *retained*. A sidecar lives as long as its connection, so carry could
// grow for the process's entire runtime, and each push cost O(len(carry)) for `t.carry + chunk`
// plus ReplaceAll/Split — O(n^2) in total stderr volume.
//
// Two independent fixes, both below: capCarry keeps only the first stderrTailMax bytes of carry
// after every push. That is safe because value() can never return more than the first
// stderrTailMax bytes of whatever `last` eventually becomes (t.last[:stderrTailMax]) — once carry
// exceeds that length, every byte past it is already unobservable, so discarding them changes
// nothing value() could ever report while keeping carry's own size bounded by a constant instead
// of by cumulative input. And a chunk with no '\r' or '\n' in it at all — the common case for a
// bare-\r progress bar — takes an early, cheap path that skips splitLines' own ReplaceAll/Split
// entirely, since there is no line boundary to find.
func (t *tailTracker) push(chunk string) {
	if !strings.ContainsAny(chunk, "\r\n") {
		t.carry += chunk
		if strings.TrimSpace(t.carry) != "" {
			t.last = t.carry
		}
		t.capCarry()
		return
	}
	lines := splitLines(t.carry + chunk)
	t.carry = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		if strings.TrimSpace(line) != "" {
			t.last = line
		}
	}
	if strings.TrimSpace(t.carry) != "" {
		t.last = t.carry
	}
	t.capCarry()
}

// capCarry bounds carry to stderrTailMax bytes, keeping the *first* stderrTailMax bytes (not the
// last) to match value()'s own prefix truncation of whatever carry eventually becomes `last` —
// see push's own comment for why that makes this a size cap with no behavioural change.
func (t *tailTracker) capCarry() {
	if len(t.carry) > stderrTailMax {
		t.carry = t.carry[:stderrTailMax]
	}
}

func (t *tailTracker) value() string {
	if len(t.last) > stderrTailMax {
		return t.last[:stderrTailMax]
	}
	return t.last
}

// splitLines mirrors JS's `.split(/\r?\n/)`.
func splitLines(s string) []string {
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}
