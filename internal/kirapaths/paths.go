// Package kirapaths is the repo-root generalization of the on-disk layout logic
// apps/kira-studio/internal/config/paths.go used to own outright (KiraHome/DbPath/LogsDir/
// EnsureLayout): the same shape was needed for apps/kira-space's own, separate home
// directory (KIRA_SPACE_HOME/~/.kira-space, never sharing a home, or a database with
// Kira Studio). Rather than duplicate this
// logic per app, each app's own internal/config becomes a thin wrapper parameterized by its own
// env-var name and directory name, calling straight through to the functions here.
package kirapaths

import (
	"os"
	"path/filepath"
)

// Home resolves an app's data directory: envVar if set and non-empty, else
// $HOME/dirName (falling back to "."/dirName if the OS can't resolve a home directory at all —
// every process on the machine hits that the same way).
func Home(envVar, dirName string) string {
	if home := os.Getenv(envVar); home != "" {
		return home
	}
	requireTestHome(envVar)
	dir, err := os.UserHomeDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, dirName)
}

// DbPathAt is the app's database file path under an explicit home dir — a caller resolves its own
// home first (via Home, honouring its own env var) rather than this package doing an env lookup
// itself, the same "explicit home, not a hidden env read" shape paths.go's own DbPathAt used
// (letting a test avoid t.Setenv's parallel-test panic). fileName is the app's own
// database file name ("kira.db" / a future app's own choice).
func DbPathAt(home, fileName string) string {
	return filepath.Join(home, fileName)
}

// LogsDirAt is the app's logs directory under an explicit home dir.
func LogsDirAt(home string) string {
	return filepath.Join(home, "logs")
}

// EnsureLayoutAt creates home and its logs subdirectory at 0700, tightening an existing loose
// directory too, not only on first create — the same permissions paths.go's own EnsureLayoutAt
// used.
func EnsureLayoutAt(home string) error {
	for _, dir := range []string{home, LogsDirAt(home)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// SharedAgentDir is the agent folder both apps read (~/.kira-space/agent): Kira Space hosts and
// edits it, Kira Studio only reads it.
func SharedAgentDir() string {
	return filepath.Join(Home("KIRA_SPACE_HOME", ".kira-space"), "agent")
}
