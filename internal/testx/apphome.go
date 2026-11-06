package testx

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// RunWithTempHomes points KIRA_SPACE_HOME and KIRA_MEMORY_HOME at fresh dirs under one temp root for the
// whole test binary, runs m, then removes the root. Child processes the tests spawn inherit both.
// The env var names are hardcoded: this repo-root package must not import app packages.
// Use: func TestMain(m *testing.M) { os.Exit(testx.RunWithTempHomes(m)) }
func RunWithTempHomes(m *testing.M) int {
	return runWithEnv(m, func(root string) map[string]string {
		return map[string]string{
			"KIRA_SPACE_HOME":  filepath.Join(root, "space"),
			"KIRA_MEMORY_HOME": filepath.Join(root, "memory"),
		}
	})
}

// RunWithTempUserHome points HOME at a fresh temp dir for the whole test binary, for tests whose
// child processes (an interactive shell) write dotfiles there, then removes it after m runs.
func RunWithTempUserHome(m *testing.M) int {
	return runWithEnv(m, func(root string) map[string]string {
		return map[string]string{"HOME": root}
	})
}

func runWithEnv(m *testing.M, vars func(root string) map[string]string) int {
	root, err := os.MkdirTemp("", "kira-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "testx: temp home:", err)
		return 1
	}
	for k, v := range vars(root) {
		if err := os.Setenv(k, v); err != nil {
			fmt.Fprintln(os.Stderr, "testx: setenv:", err)
			_ = os.RemoveAll(root)
			return 1
		}
	}
	code := m.Run()
	_ = os.RemoveAll(root)
	return code
}
