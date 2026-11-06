// Package homefold folds the retired Kira Studio home (~/.kira-studio) into the Kira Space home once (P539).
//
// Files moves the whole legacy folder into a backup and lands its Studio database, connection and API
// config folders, tokens and prompt overrides in the Space home. Runs copies the Studio runs rows, run
// files and automation folders into the Space stack. Both stages are idempotent and resumable: a marker
// row in the Space database records progress and every step checks its own end state. This package is the
// only code allowed to name KIRA_HOME or .kira-studio.
package homefold

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/internal/fsmove"
)

// ErrStudioRunning means the old Studio holds its app.lock; nothing was moved.
var ErrStudioRunning = errors.New("homefold: Kira Studio is running")

const markerKey = "homefold.p539"

// Marker states.
const (
	stateMoving = "moving"
	stateFiles  = "files"
	stateRuns   = "runs"
	stateDone   = "done"
)

// Options names the databases and homes. Rename, Step and Now are test seams.
type Options struct {
	Space     *sql.DB // marker and runs target
	SpaceHome string
	Legacy    string  // "" folds nothing
	Studio    *sql.DB // Runs only: <space>/studio.db, opened after Files
	Now       func() time.Time
	Rename    func(oldpath, newpath string) error // nil is os.Rename
	Step      func(name string) error             // nil in production; returns an error to interrupt
}

type marker struct {
	State      string            `json:"state"`
	Fresh      bool              `json:"fresh,omitempty"`
	Backup     string            `json:"backup,omitempty"`
	Legacy     string            `json:"legacy,omitempty"`
	Scripts    map[string]string `json:"scripts,omitempty"`
	Runs       map[string]string `json:"runs,omitempty"`
	Reregister bool              `json:"reregister,omitempty"`
	RowsCopied bool              `json:"rowsCopied,omitempty"`
}

func (o *Options) step(name string) error {
	if o.Step == nil {
		return nil
	}
	return o.Step(name)
}

func (o *Options) rename(from, to string) error {
	if o.Rename == nil {
		return os.Rename(from, to)
	}
	return o.Rename(from, to)
}

func (o *Options) now() time.Time {
	if o.Now == nil {
		return time.Now()
	}
	return o.Now()
}

// LegacyHome is the folder Files folds from, "" for none. KIRA_HOME is honoured as this source only;
// an overridden Space home never pulls the real ~/.kira-studio, which keeps dev and test runs clean.
func LegacyHome() string {
	return legacyHome(config.KiraSpaceHome())
}

// Reappeared reports whether legacy holds files again after the fold: a Kira Studio that is still
// installed recreated it. Files never folds twice, so those files stay unread.
func Reappeared(legacy string) bool {
	if legacy == "" {
		return false
	}
	empty, err := fsmove.IsEmptyDir(legacy)
	return err == nil && !empty
}

func legacyHome(spaceHome string) string {
	legacy := os.Getenv("KIRA_HOME")
	switch {
	case legacy != "":
	case os.Getenv("KIRA_SPACE_HOME") != "", testing.Testing():
		return ""
	default:
		h, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		legacy = filepath.Join(h, ".kira-studio")
	}
	if overlaps(legacy, spaceHome) {
		slog.Warn("homefold: legacy home overlaps the Space home, nothing folded", "scope", "homefold", "legacy", legacy)
		return ""
	}
	return legacy
}

func overlaps(a, b string) bool {
	return within(a, b) || within(b, a)
}

// within reports whether child equals or sits inside parent.
func within(child, parent string) bool {
	rel, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// NeedsReregister reports whether the DB MCP registration still names the moved helper script.
func NeedsReregister(db *sql.DB) (bool, error) {
	m, _, err := readMarker(db)
	return m.Reregister, err
}

// ClearReregister records that the DB MCP registration was rewritten.
func ClearReregister(db *sql.DB) error {
	m, found, err := readMarker(db)
	if err != nil || !found {
		return err
	}
	m.Reregister = false
	return writeMarker(db, m)
}

func readMarker(db *sql.DB) (m marker, found bool, err error) {
	var raw string
	switch err = db.QueryRow(`SELECT value FROM settings WHERE key = ?`, markerKey).Scan(&raw); {
	case errors.Is(err, sql.ErrNoRows):
		return marker{}, false, nil
	case err != nil:
		return marker{}, false, fmt.Errorf("homefold: read marker: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return marker{}, false, fmt.Errorf("homefold: marker: %w", err)
	}
	return m, true, nil
}

func writeMarker(db interface {
	Exec(query string, args ...any) (sql.Result, error)
}, m marker) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, markerKey, string(raw)); err != nil {
		return fmt.Errorf("homefold: write marker: %w", err)
	}
	return nil
}
