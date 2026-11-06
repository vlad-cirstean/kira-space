package homefold

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/google/renameio/v2"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/fsmove"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// Files moves the legacy folder into a backup and lands the Studio database, config folders, tokens and
// prompt overrides in the Space home.
func Files(o Options) error {
	m, found, err := readMarker(o.Space)
	if err != nil {
		return err
	}
	switch {
	case found && (m.State == stateDone || m.State == stateFiles || m.State == stateRuns):
		return nil
	case !found:
		empty, err := fsmove.IsEmptyDir(o.Legacy)
		if err != nil {
			return fmt.Errorf("homefold: read %s: %w", o.Legacy, err)
		}
		if o.Legacy == "" || empty {
			return writeMarker(o.Space, marker{State: stateDone, Fresh: true})
		}
	}

	if fsmove.Exists(o.Legacy) {
		lock, ok, err := config.AcquireLock(filepath.Join(o.Legacy, "app.lock"))
		if err != nil {
			return fmt.Errorf("homefold: lock: %w", err)
		}
		if !ok {
			return ErrStudioRunning
		}
		defer func() { _ = lock.Close() }()
	}

	if !found {
		if m, err = startMove(o); err != nil {
			return err
		}
	}
	return landFiles(o, m)
}

// startMove picks the backup folder, parks dev leftovers and records the move.
func startMove(o Options) (marker, error) {
	m := marker{State: stateMoving, Legacy: o.Legacy,
		Backup: filepath.Join(o.SpaceHome, "backups", "p539-"+o.now().UTC().Format("20060102T150405Z"))}
	if err := os.MkdirAll(m.Backup, 0o700); err != nil {
		return m, fmt.Errorf("homefold: backup dir: %w", err)
	}
	if err := moveAside(o, m.Backup); err != nil {
		return m, err
	}
	return m, writeMarker(o.Space, m)
}

// landFiles runs the Files steps in order; each is safe to repeat.
func landFiles(o Options, m marker) error {
	src := filepath.Join(m.Backup, "kira-studio")
	steps := []struct {
		name string
		run  func() error
	}{
		{"move", func() error { return moveLegacy(o, m.Legacy, src) }},
		{"db", func() error { return landDB(o, src) }},
		{"dirs", func() error {
			for _, name := range []string{connfiles.Folder, apifiles.Folder} {
				if err := landDir(o, src, name); err != nil {
					return err
				}
			}
			return nil
		}},
		{"tokens", func() error {
			seen, err := landTokens(o, src)
			m.Reregister = m.Reregister || seen
			return err
		}},
		{"prompts", func() error {
			if err := foldPrompts(o, m.Backup, src); err != nil {
				return fmt.Errorf("homefold: prompts: %w", err)
			}
			logLeftovers(src)
			return nil
		}},
		{"files-done", func() error {
			m.State = stateFiles
			return writeMarker(o.Space, m)
		}},
	}
	for _, st := range steps {
		if err := o.step(st.name); err != nil {
			return err
		}
		if err := st.run(); err != nil {
			return err
		}
	}
	return nil
}

// moveAside parks dev leftovers the fold would overwrite; real installs have none.
func moveAside(o Options, backup string) error {
	for _, name := range []string{"studio.db", "studio.db-wal", "studio.db-shm", connfiles.Folder, apifiles.Folder} {
		p := parkedPath(o.SpaceHome, name)
		if !fsmove.Exists(p) {
			continue
		}
		if err := os.Rename(p, filepath.Join(backup, name+".pre-p539")); err != nil {
			return fmt.Errorf("homefold: move aside %s: %w", name, err)
		}
	}
	return nil
}

// moveLegacy renames the legacy folder to src, copying across devices. Safe to repeat after any crash.
func moveLegacy(o Options, legacy, src string) error {
	legacyOK, srcOK := fsmove.Exists(legacy), fsmove.Exists(src)
	switch {
	case legacyOK && srcOK:
		if err := fsmove.VerifyTree(legacy, src); err != nil {
			return err
		}
		return os.RemoveAll(legacy)
	case !legacyOK:
		return nil
	}
	err := o.rename(legacy, src)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return fmt.Errorf("homefold: move %s: %w", legacy, err)
	}
	tmp := src + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := fsmove.CopyTree(legacy, tmp); err != nil {
		return fmt.Errorf("homefold: copy %s: %w", legacy, err)
	}
	if err := fsmove.VerifyTree(legacy, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, src); err != nil {
		return err
	}
	return os.RemoveAll(legacy)
}

// landDB writes the Studio database to <space>/studio.db as a consistent snapshot of the backup copy.
func landDB(o Options, src string) error {
	from := filepath.Join(src, "kira.db")
	if !fsmove.Exists(from) {
		return nil
	}
	db, err := sqlitex.Open(from)
	if err != nil {
		return fmt.Errorf("homefold: open Studio database: %w", err)
	}
	dst, tmp := config.ModuleDbPathAt(o.SpaceHome), config.ModuleDbPathAt(o.SpaceHome)+".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		_ = db.Close()
		return err
	}
	_, err = db.Exec(`VACUUM INTO ?`, tmp)
	if cerr := db.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("homefold: snapshot Studio database: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.RemoveAll(dst + suffix); err != nil {
			return err
		}
	}
	return os.Rename(tmp, dst)
}

// runsDir is the Space runs folder.
func runsDir(home string) string { return configfs.ModuleDir(home, scripts.Folder) }

// parkedPath is where a Space item the fold would overwrite lives: a config folder under <home>/config,
// a database file at the home.
func parkedPath(home, name string) string {
	if name == connfiles.Folder || name == apifiles.Folder {
		return configfs.ModuleDir(home, name)
	}
	return filepath.Join(home, name)
}

func landDir(o Options, src, name string) error {
	from := filepath.Join(src, name)
	if !fsmove.Exists(from) {
		return nil
	}
	if err := os.MkdirAll(configfs.Root(o.SpaceHome), 0o700); err != nil {
		return err
	}
	dst := configfs.ModuleDir(o.SpaceHome, name)
	tmp := dst + ".p539tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := fsmove.CopyTree(from, tmp); err != nil {
		return fmt.Errorf("homefold: copy %s: %w", name, err)
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// landTokens copies the DB MCP bearer files; reports whether the registration token existed.
func landTokens(o Options, src string) (registered bool, err error) {
	for _, name := range []string{"mcp-db-token.json", "mcp-db-header.token"} {
		b, err := os.ReadFile(filepath.Join(src, name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		if err := renameio.WriteFile(filepath.Join(o.SpaceHome, name), b, 0o600); err != nil {
			return false, err
		}
		registered = registered || name == "mcp-db-token.json"
	}
	return registered, nil
}

var handled = map[string]bool{
	"kira.db": true, "connections": true, "api": true, "runs": true, "automations": true,
	"mcp-db-token.json": true, "mcp-db-header.token": true, "llm-prompts.json": true,
}

var transient = map[string]bool{
	"logs": true, "app.lock": true, "automations-mcp": true, "sandbox": true, "sandbox-cache": true,
	"mcp-header-helper.sh": true, "kira.db-wal": true, "kira.db-shm": true,
}

func logLeftovers(src string) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	for _, e := range entries {
		switch {
		case handled[e.Name()]:
		case transient[e.Name()]:
			slog.Info("homefold: left in backup", "scope", "homefold", "name", e.Name())
		default:
			slog.Warn("homefold: unknown, left in backup", "scope", "homefold", "name", e.Name())
		}
	}
}

// foldPrompts backs up the Space overrides file once, then adds Studio overrides Space lacks.
func foldPrompts(o Options, backup, src string) error {
	spacePath := filepath.Join(configfs.ModuleDir(o.SpaceHome, llmprompts.Folder), llmprompts.File)
	bak := filepath.Join(backup, "space-llm-prompts.json")
	if !fsmove.Exists(bak) && fsmove.Exists(spacePath) {
		b, err := os.ReadFile(spacePath)
		if err != nil {
			return err
		}
		if err := renameio.WriteFile(bak, b, 0o600); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(spacePath), 0o700); err != nil {
		return err
	}
	return mergePrompts(filepath.Join(src, llmprompts.File), spacePath)
}

// mergePrompts copies Studio prompt overrides Space has not overridden. Records stay raw JSON so
// the llmprompts file format keeps one owner.
func mergePrompts(srcPath, dstPath string) error {
	src, ok, err := readPrompts(srcPath)
	if err != nil || !ok || len(src.Overrides) == 0 {
		return err
	}
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
		slog.Warn("homefold: prompt overrides unreadable, skipped", "scope", "homefold", "file", filepath.Base(path), "err", err)
		return d, false, nil
	}
	if parsed.Overrides == nil {
		parsed.Overrides = d.Overrides
	}
	return parsed, true, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
