// Package configfold moves the config files from the Space home root into <home>/config once (P540).
//
// Each config folder moves to <home>/config/<module> and the prompt overrides to
// <home>/config/prompts. The step is idempotent and resumable: a marker row in the Space database records
// progress, a backup of every legacy entry is taken first, and every sub-step checks its own end state.
package configfold

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adeflow"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/internal/agentctx"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/fsmove"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

const markerKey = "configfold.p540"

// Marker states; absent is the state before the first run.
const (
	stateMoving = "moving"
	stateDone   = "done"
)

// Options names the Space database and home. Rename and Step are test seams.
type Options struct {
	Space  *sql.DB // marker
	Home   string  // the Space home
	Now    func() time.Time
	Rename func(oldpath, newpath string) error // nil is os.Rename
	Step   func(name string) error             // nil in production; returns an error to interrupt
}

type marker struct {
	State  string `json:"state"`
	Fresh  bool   `json:"fresh,omitempty"`
	Backup string `json:"backup,omitempty"`
}

// entry is one legacy item at <home>/<name>: a folder moving to <home>/config/<name>, or the prompts file
// moving into <home>/config/prompts.
type entry struct {
	name string
	file bool
}

// entries lists every legacy item in move order.
var entries = []entry{
	{name: connfiles.Folder},
	{name: apifiles.Folder},
	{name: scripts.Folder},
	{name: adeflow.WorkflowsFolder},
	{name: adeflow.StepsFolder},
	{name: repocfg.Folder},
	{name: agentctx.Folder},
	{name: llmprompts.File, file: true},
}

func (e entry) src(home string) string { return filepath.Join(home, e.name) }

func (e entry) dst(home string) string {
	if e.file {
		return filepath.Join(configfs.ModuleDir(home, llmprompts.Folder), e.name)
	}
	return configfs.ModuleDir(home, e.name)
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

// Run moves the config entries once. A failed run keeps its marker; the next run resumes.
func Run(o Options) error {
	m, found, err := readMarker(o.Space)
	if err != nil {
		return err
	}
	if found && m.State == stateDone {
		return nil
	}
	if !found {
		legacy := false
		for _, e := range entries {
			if fsmove.Exists(e.src(o.Home)) {
				legacy = true
				break
			}
		}
		if !legacy {
			return writeMarker(o.Space, marker{State: stateDone, Fresh: true})
		}
		m = marker{
			State:  stateMoving,
			Backup: filepath.Join(o.Home, "backups", "p540-"+o.now().UTC().Format("20060102T150405Z")),
		}
		if err := writeMarker(o.Space, m); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(m.Backup, 0o700); err != nil {
		return fmt.Errorf("configfold: backup dir: %w", err)
	}
	for _, e := range entries {
		if err := o.step("backup/" + e.name); err != nil {
			return err
		}
		if err := backup(e, o.Home, m.Backup); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(configfs.Root(o.Home), 0o700); err != nil {
		return fmt.Errorf("configfold: config root: %w", err)
	}
	for _, e := range entries {
		if err := o.step("move/" + e.name); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(e.dst(o.Home)), 0o700); err != nil {
			return fmt.Errorf("configfold: %s: %w", e.name, err)
		}
		if err := o.move(e); err != nil {
			return err
		}
	}
	if err := o.step("done"); err != nil {
		return err
	}
	m.State = stateDone
	if err := writeMarker(o.Space, m); err != nil {
		return err
	}
	slog.Info("config fold done", "scope", "startup", "backup", m.Backup)
	return nil
}

func readMarker(db *sql.DB) (m marker, found bool, err error) {
	var raw string
	switch err = db.QueryRow(`SELECT value FROM settings WHERE key = ?`, markerKey).Scan(&raw); {
	case errors.Is(err, sql.ErrNoRows):
		return marker{}, false, nil
	case err != nil:
		return marker{}, false, fmt.Errorf("configfold: read marker: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return marker{}, false, fmt.Errorf("configfold: marker: %w", err)
	}
	return m, true, nil
}

func writeMarker(db *sql.DB, m marker) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, markerKey, string(raw)); err != nil {
		return fmt.Errorf("configfold: write marker: %w", err)
	}
	return nil
}
