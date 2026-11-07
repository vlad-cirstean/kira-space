package dbmcp

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCapLogBytes(t *testing.T) {
	big := strings.Repeat("é", scriptLogsMaxBytes) // 2 bytes per rune
	for _, fromEnd := range []bool{true, false} {
		got, cut := capLogBytes([]ScriptLogLine{{Seq: 1, Text: "a"}, {Seq: 2, Text: big}}, fromEnd)
		if !cut || len(got) != 1 {
			t.Fatalf("fromEnd=%v: cut=%v lines=%d", fromEnd, cut, len(got))
		}
		if len(got[0].Text) > scriptLogsMaxBytes || !utf8.ValidString(got[0].Text) {
			t.Fatalf("fromEnd=%v: line %d bytes, valid=%v", fromEnd, len(got[0].Text), utf8.ValidString(got[0].Text))
		}
	}
	small := []ScriptLogLine{{Seq: 1, Text: "x"}}
	if got, cut := capLogBytes(small, true); cut || len(got) != 1 {
		t.Fatalf("small line changed: cut=%v", cut)
	}
}
