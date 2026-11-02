// Package agentctx is the app's one agent context: a Markdown file in the shared agent folder that
// every launched agent receives, rendered per provider. Kira Space hosts the folder and its editor;
// Kira Studio reads it for its own launches.
package agentctx

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/configfs"
)

// File is the context file name in the agent folder.
const File = "context.md"

// MaxBytes caps the context: it is added to every launch.
const MaxBytes = 32 << 10

// Folder is the config folder name, the key of its last good row.
const Folder = "agent"

//go:embed agents_intro.md
var agentsIntro string

// Options builds a Store.
type Options struct {
	Dir         string              // shared agent folder
	SnapshotDir string              // <own home>/agent-context
	Projection  configfs.Projection // last good text
	App         string              // "Space" or "Studio", AGENTS.md header
	// KeepsOnResume lists the providers whose resumed conversation keeps its recorded context.
	KeepsOnResume func() []string
}

// Store reads and writes the agent context file and renders it for launches.
type Store struct {
	o      Options
	folder *configfs.Folder[string]

	mu     sync.Mutex // snapshot writes and prune
	pruned bool
	saveMu sync.Mutex
}

// ViewProblem is why the file is invalid.
type ViewProblem struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// View is the editor's state of the file.
type View struct {
	Folder        string       `json:"folder"`
	File          string       `json:"file"`
	Path          string       `json:"path"`
	Exists        bool         `json:"exists"`
	Text          string       `json:"text"`
	Hash          string       `json:"hash"`
	Editable      bool         `json:"editable"`
	Problem       *ViewProblem `json:"problem"`
	LastGood      bool         `json:"lastGood"`
	MaxBytes      int          `json:"maxBytes"`
	KeepsOnResume []string     `json:"keepsOnResume"`
}

// New builds a Store; it creates nothing on disk.
func New(o Options) *Store {
	return &Store{o: o, folder: &configfs.Folder[string]{
		Dir:        o.Dir,
		Ext:        ".md",
		Name:       regexp.MustCompile(`^context\.md$`),
		Decode:     configfs.Text(MaxBytes),
		Projection: o.Projection,
	}}
}

// View returns the file's text, hash and state. It also records a valid file as last good.
func (s *Store) View() View {
	v := View{Folder: Folder, File: File, Path: s.path(), MaxBytes: MaxBytes, Editable: true, KeepsOnResume: []string{}}
	if s.o.KeepsOnResume != nil {
		if l := s.o.KeepsOnResume(); l != nil {
			v.KeepsOnResume = l
		}
	}
	e, ok := s.folder.Get(File)
	if !ok {
		return v
	}
	v.Exists = true
	v.Hash = e.Hash
	if e.Problem != nil {
		v.Problem = &ViewProblem{Line: e.Problem.Line, Message: e.Problem.Message}
		v.LastGood = e.Value != nil
	}
	raw, hash, err := s.folder.Read(File)
	if err != nil || !utf8.Valid(raw) {
		v.Editable = false
		return v
	}
	v.Text, v.Hash = string(raw), hash
	return v
}

func (s *Store) path() string { return filepath.Join(s.folder.Dir, File) }

// Save writes text as the file. baseHash is the hash the editor loaded (configfs.TextHash(nil), "",
// for no file): a file changed since then is configfs.ErrConflict, a concurrent create included.
// Invalid text is saved too; the last good text stays in use.
func (s *Store) Save(text, baseHash string) (View, error) {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	cur, err := os.ReadFile(s.path())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return View{}, fmt.Errorf("agentctx: read %s: %w", File, err)
	}
	if configfs.TextHash(cur) != baseHash {
		return View{}, fmt.Errorf("%w: the file changed since it was loaded", configfs.ErrConflict)
	}
	if _, err := s.folder.Save(File, "", []byte(text)); err != nil {
		return View{}, err
	}
	return s.View(), nil
}

// Watch calls onChange (debounced) when the folder's Markdown files change.
func (s *Store) Watch(onChange func()) (stop func(), err error) { return s.folder.Watch(onChange) }

// WriteDocs writes the folder's AGENTS.md.
func (s *Store) WriteDocs() error {
	return s.folder.WriteDocs(configfs.DocModel{
		App: s.o.App, Title: "Kira agent folder", Intro: agentsIntro,
		Pattern: File, Format: "Markdown", Singleton: true, MaxBytes: MaxBytes,
	})
}
