// Package leftovers is a one-time boot cleanup that deletes what v0.3.1 and earlier left behind and
// the current code never reads (P557). It deletes only by the allowlist below: exact names, three fixed
// patterns and the retired Studio and memory homes. Logs carry names and counts, never contents. The next chapter
// deletes this package and its call in appwire.RunOneTimeMoves.
package leftovers

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/internal/filelock"
)

// firstKeptBackup is the phase whose backup folder (backups/p557-*) must survive.
const firstKeptBackup = 557

// Files the current code never reads, relative to the Space home.
var homeFiles = []string{
	"studio.db", "studio.db-wal", "studio.db-shm", "studio.db.tmp", // P539/P544 Studio database
	"git.sock", "git.sock.lock", // pre-P243
	"llm-prompts.json",               // P540 moved it to config/prompts
	"mobile/ca.key", "mobile/ca.crt", // pre-P223 local CA
}

// Folders removed whole, relative to the Space home: root copies P540 moved under config/, and P539
// partial copies.
var homeDirs = []string{
	"connections", "api", "runs", "workflows", "steps", "repos", "agent",
	"config/connections.p539tmp", "config/api.p539tmp",
}

// mobileTmp is the one glob: temp files of the pre-P223 CA writer.
const mobileTmp = "mobile/*.tmp"

var backupName = regexp.MustCompile(`^p(\d{3})-\d{8}T\d{6}Z$`)

// Options names the locations. An empty StudioHome or MemoryHome skips that home.
type Options struct {
	SpaceHome  string
	StudioHome string
	MemoryHome string
}

// StudioHomeDefault is ~/.kira-studio, or "" when a dev or test override is set.
func StudioHomeDefault() string {
	return retiredHomeDefault(".kira-studio", "KIRA_HOME")
}

// MemoryHomeDefault is ~/.kira-memory, the home before memory moved under the Space home, or "" when
// a dev or test override is set.
func MemoryHomeDefault() string {
	return retiredHomeDefault(".kira-memory", "KIRA_MEMORY_HOME")
}

func retiredHomeDefault(name, overrideEnv string) string {
	if os.Getenv("KIRA_SPACE_HOME") != "" || os.Getenv(overrideEnv) != "" {
		return ""
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, name)
}

// Run deletes the allowlisted items that exist. A failed item is logged and skipped.
func Run(o Options) {
	c := cleaner{}
	if home, err := filepath.EvalSymlinks(o.SpaceHome); err == nil {
		c.home = home
		c.spaceHome(home)
		c.retiredHome(o.StudioHome, ".kira-studio", "app.lock")
		c.retiredHome(o.MemoryHome, ".kira-memory", "import-runner.lock")
	} else if !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("leftovers: resolve the Space home", "scope", "startup", "err", err)
	}
	if len(c.removed) > 0 {
		slog.Info("leftovers removed", "scope", "startup", "count", len(c.removed), "names", strings.Join(c.removed, ","))
	}
}

type cleaner struct {
	home    string
	removed []string
}

func (c *cleaner) spaceHome(home string) {
	for _, rel := range homeFiles {
		c.remove(home, rel, false)
	}
	if tmps, err := filepath.Glob(filepath.Join(home, filepath.FromSlash(mobileTmp))); err == nil {
		for _, p := range tmps {
			if rel, err := filepath.Rel(home, p); err == nil {
				c.remove(home, filepath.ToSlash(rel), false)
			}
		}
	}
	if dir, ok := c.safe(home, "mobile"); ok {
		if fi, err := os.Lstat(dir); err == nil && fi.IsDir() {
			_ = os.Remove(dir) // only succeeds when empty
		}
	}
	for _, rel := range homeDirs {
		c.remove(home, rel, true)
	}
	c.backups(home)
}

// backups removes the pre-P557 backup folders of earlier chapters.
func (c *cleaner) backups(home string) {
	dir, ok := c.safe(home, "backups")
	if !ok {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("leftovers: read backups", "scope", "startup", "err", err)
		}
		return
	}
	for _, e := range entries {
		m := backupName.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		if n, _ := strconv.Atoi(m[1]); n < firstKeptBackup {
			c.remove(home, "backups/"+e.Name(), true)
		}
	}
}

// safe returns the path of rel under home when its parent resolves inside home.
func (c *cleaner) safe(home, rel string) (string, bool) {
	p := filepath.Join(home, filepath.FromSlash(rel))
	parent, err := filepath.EvalSymlinks(filepath.Dir(p))
	if err != nil {
		return "", false
	}
	if !within(parent, home) {
		slog.Warn("leftovers: refused, a parent folder resolves outside the Space home", "scope", "startup", "name", rel)
		return "", false
	}
	return filepath.Join(parent, filepath.Base(p)), true
}

// remove deletes one allowlisted item of the expected kind. A symlink, or an item of the other kind, is
// refused.
func (c *cleaner) remove(home, rel string, dir bool) {
	p, ok := c.safe(home, rel)
	if !ok {
		return
	}
	fi, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		slog.Warn("leftovers: inspect failed", "scope", "startup", "name", rel, "err", err)
		return
	}
	if fi.Mode()&fs.ModeSymlink != 0 || fi.IsDir() != dir {
		slog.Warn("leftovers: refused, not the expected kind", "scope", "startup", "name", rel)
		return
	}
	if dir {
		err = os.RemoveAll(p)
	} else {
		err = os.Remove(p)
	}
	if err != nil {
		slog.Warn("leftovers: remove failed", "scope", "startup", "name", rel, "err", err)
		return
	}
	c.removed = append(c.removed, rel)
}

// retiredHome removes a retired home folder when it is a real folder named name that nothing running
// holds through lock. Logs carry the name and the entry count, never contents.
func (c *cleaner) retiredHome(path, name, lock string) {
	if path == "" {
		return
	}
	if filepath.Base(path) != name {
		slog.Warn("leftovers: refused, wrong folder name", "scope", "startup", "name", name)
		return
	}
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		slog.Warn("leftovers: inspect failed", "scope", "startup", "name", name, "err", err)
		return
	}
	if fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir() {
		slog.Warn("leftovers: refused, not a real folder", "scope", "startup", "name", name)
		return
	}
	if within(path, c.home) || within(c.home, path) {
		slog.Warn("leftovers: refused, overlaps the Space home", "scope", "startup", "name", name)
		return
	}
	if inUse(filepath.Join(path, lock)) {
		slog.Warn("leftovers: skipped, in use", "scope", "startup", "name", name)
		return
	}
	entries, _ := os.ReadDir(path)
	if err := os.RemoveAll(path); err != nil {
		slog.Warn("leftovers: remove failed", "scope", "startup", "name", name, "err", err)
		return
	}
	c.removed = append(c.removed, fmt.Sprintf("%s (%d entries)", name, len(entries)))
}

// inUse reports whether the lock file is held, or cannot be probed.
func inUse(lock string) bool {
	if _, err := os.Lstat(lock); errors.Is(err, fs.ErrNotExist) {
		return false
	}
	f, ok, err := filelock.TryLock(lock)
	if err != nil || !ok {
		return true
	}
	_ = f.Close()
	return false
}

// within reports whether child equals or sits inside parent.
func within(child, parent string) bool {
	rel, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
