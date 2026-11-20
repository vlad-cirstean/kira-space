package appwire

import (
	"bytes"
	"testing"
)

// v0.3.x only: drops with retiredMemoryCommand at the v0.4.0 minor bump.
func TestRetiredMemoryCommandExitCodes(t *testing.T) {
	for word, want := range map[string]int{"memory-check": 0, "memory-mcp": 1, "memory-ingest": 1, "memory-embed": 1} {
		var buf bytes.Buffer
		if got := retiredMemoryCommand(word, &buf); got != want || buf.Len() == 0 {
			t.Errorf("%s: code %d (want %d), stderr %q", word, got, want, buf.String())
		}
		if code, ok := RunArgvShim([]string{"kira-space", word}); !ok || code != want {
			t.Errorf("%s via shim: code %d ok %v", word, code, ok)
		}
	}
}
