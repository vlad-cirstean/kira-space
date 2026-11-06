package kubernetes

import (
	"reflect"
	"strings"
	"testing"
)

func texts(lines []LogLine) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, l.Text)
	}
	return out
}

func TestLineFramerSplitsAcrossChunks(t *testing.T) {
	f := newLineFramer(false)
	var got []LogLine
	for _, chunk := range []string{"hel", "lo\nwor", "ld\n", "tail"} {
		got = append(got, f.write([]byte(chunk))...)
	}
	if want := []string{"hello", "world"}; !reflect.DeepEqual(texts(got), want) {
		t.Fatalf("got %v, want %v", texts(got), want)
	}
	if rest := f.flush(); !reflect.DeepEqual(texts(rest), []string{"tail"}) {
		t.Fatalf("flush got %v", texts(rest))
	}
	if again := f.flush(); len(again) != 0 {
		t.Fatalf("second flush must be empty, got %v", texts(again))
	}
}

func TestLineFramerStripsOneCR(t *testing.T) {
	f := newLineFramer(false)
	got := f.write([]byte("a\r\n\nb\r\r\n"))
	if want := []string{"a", "", "b\r"}; !reflect.DeepEqual(texts(got), want) {
		t.Fatalf("got %q, want %q", texts(got), want)
	}
}

func TestLineFramerTruncatesAndSkipsToNewline(t *testing.T) {
	f := newLineFramer(false)
	long := strings.Repeat("x", maxLogLine+5000)
	var got []LogLine
	got = append(got, f.write([]byte(long[:70000]))...)
	got = append(got, f.write([]byte(long[70000:]+"\nnext\n"))...)
	if len(got) != 2 {
		t.Fatalf("want a truncated line then next, got %d lines", len(got))
	}
	if want := strings.Repeat("x", maxLogLine) + truncationMarker; got[0].Text != want {
		t.Fatalf("truncated line has %d bytes, want %d", len(got[0].Text), len(want))
	}
	if got[1].Text != "next" {
		t.Fatalf("line after a truncated one lost: %q", got[1].Text)
	}
}

func TestLineFramerSplitsTimestampPrefix(t *testing.T) {
	f := newLineFramer(true)
	got := f.write([]byte("2026-01-02T03:04:05.123456789Z hello world\nnot-a-time stays\n"))
	if len(got) != 2 || got[0].TS != "2026-01-02T03:04:05.123456789Z" || got[0].Text != "hello world" {
		t.Fatalf("timestamp line: %+v", got)
	}
	if got[1].TS != "" || got[1].Text != "not-a-time stays" {
		t.Fatalf("non-timestamp line: %+v", got[1])
	}
}
