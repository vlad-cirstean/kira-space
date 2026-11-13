package harness

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A binary that appears earlier on an unchanged PATH wins at the next resolve: only the
// login-shell probe is cached, never a PATH hit.
func TestResolveCLI_newerBinaryEarlierOnPathWins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix executables")
	}
	older, newer := t.TempDir(), t.TempDir()
	name := "kira-test-cli-" + filepath.Base(older)
	mk := func(dir string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	oldBin := mk(older)
	t.Setenv("PATH", newer+string(os.PathListSeparator)+older)
	spec := CLISpec{Name: name, Product: "Test"}
	got, _, err := ResolveCLI(context.Background(), spec, CLIDeps{})
	if err != nil || got != oldBin {
		t.Fatalf("first resolve = %q, %v; want %q", got, err, oldBin)
	}
	newBin := mk(newer)
	got, _, err = ResolveCLI(context.Background(), spec, CLIDeps{})
	if err != nil || got != newBin {
		t.Fatalf("after reinstall earlier on PATH = %q, %v; want %q", got, err, newBin)
	}
}
