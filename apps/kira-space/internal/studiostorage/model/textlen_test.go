package model

import "testing"

func TestTruncateUTF16(t *testing.T) {
	// "😀" is 2 UTF-16 units: a cut inside it must drop the whole rune.
	if got, cut := TruncateUTF16("ab😀cd", 3); got != "ab" || !cut {
		t.Errorf("TruncateUTF16 = %q, %v, want %q, true", got, cut, "ab")
	}
	if got, cut := TruncateUTF16("ab😀", 4); got != "ab😀" || cut {
		t.Errorf("TruncateUTF16 = %q, %v, want unchanged", got, cut)
	}
	if n := UTF16Len("日本語"); n != 3 {
		t.Errorf("UTF16Len = %d, want 3 (units, not bytes)", n)
	}
}
