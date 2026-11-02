package configfs

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// DocsFile is the generated agent guide in every config folder. Never read as config.
const DocsFile = "AGENTS.md"

// DecodeFunc parses and validates one file. A non-nil Problem means the file is invalid.
type DecodeFunc[T any] func(file string, src []byte) (T, *Problem)

// Projection is a module's last good config in SQLite, written only by the load path.
type Projection interface {
	Snapshot() (map[string]string, error) // file -> last good JSON
	Record(file, valueJSON string, now int64) error
	Forget(file string) error
}

// Folder is one config folder under an app home: files of one extension, one item per file.
type Folder[T any] struct {
	Dir        string
	Ext        string
	Name       *regexp.Regexp // valid file names for Save and Get
	Decode     DecodeFunc[T]
	Projection Projection
	Now        func() time.Time

	mu     sync.Mutex        // serializes Save
	warnMu sync.Mutex        // guards warned
	warned map[string]string // file -> hash a warning was logged for
}

// Entry is one file's state. Value is the current content, or the last good one when Problem is set.
type Entry[T any] struct {
	File, Path, Hash string
	Value            *T
	Problem          *Problem
}

func (f *Folder[T]) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *Folder[T]) visible(name string) bool {
	return strings.HasSuffix(name, f.Ext) && !strings.HasPrefix(name, ".") && name != DocsFile
}

// List loads every config file, sorted by name. It projects valid files, keeps the last good value
// for invalid ones and forgets projected files that are gone. A missing dir is an empty list.
func (f *Folder[T]) List() []Entry[T] {
	entries, err := os.ReadDir(f.Dir)
	if err != nil && !os.IsNotExist(err) {
		slog.Warn("config: read dir", "scope", "config", "dir", f.Dir, "err", err)
		return []Entry[T]{}
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Type().IsRegular() && f.visible(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	lv := &lastGood{p: f.Projection}
	out := make([]Entry[T], 0, len(names))
	present := make(map[string]bool, len(names))
	for _, name := range names {
		present[name] = true
		out = append(out, f.entryFor(name, lv))
	}
	for file := range lv.get() {
		if !present[file] {
			if err := f.Projection.Forget(file); err != nil {
				slog.Warn("config: forget", "scope", "config", "file", file, "err", err)
			}
		}
	}
	return out
}

// Get loads one file; ok is false when it does not exist or the name is not a config file name.
func (f *Folder[T]) Get(file string) (Entry[T], bool) {
	if file != filepath.Base(file) || !f.visible(file) {
		return Entry[T]{}, false
	}
	if _, err := os.Stat(filepath.Join(f.Dir, file)); err != nil {
		return Entry[T]{}, false
	}
	return f.entryFor(file, &lastGood{p: f.Projection}), true
}

// Read returns a file's raw text and hash.
func (f *Folder[T]) Read(file string) ([]byte, string, error) {
	p, err := f.path(file)
	if err != nil {
		return nil, "", err
	}
	src, err := ReadCapped(p, MaxFileBytes)
	if err != nil {
		return nil, "", err
	}
	return src, TextHash(src), nil
}

// Save writes src as the file's content, valid or not: the user's text is never refused and the last
// good value stays in use while the file is broken. baseHash, when set, must match the file's
// current text, else ErrConflict.
func (f *Folder[T]) Save(file, baseHash string, src []byte) (Entry[T], error) {
	p, err := f.path(file)
	if err != nil {
		return Entry[T]{}, err
	}
	if len(src) > MaxFileBytes {
		return Entry[T]{}, fmt.Errorf("%w: %s", ErrInvalid, ErrTooLarge)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if baseHash != "" {
		cur, err := os.ReadFile(p)
		if err != nil && !os.IsNotExist(err) {
			return Entry[T]{}, fmt.Errorf("configfs: read %s: %w", file, err)
		}
		if TextHash(cur) != baseHash {
			return Entry[T]{}, fmt.Errorf("%w: the file changed since it was loaded", ErrConflict)
		}
	}
	if err := WriteAtomic(p, src, 0o644); err != nil {
		return Entry[T]{}, err
	}
	e, ok := f.Get(file)
	if !ok {
		return Entry[T]{}, fmt.Errorf("configfs: %s vanished after write", file)
	}
	return e, nil
}

func (f *Folder[T]) path(file string) (string, error) {
	if f.Name != nil && !f.Name.MatchString(file) || file != filepath.Base(file) || !f.visible(file) {
		return "", fmt.Errorf("%w: file name %q", ErrInvalid, file)
	}
	return filepath.Join(f.Dir, file), nil
}

// lastGood loads Projection.Snapshot once per read and keeps it current with the writes made through it.
type lastGood struct {
	p      Projection
	loaded bool
	m      map[string]string
}

func (l *lastGood) get() map[string]string {
	if !l.loaded {
		l.loaded = true
		m, err := l.p.Snapshot()
		if err != nil {
			slog.Warn("config: read last good", "scope", "config", "err", err)
			m = map[string]string{}
		}
		l.m = m
	}
	return l.m
}

func (f *Folder[T]) entryFor(file string, lg *lastGood) Entry[T] {
	entry := Entry[T]{File: file, Path: filepath.Join(f.Dir, file)}
	src, err := ReadCapped(entry.Path, MaxFileBytes)
	entry.Hash = TextHash(src)
	var v T
	var prob *Problem
	if err != nil {
		prob = &Problem{Message: i18n.T("backend.core.config.unreadable.body", map[string]string{"detail": err.Error()})}
	} else {
		v, prob = f.Decode(file, src)
	}
	if prob == nil {
		entry.Value = &v
		f.record(file, &v, lg)
		return entry
	}
	entry.Problem = prob
	f.warnOnce(file, entry.Hash, prob)
	if js, ok := lg.get()[file]; ok {
		var prev T
		if json.Unmarshal([]byte(js), &prev) == nil {
			entry.Value = &prev
		}
	}
	return entry
}

func (f *Folder[T]) warnOnce(file, hash string, p *Problem) {
	f.warnMu.Lock()
	defer f.warnMu.Unlock()
	if f.warned == nil {
		f.warned = map[string]string{}
	}
	if f.warned[file] == hash {
		return
	}
	f.warned[file] = hash
	slog.Warn("config: invalid file, keeping last good", "scope", "config", "file", file, "line", p.Line)
}

func (f *Folder[T]) record(file string, v *T, lg *lastGood) {
	js, err := json.Marshal(v)
	if err != nil {
		slog.Warn("config: encode", "scope", "config", "file", file, "err", err)
		return
	}
	last := lg.get()
	if last[file] == string(js) {
		return
	}
	if err := f.Projection.Record(file, string(js), f.now().UnixMilli()); err != nil {
		slog.Warn("config: record", "scope", "config", "file", file, "err", err)
		return
	}
	last[file] = string(js)
}
