// Package appmerge carries a Kira Studio install into the merged app on first launch (P537).
//
// Both databases stay until P538 and P540. This moves what the one UI needs from the Studio side:
// tabs of closed Studio windows, layout of Studio modes, shared settings when Space has none, and
// prompt overrides. Run is idempotent and resumable: a marker row in the Space database records
// progress, and every step checks its own end state.
package appmerge

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/renameio/v2"
	"github.com/kirathecat/kira-studio/internal/ids"
)

const markerKey = "appmerge.p537"

// Options names the two databases and homes. Ephemeral reports window keys (host views, captures,
// opened files, proofs) that never carry user tabs.
type Options struct {
	Space, Studio         *sql.DB
	SpaceHome, StudioHome string
	Ephemeral             func(key string) bool
	Now                   func() time.Time
}

type marker struct {
	State  string `json:"state"`
	Fresh  bool   `json:"fresh"`
	Backup string `json:"backup,omitempty"`
}

var studioModes = []string{"studio", "api", "docker", "kubernetes"}

// Run migrates once. A finished marker makes it a no-op.
func Run(o Options) error {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Ephemeral == nil {
		o.Ephemeral = func(string) bool { return false }
	}
	m, found, err := readMarker(o.Space)
	if err != nil {
		return err
	}
	if found && m.State == "done" {
		return nil
	}
	if !found {
		m.Fresh, err = spaceFresh(o.Space)
		if err != nil {
			return err
		}
		has, err := hasStudioData(o)
		if err != nil {
			return err
		}
		if !has {
			return writeMarker(o.Space, marker{State: "done", Fresh: m.Fresh})
		}
	}
	m.State = "running"
	if m.Backup == "" || !dirExists(m.Backup) {
		if m.Backup, err = backup(o); err != nil {
			return err
		}
	}
	if err := writeMarker(o.Space, m); err != nil {
		return err
	}
	if err := rekeyTabs(o); err != nil {
		return fmt.Errorf("appmerge: tabs: %w", err)
	}
	if err := importLayout(o); err != nil {
		return fmt.Errorf("appmerge: layout: %w", err)
	}
	if m.Fresh {
		if err := importSettings(o); err != nil {
			return fmt.Errorf("appmerge: settings: %w", err)
		}
	}
	if err := mergePrompts(o); err != nil {
		return fmt.Errorf("appmerge: prompts: %w", err)
	}
	m.State = "done"
	slog.Info("appmerge: Studio data carried over", "scope", "appmerge", "backup", m.Backup)
	return writeMarker(o.Space, m)
}

func readMarker(db *sql.DB) (m marker, found bool, err error) {
	var raw string
	switch err = db.QueryRow(`SELECT value FROM settings WHERE key = ?`, markerKey).Scan(&raw); {
	case errors.Is(err, sql.ErrNoRows):
		return marker{}, false, nil
	case err != nil:
		return marker{}, false, fmt.Errorf("appmerge: read marker: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return marker{}, false, fmt.Errorf("appmerge: marker: %w", err)
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
		return fmt.Errorf("appmerge: write marker: %w", err)
	}
	return nil
}

func spaceFresh(db *sql.DB) (bool, error) {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&n); err != nil {
		return false, fmt.Errorf("appmerge: count settings: %w", err)
	}
	return n == 0, nil
}

// hasStudioData is false for an install that never ran Studio: nothing to carry, nothing to back up.
func hasStudioData(o Options) (bool, error) {
	var n int
	if err := o.Studio.QueryRow(`SELECT
		(SELECT COUNT(*) FROM settings) + (SELECT COUNT(*) FROM ui_layout) + (SELECT COUNT(*) FROM tabs)`).Scan(&n); err != nil {
		return false, fmt.Errorf("appmerge: count studio rows: %w", err)
	}
	if n > 0 {
		return true, nil
	}
	_, err := os.Stat(filepath.Join(o.StudioHome, "llm-prompts.json"))
	return err == nil, nil
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func backup(o Options) (string, error) {
	dir := filepath.Join(o.SpaceHome, "backups", "p537-"+o.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("appmerge: backup dir: %w", err)
	}
	for name, db := range map[string]*sql.DB{"space.db": o.Space, "studio.db": o.Studio} {
		dst := filepath.Join(dir, name)
		if _, err := db.Exec(`VACUUM INTO ?`, dst); err != nil {
			return "", fmt.Errorf("appmerge: backup %s: %w", name, err)
		}
	}
	return dir, nil
}

type studioTab struct {
	id, windowKey string
	order         int
	active        bool
}

// rekeyTabs moves Studio tabs whose window has no Space window onto Space's primary window,
// appended after every tab it already holds. It creates that window when Space has none.
func rekeyTabs(o Options) error {
	spaceKeys, err := windowKeys(o.Space)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(spaceKeys))
	primary := ""
	for _, k := range spaceKeys {
		known[k] = true
		if primary == "" && !o.Ephemeral(k) {
			primary = k
		}
	}
	orphans, err := orphanTabs(o.Studio, known)
	if err != nil {
		return err
	}
	if len(orphans) == 0 {
		return nil
	}
	if primary == "" {
		primary = ids.NewID()
		if _, err := o.Space.Exec(`INSERT INTO windows (key, "order") VALUES (?, 0)`, primary); err != nil {
			return err
		}
	}
	tx, err := o.Studio.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO windows (key, "order")
		VALUES (?, (SELECT COALESCE(MAX("order"), -1) + 1 FROM windows))`, primary); err != nil {
		return err
	}
	var next int
	var activeTaken bool
	if err := tx.QueryRow(`SELECT COALESCE(MAX("order"), -1) + 1, COALESCE(MAX(active), 0) FROM tabs WHERE window_key = ?`,
		primary).Scan(&next, &activeTaken); err != nil {
		return err
	}
	var spaceNext int
	var spaceActive bool
	if err := o.Space.QueryRow(`SELECT COALESCE(MAX("order"), -1) + 1, COALESCE(MAX(active), 0) FROM tabs WHERE window_key = ?`,
		primary).Scan(&spaceNext, &spaceActive); err != nil {
		return err
	}
	next, activeTaken = max(next, spaceNext), activeTaken || spaceActive
	for _, t := range orphans {
		active := t.active && !activeTaken
		activeTaken = activeTaken || active
		if _, err := tx.Exec(`UPDATE tabs SET window_key = ?, "order" = ?, active = ? WHERE id = ?`,
			primary, next, active, t.id); err != nil {
			return err
		}
		next++
	}
	return tx.Commit()
}

func windowKeys(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT key FROM windows ORDER BY "order" ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func orphanTabs(db *sql.DB, known map[string]bool) ([]studioTab, error) {
	rows, err := db.Query(`SELECT t.id, t.window_key, t."order", t.active FROM tabs t
		LEFT JOIN windows w ON w.key = t.window_key ORDER BY COALESCE(w."order", 0), t."order"`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []studioTab
	for rows.Next() {
		var t studioTab
		if err := rows.Scan(&t.id, &t.windowKey, &t.order, &t.active); err != nil {
			return nil, err
		}
		if !known[t.windowKey] {
			out = append(out, t)
		}
	}
	return out, rows.Err()
}

// importLayout copies panel sizes of the Studio-only modes; a row Space already holds wins.
func importLayout(o Options) error {
	for _, mode := range studioModes {
		rows, err := o.Studio.Query(`SELECT key, value FROM ui_layout WHERE key LIKE ?`, "modes."+mode+".%")
		if err != nil {
			return err
		}
		var kv [][2]string
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				_ = rows.Close()
				return err
			}
			kv = append(kv, [2]string{k, v})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		_ = rows.Close()
		for _, e := range kv {
			if _, err := o.Space.Exec(`INSERT OR IGNORE INTO ui_layout (key, value) VALUES (?, ?)`, e[0], e[1]); err != nil {
				return err
			}
		}
	}
	return nil
}

// settingRenames maps a Studio leaf to the Space leaf it feeds.
var settingRenames = map[string]string{
	"appearance.fontFamily":  "appearance.fontFamily",
	"appearance.fontSize":    "appearance.fontSize",
	"appearance.rowDensity":  "appearance.rowDensity",
	"appearance.wordWrap":    "appearance.wordWrap",
	"appearance.rowColoring": "appearance.rowColoring",
	"agent.provider":         "agent.provider",
	"agent.hermetic":         "agent.hermetic",
	"advanced.notifyPrompts": "advanced.notifyPrompts",
	"advanced.logLevel":      "advanced.gitLogLevel",
}

func importSettings(o Options) error {
	for from, to := range settingRenames {
		var v string
		switch err := o.Studio.QueryRow(`SELECT value FROM settings WHERE key = ?`, from).Scan(&v); {
		case errors.Is(err, sql.ErrNoRows):
			continue
		case err != nil:
			return err
		}
		if _, err := o.Space.Exec(`INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)`, to, v); err != nil {
			return err
		}
	}
	return nil
}

// mergePrompts copies Studio prompt overrides Space has not overridden. Records stay raw JSON so
// the llmprompts file format keeps one owner.
func mergePrompts(o Options) error {
	src, ok, err := readPrompts(filepath.Join(o.StudioHome, "llm-prompts.json"))
	if err != nil || !ok || len(src.Overrides) == 0 {
		return err
	}
	dstPath := filepath.Join(o.SpaceHome, "llm-prompts.json")
	dst, ok, err := readPrompts(dstPath)
	if err != nil || !ok {
		return err
	}
	added := false
	for id, rec := range src.Overrides {
		if _, ok := dst.Overrides[id]; !ok {
			dst.Overrides[id] = rec
			added = true
		}
	}
	if !added {
		return nil
	}
	dst.Revision++
	raw, err := json.MarshalIndent(dst, "", "  ")
	if err != nil {
		return err
	}
	return renameio.WriteFile(dstPath, raw, 0o600)
}

type promptsDoc struct {
	Version   int                        `json:"version"`
	Revision  uint64                     `json:"revision"`
	Overrides map[string]json.RawMessage `json:"overrides"`
}

// readPrompts reads one overrides file; an absent file is empty. ok is false for a corrupt or
// foreign-version file, which the merge leaves alone rather than overwrite.
func readPrompts(path string) (d promptsDoc, ok bool, err error) {
	d = promptsDoc{Version: 1, Overrides: map[string]json.RawMessage{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return d, true, nil
	}
	if err != nil {
		return d, false, err
	}
	var parsed promptsDoc
	if err := json.Unmarshal(b, &parsed); err != nil || parsed.Version != 1 {
		slog.Warn("appmerge: prompt overrides unreadable, skipped", "scope", "appmerge", "file", filepath.Base(path), "err", err)
		return d, false, nil
	}
	if parsed.Overrides == nil {
		parsed.Overrides = d.Overrides
	}
	return parsed, true, nil
}
