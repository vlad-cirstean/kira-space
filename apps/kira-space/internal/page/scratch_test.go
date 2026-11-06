package page

import (
	"bytes"
	"testing"
)

// Go's truncateUTF8ToBoundary used to back off with
// utf8.DecodeLastRune, which treats any trailing byte invalid in isolation as "back off one more"
// — unbounded for a run of bytes that merely resemble a UTF-8 lead byte without ever completing
// one, clipping a value far below the intended limit. page.ts's own truncateUtf8ToBoundary only
// backs off over genuine continuation bytes (at most 3 for any real UTF-8 sequence), which is what
// these pin.

// A run of Latin-1-style bytes (à-ÿ, 0xE0-0xFF) is each individually invalid as a standalone UTF-8
// rune (DecodeLastRune's own reason to keep backing off, pre-fix) but is NOT a continuation-byte
// pattern (RuneStart's own check) — the fixed function must cut at exactly maxBytes, not walk
// back through the whole run.
func TestTruncateUTF8ToBoundary_LatinOneStyleRunIsNotTreatedAsContinuationBytes(t *testing.T) {
	b := bytes.Repeat([]byte{0xE9}, 20) // 0xE9 = 'é' in Latin-1; a valid 3-byte UTF-8 lead byte's
	// own first byte, but never followed by real continuation bytes here.
	got := truncateUTF8ToBoundary(b, 10)
	if len(got) != 10 {
		t.Fatalf("len(got) = %d, want exactly 10 (no back-off past a non-continuation byte)", len(got))
	}
}

// A genuine multi-byte UTF-8 rune split by the cut must still be dropped entirely, matching
// page.ts's own boundary rule (and Go's pre-fix behavior for this exact, valid-UTF-8 case).
func TestTruncateUTF8ToBoundary_DropsAGenuinelySplitMultiByteRune(t *testing.T) {
	s := "ab€" // 'a', 'b', then '€' (E2 82 AC, a 3-byte rune) — 5 bytes total.
	b := []byte(s)
	if len(b) != 5 {
		t.Fatalf("setup: len(b) = %d, want 5", len(b))
	}
	got := truncateUTF8ToBoundary(b, 4) // cuts one byte into the 3-byte rune (E2 82, missing AC).
	if string(got) != "ab" {
		t.Errorf("got %q, want %q (the incomplete rune dropped entirely)", got, "ab")
	}
}

// The back-off is capped at utf8.UTFMax-1 (3) bytes even across an adversarial run of bytes that
// do match the continuation-byte pattern (0x80-0xBF) with no real lead byte ever preceding them —
// belt-and-suspenders alongside the RuneStart check itself.
func TestTruncateUTF8ToBoundary_BackOffNeverExceedsThreeBytes(t *testing.T) {
	b := bytes.Repeat([]byte{0x80}, 20) // every byte matches the continuation-byte bit pattern.
	got := truncateUTF8ToBoundary(b, 10)
	if len(got) != 7 {
		t.Errorf("len(got) = %d, want exactly 7 (at most 3 bytes backed off from the 10-byte cut)", len(got))
	}
}

func TestTruncateUTF8ToBoundary_NoTruncationNeeded(t *testing.T) {
	b := []byte("hello")
	got := truncateUTF8ToBoundary(b, 10)
	if string(got) != "hello" {
		t.Errorf("got %q, want the input unchanged", got)
	}
}
