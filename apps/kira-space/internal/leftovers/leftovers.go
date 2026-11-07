// Package leftovers is a one-time boot cleanup that deletes what v0.3.1 and earlier left behind and
// the current code never reads (P557). It deletes only by the allowlist below: exact names, three fixed
// patterns and the retired Studio home. Logs carry names and counts, never contents. The next chapter
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

// Options names the locations. StudioHome "" skips the Studio home.
type Options struct {
	SpaceHome  string
	StudioHome string
}

// StudioHomeDefault is ~/.kira-studio, or "" when a dev or test override is set.
func StudioHomeDefault() string {
	if os.Getenv("KIRA_SPACE_HOME") != "" || os.Getenv("KIRA_HOME") != "" {
		return ""
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, ".kira-studio")
}

// Run deletes the allowlisted items that exist. A failed item is logged and skipped.
func Run(o Options) {
	c := cleaner{}
	if home, err := filepath.EvalSymlinks(o.SpaceHome); err == nil {
		c.home = home
		c.spaceHome(home)
		c.studioHome(o.StudioHome)
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

// studioHome removes the retired Studio home when it is a real folder that no running Studio holds.
func (c *cleaner) studioHome(studio string) {
	if studio == "" {
		return
	}
	if filepath.Base(studio) != ".kira-studio" {
		slog.Warn("leftovers: refused, not a .kira-studio folder", "scope", "startup")
		return
	}
	fi, err := os.Lstat(studio)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		slog.Warn("leftovers: inspect the Studio home failed", "scope", "startup", "err", err)
		return
	}
	if fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir() {
		slog.Warn("leftovers: refused, the Studio home is not a real folder", "scope", "startup")
		return
	}
	if within(studio, c.home) || within(c.home, studio) {
		slog.Warn("leftovers: refused, the Studio home overlaps the Space home", "scope", "startup")
		return
	}
	if inUse(filepath.Join(studio, "app.lock")) {
		slog.Warn("leftovers: skipped, Kira Studio is running", "scope", "startup")
		return
	}
	entries, _ := os.ReadDir(studio)
	if err := os.RemoveAll(studio); err != nil {
		slog.Warn("leftovers: remove the Studio home failed", "scope", "startup", "err", err)
		return
	}
	c.removed = append(c.removed, fmt.Sprintf(".kira-studio (%d entries)", len(entries)))
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
