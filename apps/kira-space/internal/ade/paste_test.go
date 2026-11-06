package ade

import "testing"

func TestNormalizeMessageStripsControls(t *testing.T) {
	cases := map[string]string{
		"a\r\nb\rc":                  "a\nb\nc",
		"x\x1b[20\x1b[201~1~y":       "x[20[201~1~y",
		"\x1b[201~\x03\x04ok\tz\x7f": "[201~ok\tz",
	}
	for in, want := range cases {
		got := normalizeMessage(in)
		if got != want {
			t.Errorf("normalizeMessage(%q) = %q, want %q", in, got, want)
		}
		for _, r := range got {
			if r == 0x1b {
				t.Errorf("ESC survived in %q", got)
			}
		}
	}
}
