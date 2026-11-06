package llmprompts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/google/renameio/v2"
)

const fileVersion = 1

const (
	// Folder is the prompts module folder name under <home>/config.
	Folder = "prompts"
	// File is the user prompt overrides file inside Folder.
	File = "llm-prompts.json"
)

// Hash is the content hash clients echo back as a change's baseHash.
func Hash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Change sets one prompt's text. Text equal to the default removes the override.
type Change struct {
	ID ID `json:"id"`
	// Text is the new text.
	Text string `json:"text"`
	// BaseHash is the hash of the text the client edited from (State.Text).
	BaseHash string `json:"baseHash"`
}

type record struct {
	Text        string `json:"text"`
	DefaultHash string `json:"defaultHash"`
	UpdatedAt   int64  `json:"updatedAt"`
}

type document struct {
	Version   int            `json:"version"`
	Revision  uint64         `json:"revision"`
	Overrides map[ID]*record `json:"overrides"`
}

type stamp struct {
	mod  int64
	size int64
	ok   bool
}

// State is one entry as the store sees it.
type State struct {
	// Text is what an editor starts from: the stored override, valid or not, else the default.
	Text string
	// Overridden: an override is stored.
	Overridden bool
	// Invalid is why a stored override is not used, "" when it is used.
	Invalid string
	// DefaultChanged: the default differs from the one the override was made against.
	DefaultChanged bool
	UpdatedAt      int64
}

// Snapshot is the store's state at one moment.
type Snapshot struct {
	States   map[ID]State
	Error    string
	Revision uint64
}

// ErrConflict means a change was made from a text that is no longer current.
var ErrConflict = errors.New("llmprompts: prompt changed since it was read")

// ProblemsError carries the findings that refused a save.
type ProblemsError struct{ Problems []Problem }

func (e *ProblemsError) Error() string {
	return fmt.Sprintf("llmprompts: %d invalid prompt change(s)", len(e.Problems))
}

// Store is a Source over one app's llm-prompts.json. It re-reads the file whenever its mtime or
// size changes, so another process's edit reaches the next call. A corrupt file or an override
// that no longer validates never breaks a call: the default is used.
type Store struct {
	path string
	// Now stamps updatedAt; tests replace it.
	Now func() time.Time

	mu      sync.Mutex
	loaded  bool
	stamp   stamp
	doc     document
	corrupt string
	states  map[ID]State
	valid   map[ID]string
}

// NewStore returns a Store over path. The file need not exist.
func NewStore(path string) *Store { return &Store{path: path, Now: time.Now} }

// Resolve implements Source.
func (s *Store) Resolve(id ID) Resolved {
	e := mustEntry(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	if t, ok := s.valid[id]; ok {
		return Resolved{Text: t, Overridden: true}
	}
	return Resolved{Text: e.Default}
}

// Revision implements Source: the file's revision, 0 when it is absent or unreadable.
func (s *Store) Revision() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	return s.doc.Revision
}

// Snapshot returns the state of every entry.
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	return s.snapshot()
}

func (s *Store) snapshot() Snapshot {
	return Snapshot{States: maps.Clone(s.states), Error: s.corrupt, Revision: s.doc.Revision}
}

func statOf(path string) (stamp, error) {
	fi, err := os.Stat(path)
	switch {
	case err == nil:
		return stamp{mod: fi.ModTime().UnixNano(), size: fi.Size(), ok: true}, nil
	case errors.Is(err, fs.ErrNotExist):
		return stamp{}, nil
	}
	return stamp{}, err
}

// refresh reloads the file when it changed. s.mu is held.
func (s *Store) refresh() {
	st, err := statOf(s.path)
	if err != nil {
		slog.Warn("llmprompts: stat overrides", "scope", "llmprompts", "err", err)
		return
	}
	if s.loaded && st == s.stamp {
		return
	}
	s.loaded, s.stamp = true, st
	s.doc, s.corrupt = document{Version: fileVersion, Overrides: map[ID]*record{}}, ""
	if st.ok {
		if err := s.read(); err != nil {
			s.doc = document{Version: fileVersion, Overrides: map[ID]*record{}}
			s.corrupt = err.Error()
			slog.Warn("llmprompts: overrides unreadable, using defaults", "scope", "llmprompts", "err", err)
		}
	}
	s.rebuild()
}

func (s *Store) read() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var d document
	if err := json.Unmarshal(b, &d); err != nil {
		return err
	}
	if d.Version != fileVersion {
		return fmt.Errorf("unsupported version %d", d.Version)
	}
	if d.Overrides == nil {
		d.Overrides = map[ID]*record{}
	}
	for id, r := range d.Overrides {
		if r == nil {
			delete(d.Overrides, id)
		}
	}
	s.doc = d
	return nil
}

// rebuild recomputes what every override resolves to. s.mu is held.
func (s *Store) rebuild() {
	s.states = make(map[ID]State, len(registry))
	s.valid = map[ID]string{}
	invalid := map[ID]string{}
	ids := make([]ID, 0, len(s.doc.Overrides))
	for id := range s.doc.Overrides {
		if _, ok := byID[id]; ok {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		r := s.doc.Overrides[id]
		if ps := validateText(id, r.Text, defaultText); len(ps) > 0 {
			invalid[id] = ps[0].Message
			continue
		}
		s.valid[id] = r.Text
	}
	// An override that passes alone can still break an entry that includes it.
	for len(s.valid) > 0 {
		ps := validateChanges(s.valid, defaultText)
		if len(ps) == 0 {
			break
		}
		for _, p := range ps {
			if _, ok := s.valid[p.ID]; ok {
				invalid[p.ID] = p.Message
				delete(s.valid, p.ID)
			}
		}
	}
	for _, e := range registry {
		st := State{Text: e.Default}
		if r, ok := s.doc.Overrides[e.ID]; ok {
			st = State{
				Text: r.Text, Overridden: true, Invalid: invalid[e.ID],
				DefaultChanged: r.DefaultHash != Hash(e.Default), UpdatedAt: r.UpdatedAt,
			}
		}
		s.states[e.ID] = st
	}
}

func (s *Store) effective(id ID) string {
	if t, ok := s.valid[id]; ok {
		return t
	}
	return defaultText(id)
}

// Validate returns the findings for text as the text of id, against the effective text of the rest.
func (s *Store) Validate(id ID, text string) []Problem {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	return validateText(id, text, s.effective)
}

// Texts returns the effective text lookup for previews.
func (s *Store) texts() textFunc {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	valid := maps.Clone(s.valid)
	return func(id ID) string {
		if t, ok := valid[id]; ok {
			return t
		}
		return defaultText(id)
	}
}

// Save applies changes all or nothing. It returns ErrConflict for a stale baseHash and a
// *ProblemsError when any change is invalid. It returns the snapshot after the save.
func (s *Store) Save(changes []Change) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	changed, seen := map[ID]string{}, map[ID]bool{}
	var problems []Problem
	for _, c := range changes {
		switch {
		case byID[c.ID] == nil:
			problems = append(problems, problem(c.ID, "backend.llmPrompts.unknown.body", map[string]string{"name": string(c.ID)}))
			continue
		case seen[c.ID]:
			problems = append(problems, problem(c.ID, "backend.llmPrompts.duplicate.body", nil))
			continue
		}
		seen[c.ID] = true
		if Hash(s.states[c.ID].Text) != c.BaseHash {
			return Snapshot{}, ErrConflict
		}
		changed[c.ID] = c.Text
	}
	if len(problems) == 0 {
		problems = validateChanges(changed, s.effective)
	}
	if len(problems) > 0 {
		return Snapshot{}, &ProblemsError{Problems: problems}
	}
	next := make(map[ID]*record, len(s.doc.Overrides))
	maps.Copy(next, s.doc.Overrides)
	dirty := false
	for id, text := range changed {
		e := byID[id]
		cur, had := next[id]
		switch {
		case text == e.Default && had:
			delete(next, id)
			dirty = true
		case text != e.Default && (!had || cur.Text != text):
			next[id] = &record{Text: text, DefaultHash: Hash(e.Default), UpdatedAt: s.Now().UnixMilli()}
			dirty = true
		}
	}
	if !dirty {
		return s.snapshot(), nil
	}
	doc := document{Version: fileVersion, Revision: s.doc.Revision + 1, Overrides: next}
	if err := s.write(doc); err != nil {
		return Snapshot{}, err
	}
	return s.snapshot(), nil
}

func (s *Store) write(doc document) error {
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	if err := renameio.WriteFile(s.path, b, 0o600); err != nil {
		return err
	}
	s.doc, s.corrupt = doc, ""
	if st, err := statOf(s.path); err == nil {
		s.loaded, s.stamp = true, st
	} else {
		s.loaded = false
	}
	s.rebuild()
	return nil
}
