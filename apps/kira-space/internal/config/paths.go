// Package config resolves this app's on-disk locations and dev/packaged mode.
//
// The path logic itself is repo-root internal/kirapaths. This file supplies Kira Space's own env
// var (KIRA_SPACE_HOME), directory name (.kira-space) and database file name (kira.db), plus the
// Studio side's (KIRA_HOME, .kira-studio, kira.db) until the home fold.
package config

import "github.com/kirathecat/kira-studio/internal/kirapaths"

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

// EnsureLayout creates both homes and the Space logs directory at 0700, tightening an existing
// loose directory too, not only on first create.
func EnsureLayout() error {
	if err := EnsureLayoutAt(KiraSpaceHome()); err != nil {
		return err
	}
	return kirapaths.EnsureLayoutAt(StudioHome())
}

// EnsureLayoutAt is EnsureLayout against an explicit home dir — see DbPathAt.
func EnsureLayoutAt(home string) error {
	return kirapaths.EnsureLayoutAt(home)
}

// StudioHome is the Studio data directory, honouring KIRA_HOME.
func StudioHome() string {
	return kirapaths.Home("KIRA_HOME", ".kira-studio")
}

// StudioDbPath is the Studio database file, kira.db under StudioHome.
func StudioDbPath() string {
	return StudioDbPathAt(StudioHome())
}

// StudioDbPathAt is StudioDbPath against an explicit home dir.
func StudioDbPathAt(home string) string {
	return kirapaths.DbPathAt(home, "kira.db")
}

// ModuleDbPathAt is the Studio modules' database file, studio.db under the Space home, until the DB
// merge (P544).
func ModuleDbPathAt(home string) string {
	return kirapaths.DbPathAt(home, "studio.db")
}
