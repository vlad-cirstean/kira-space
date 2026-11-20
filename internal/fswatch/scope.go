package fswatch

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/text/unicode/norm"
)

const sep = string(filepath.Separator)

// clean is Clean plus NFC: the spelling every comparison and every reported path uses.
func clean(p string) string { return norm.NFC.String(filepath.Clean(p)) }

type rootSpec struct {
	caller string // the caller's spelling, clean
	key    string // symlink-resolved, clean: the matching spelling
	disk   string // symlink-resolved, Clean only: what the OS knows the dir as (NFC would break Linux)
}

type verdict struct {
	watched bool
	disk    string
}

// scope decides which dirs are watched and which events are delivered. Pure over add/remove, which
// the backend supplies; single-owner (the watcher goroutine), so no lock.
type scope struct {
	roots    []rootSpec
	maxDepth int
	skip     func(string) bool
	fold     bool // match root prefixes case-insensitively (APFS default volumes)
	decided  map[string]verdict
	add      func(disk string) error
	remove   func(disk string)
}

func newScope(o Options) (*scope, error) {
	if len(o.Roots) == 0 {
		return nil, errors.New("fswatch: no roots")
	}
	s := &scope{maxDepth: o.MaxDepth, skip: o.SkipDir, fold: runtime.GOOS == "darwin", decided: map[string]verdict{}}
	seen := map[string]bool{}
	for _, r := range o.Roots {
		fi, err := os.Stat(r)
		if err != nil {
			return nil, fmt.Errorf("fswatch: root %s: %w", r, err)
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("fswatch: root %s is not a directory", r)
		}
		disk := filepath.Clean(r)
		if res, err := filepath.EvalSymlinks(r); err == nil {
			disk = filepath.Clean(res)
		}
		key := clean(disk)
		if seen[key] {
			continue
		}
		seen[key] = true
		s.roots = append(s.roots, rootSpec{caller: clean(r), key: key, disk: disk})
	}
	return s, nil
}

// topRoots are the disk spellings of roots not nested in another root.
func (s *scope) topRoots() []string {
	var out []string
	for i, r := range s.roots {
		nested := false
		for j, o := range s.roots {
			if _, ok := under(r.key, o.key, false); ok && i != j {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, r.disk)
		}
	}
	return out
}

// under reports whether path equals root or sits below it, and the relative remainder.
func under(path, root string, fold bool) (rel string, ok bool) {
	if len(path) < len(root) {
		return "", false
	}
	head := path[:len(root)]
	if head != root && (!fold || !strings.EqualFold(head, root)) {
		return "", false
	}
	rest := path[len(root):]
	switch {
	case rest == "":
		return "", true
	case strings.HasSuffix(root, sep):
		return rest, true
	case rest[0] != filepath.Separator:
		return "", false
	}
	return rest[1:], true
}

// locate finds the deepest root holding key.
func (s *scope) locate(key string, fold bool) (idx int, rel string, ok bool) {
	idx = -1
	for i, r := range s.roots {
		if rest, in := under(key, r.key, fold); in && (idx < 0 || len(r.key) > len(s.roots[idx].key)) {
			idx, rel = i, rest
		}
	}
	return idx, rel, idx >= 0
}

func (s *scope) isRoot(key string) bool {
	for _, r := range s.roots {
		if r.key == key {
			return true
		}
	}
	return false
}

// watched decides dir once (see Watch) and adds it to the backend on a yes. key and disk spell the
// same dir; disk is what add receives.
func (s *scope) watched(key, disk string) bool {
	i, rel, ok := s.locate(key, false)
	if !ok {
		return false
	}
	if rel == "" {
		return true
	}
	if s.maxDepth >= 0 && strings.Count(rel, sep)+1 > s.maxDepth {
		return false
	}
	if v, ok := s.decided[key]; ok {
		return v.watched
	}
	yes := s.watched(filepath.Dir(key), filepath.Dir(disk))
	if yes && s.skip != nil && s.skip(filepath.Join(s.roots[i].caller, rel)) {
		yes = false
	}
	if yes {
		if err := s.add(disk); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("fswatch: watch directory", "scope", "fswatch", "dir", disk, "err", err)
		}
	}
	s.decided[key] = verdict{watched: yes, disk: disk}
	return yes
}

// watchTree decides dir and, if watched, every dir below it that qualifies.
func (s *scope) watchTree(key, disk string) {
	if !s.watched(key, disk) {
		return
	}
	entries, err := os.ReadDir(disk)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() { // false for a symlink: links are never followed
			continue
		}
		ck := filepath.Join(key, norm.NFC.String(e.Name()))
		if !s.isRoot(ck) { // a nested root walks itself
			s.watchTree(ck, filepath.Join(disk, e.Name()))
		}
	}
}

// walkAll watches every root and the qualifying tree below it.
func (s *scope) walkAll() error {
	for _, r := range s.roots {
		if err := s.add(r.disk); err != nil {
			return fmt.Errorf("fswatch: watch %s: %w", r.caller, err)
		}
		s.watchTree(r.key, r.disk)
	}
	return nil
}

// rescan forgets every verdict and walks again; fsnotify re-adds what went missing.
func (s *scope) rescan() {
	clear(s.decided)
	if err := s.walkAll(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("fswatch: rescan", "scope", "fswatch", "err", err)
	}
}

// forget drops the verdict of key and every dir below it, and the backend watches that went with them.
func (s *scope) forget(key string) {
	if _, had := s.decided[key]; !had && !s.isRoot(key) {
		return
	}
	pre := key + sep
	for k, v := range s.decided {
		if k == key || strings.HasPrefix(k, pre) {
			delete(s.decided, k)
			if v.watched {
				s.remove(v.disk)
			}
		}
	}
}

// admit applies scope to one backend event: re-roots it, keeps verdicts current and filters.
func (s *scope) admit(r rawEvent) (Event, bool) {
	disk := filepath.Clean(r.path)
	key := clean(r.path)
	i, rel, ok := s.locate(key, false)
	if !ok && s.fold {
		i, rel, ok = s.locate(key, true)
	}
	if !ok {
		return Event{}, false
	}
	rt := s.roots[i]
	caller := rt.caller
	if rel != "" {
		key, caller = filepath.Join(rt.key, rel), filepath.Join(rt.caller, rel)
	} else {
		key = rt.key
	}
	if r.op&(Create|Remove|Rename) != 0 {
		s.forget(key) // a stale verdict must not outlive a dir that was replaced
	}
	if rel != "" {
		if !s.watched(filepath.Dir(key), filepath.Dir(disk)) {
			return Event{}, false
		}
		if r.dir && r.op&(Create|Rename) != 0 {
			s.watchTree(key, disk)
		}
	}
	return Event{Path: caller, Op: r.op, Dir: r.dir}, true
}
