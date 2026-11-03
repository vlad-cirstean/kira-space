package scripts

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// automationsDir is the folder under the app home that holds one folder per script.
const automationsDir = "automations"

// Dir is where a script runs. Blocker, when set, says why it cannot run there.
type Dir struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	// Base is the app home for DirModeKira; "" for DirModeFixed.
	Base    string `json:"base"`
	Blocker string `json:"blocker"`
	// Branch labels a DirModeWorktree folder as "repo · branch"; Pending says the worktree is created on Run.
	Branch  string `json:"branch"`
	Pending bool   `json:"pending"`
}

// ResolveDir reports where s runs, without creating anything. appHome is the app's data folder.
func ResolveDir(s CustomScript, appHome string) Dir {
	if s.DirMode == DirModeFixed {
		d := Dir{Path: s.WorkingDir, Mode: DirModeFixed}
		if info, err := os.Stat(s.WorkingDir); err != nil || !info.IsDir() {
			d.Blocker = i18n.T("backend.core.scriptDir.missing.body", map[string]string{"path": s.WorkingDir})
		}
		return d
	}
	return Dir{Path: filepath.Join(appHome, automationsDir, s.ID), Mode: DirModeKira, Base: appHome}
}

// PrepareDir creates a DirModeKira folder (0700) and refuses a symlink or non-directory at any level.
// Other modes only report their blocker. A returned error text is user-facing.
func PrepareDir(d Dir) error {
	if d.Blocker != "" {
		return fmt.Errorf("%s", d.Blocker)
	}
	if d.Mode != DirModeKira {
		return nil
	}
	for _, p := range []string{filepath.Dir(d.Path), d.Path} {
		if err := ensureRealDir(p); err != nil {
			return err
		}
	}
	return nil
}

func ensureRealDir(p string) error {
	info, err := os.Lstat(p)
	if os.IsNotExist(err) {
		if err := os.Mkdir(p, 0o700); err != nil {
			return fmt.Errorf("%s could not be created: %w", p, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s could not be read: %w", p, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a real folder: remove it", p)
	}
	return nil
}

// RemoveDir deletes a script's DirModeKira folder, best effort, only when it is a real directory.
func RemoveDir(appHome, id string) {
	if id == "" || id != filepath.Base(id) {
		return
	}
	p := filepath.Join(appHome, automationsDir, id)
	if info, err := os.Lstat(p); err != nil || !info.IsDir() {
		return
	}
	_ = os.RemoveAll(p)
}
