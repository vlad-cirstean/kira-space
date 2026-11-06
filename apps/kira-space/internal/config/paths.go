// Package config resolves this app's on-disk locations and dev/packaged mode.
//
// The path logic itself is repo-root internal/kirapaths. This file supplies Kira Space's own env
// var (KIRA_SPACE_HOME), directory name (.kira-space) and database file name (kira.db).
package config

import (
	"os"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/kirapaths"
)

// KiraSpaceHome is this app's data directory, honouring KIRA_SPACE_HOME.
func KiraSpaceHome() string {
	return kirapaths.Home("KIRA_SPACE_HOME", ".kira-space")
}

// DbPath is Kira Space's own database file, named kira.db — its own file under its own home, never
// the same file Kira Studio opens.
func DbPath() string {
	return DbPathAt(KiraSpaceHome())
}

// DbPathAt is DbPath against an explicit home dir instead of KiraSpaceHome()'s env lookup — lets a
// test pass its own t.TempDir() directly rather than t.Setenv, the same reason Kira Studio's own
// config.DbPathAt exists (v1.4 P1).
func DbPathAt(home string) string {
	return kirapaths.DbPathAt(home, "kira.db")
}

func LogsDir() string {
	return LogsDirAt(KiraSpaceHome())
}

// LogsDirAt is LogsDir against an explicit home dir — see DbPathAt.
func LogsDirAt(home string) string {
	return kirapaths.LogsDirAt(home)
}

// EnsureLayout creates the home and its logs directory at 0700, tightening an existing loose
// directory too, not only on first create.
func EnsureLayout() error {
	return EnsureLayoutAt(KiraSpaceHome())
}

// EnsureLayoutAt is EnsureLayout against an explicit home dir — see DbPathAt. It also creates
// <home>/config at 0700: a config folder watcher adds its parent and fails when it is missing.
func EnsureLayoutAt(home string) error {
	if err := kirapaths.EnsureLayoutAt(home); err != nil {
		return err
	}
	root := configfs.Root(home)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	return os.Chmod(root, 0o700)
}

// ModuleDbPathAt is the Studio modules' database file, studio.db under the Space home, until the DB
// merge (P544).
func ModuleDbPathAt(home string) string {
	return kirapaths.DbPathAt(home, "studio.db")
}
