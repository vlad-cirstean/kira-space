package scripts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/configfs"
)

// Folder is the runs module folder name under <home>/config.
const Folder = "runs"

// LoaderOptions builds a Loader.
type LoaderOptions struct {
	Repo *Repo
	// Dir is the runs folder, <home>/config/runs.
	Dir string
	// Home is the app data folder; a deleted run loses its automations folder under it.
	Home string
	App  string // "Space" or "Studio", AGENTS.md header
	// SpaceKeys allows use_ade_dir in a file.
	SpaceKeys bool
	// Emit broadcasts the snapshot when it changed.
	Emit func(Snapshot)
	// OnChange runs after a changed snapshot is broadcast (the scheduler reloads).
	OnChange func()
}

// Loader keeps scripts in step with the run files: files to the DB only. It is long-lived.
type Loader struct {
	o      LoaderOptions
	folder *configfs.Folder[File]
	docs   configfs.DocModel

	mu       sync.Mutex // serializes Reload and what it publishes
	problems []FileProblem
	last     string // JSON of the last published snapshot
	stop     func()

	createMu sync.Mutex
}

// NewLoader builds the loader; it creates nothing on disk.
func NewLoader(o LoaderOptions) (*Loader, error) {
	decode, docs, err := newDecoder(o.Repo.models(), o.SpaceKeys, o.App)
	if err != nil {
		return nil, err
	}
	l := &Loader{o: o, docs: docs}
	l.folder = &configfs.Folder[File]{
		Dir: o.Dir, Ext: FileExt, Name: regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,79}\.yaml$`),
		Decode: decode, Projection: o.Repo.Projection(),
	}
	return l, nil
}

// Dir is the runs folder.
func (l *Loader) Dir() string { return l.o.Dir }

// Start loads the folder, writes AGENTS.md and watches the folder. A failure of the docs or the
// watch is logged and never fatal.
func (l *Loader) Start() {
	l.Reload()
	if err := l.folder.WriteDocs(l.docs); err != nil {
		slog.Warn("runs: write AGENTS.md", "scope", "startup", "err", err)
	}
	stop, err := l.folder.Watch(l.Reload)
	if err != nil {
		slog.Warn("runs: watch folder", "scope", "startup", "err", err)
		return
	}
	l.mu.Lock()
	l.stop = stop
	l.mu.Unlock()
}

// Close stops the watch.
func (l *Loader) Close() {
	l.mu.Lock()
	stop := l.stop
	l.stop = nil
	l.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// Reload reads the folder into the DB and publishes the snapshot when it changed.
func (l *Loader) Reload() {
	l.mu.Lock()
	defer l.mu.Unlock()
	problems := []FileProblem{}
	for _, e := range l.folder.List() {
		if e.Problem == nil {
			continue
		}
		id, _ := StemOf(e.File)
		if _, ok := StemOf(e.File); !ok {
			id = ""
		}
		problems = append(problems, FileProblem{File: e.File, ID: id, Line: e.Problem.Line, Message: e.Problem.Message, LastGood: e.Value != nil})
	}
	if err := l.o.Repo.pruneCollections(); err != nil {
		slog.Warn("runs: prune collections", "scope", "config", "err", err)
	}
	l.problems = problems
	l.publishLocked(false)
}

// Broadcast publishes the snapshot after a state change (order, secrets, schedule target).
func (l *Loader) Broadcast() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.publishLocked(true)
}

func (l *Loader) publishLocked(force bool) {
	snap, err := l.snapshotLocked()
	if err != nil {
		slog.Warn("runs: snapshot", "scope", "config", "err", err)
		return
	}
	js, err := json.Marshal(snap)
	if err != nil {
		return
	}
	if !force && string(js) == l.last {
		return
	}
	l.last = string(js)
	if l.o.Emit != nil {
		l.o.Emit(snap)
	}
	if l.o.OnChange != nil {
		l.o.OnChange()
	}
}

// Has reports whether a run with this id is stored.
func (l *Loader) Has(id string) bool {
	rec, err := l.o.Repo.Get(id)
	return err == nil && rec != nil
}

// Snapshot is the whole panel state.
func (l *Loader) Snapshot() (Snapshot, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.snapshotLocked()
}

func (l *Loader) snapshotLocked() (Snapshot, error) {
	collections, err := l.o.Repo.ListCollections()
	if err != nil {
		return Snapshot{}, err
	}
	list, err := l.o.Repo.List()
	if err != nil {
		return Snapshot{}, err
	}
	problems := l.problems
	if problems == nil {
		problems = []FileProblem{}
	}
	return Snapshot{
		Dir: l.o.Dir, Collections: collections, Scripts: list, Problems: problems, SecretStorage: l.o.Repo.secretStatus(),
	}, nil
}

// Create validates fields, writes a new run file named after the run and loads it. SecretValues
// seal after the file lands; a Space schedule target goes to the state columns.
func (l *Loader) Create(fields CustomScriptFields, secretValues map[string]*string) (CustomScript, error) {
	if err := fields.Validate(l.o.Repo.models()); err != nil {
		return CustomScript{}, fmt.Errorf("scripts: %w", err)
	}
	if _, _, err := l.o.Repo.planSecrets(fields.Params, secretValues); err != nil {
		return CustomScript{}, err
	}
	var taskID, branchID string
	if fields.Schedule != nil {
		taskID, branchID = fields.Schedule.TaskID, fields.Schedule.BranchID
	}
	text, err := FileOf(fields).Render()
	if err != nil {
		return CustomScript{}, err
	}
	l.createMu.Lock()
	id := l.freeID(slug(fields.Name))
	_, err = l.folder.Save(FileName(id), "", text)
	l.createMu.Unlock()
	if err != nil {
		return CustomScript{}, fmt.Errorf("scripts: create %s: %w", id, err)
	}
	l.Reload()
	rec, err := l.o.Repo.Get(id)
	if err != nil {
		return CustomScript{}, err
	}
	if rec == nil {
		return CustomScript{}, fmt.Errorf("scripts: create %s: the new file was not loaded", id)
	}
	if len(secretValues) > 0 {
		if err := l.o.Repo.SetSecrets(id, secretValues); err != nil {
			return CustomScript{}, err
		}
	}
	if taskID != "" {
		if err := l.o.Repo.SetScheduleTarget(id, taskID, branchID); err != nil {
			return CustomScript{}, err
		}
	}
	l.Broadcast()
	return l.get(id)
}

func (l *Loader) get(id string) (CustomScript, error) {
	rec, err := l.o.Repo.Get(id)
	if err != nil {
		return CustomScript{}, err
	}
	if rec == nil {
		return CustomScript{}, fmt.Errorf("scripts: get %s: %w", id, sql.ErrNoRows)
	}
	return *rec, nil
}

// freeID returns base, or base-2, base-3 ... the first id without a file.
func (l *Loader) freeID(base string) string {
	id := base
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(l.o.Dir, FileName(id))); errors.Is(err, os.ErrNotExist) {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

// Delete removes the run file, its row, its saved secret values and its automations folder.
func (l *Loader) Delete(id string) error {
	if _, ok := StemOf(FileName(id)); !ok {
		return fmt.Errorf("scripts: delete %s: %w", id, sql.ErrNoRows)
	}
	rec, err := l.o.Repo.Get(id)
	if err != nil {
		return err
	}
	_, statErr := os.Stat(filepath.Join(l.o.Dir, FileName(id)))
	if rec == nil && statErr != nil {
		return fmt.Errorf("scripts: delete %s: %w", id, sql.ErrNoRows)
	}
	if err := os.Remove(filepath.Join(l.o.Dir, FileName(id))); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("scripts: delete %s: %w", id, err)
	}
	l.Reload()
	if err := l.o.Repo.Projection().Forget(FileName(id)); err != nil {
		return err
	}
	if l.o.Home != "" {
		RemoveDir(l.o.Home, id)
	}
	l.Reload()
	return nil
}

// FileView is the editor's state of one run file.
type FileView struct {
	File string `json:"file"`
	// ID is "" when the file name is not a valid id.
	ID       string           `json:"id"`
	Path     string           `json:"path"`
	Text     string           `json:"text"`
	Hash     string           `json:"hash"`
	Problem  *FileViewProblem `json:"problem"`
	LastGood bool             `json:"lastGood"`
	// Editable is false for a file that is not valid UTF-8 or has a bad name: open it in an editor.
	Editable bool `json:"editable"`
}

// FileViewProblem is why the file is invalid.
type FileViewProblem struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// View returns one file's text, hash and state. A missing file is sql.ErrNoRows.
func (l *Loader) View(file string) (FileView, error) {
	e, ok := l.folder.Get(file)
	if !ok {
		return FileView{}, fmt.Errorf("scripts: file %s: %w", file, sql.ErrNoRows)
	}
	v := FileView{File: file, Path: e.Path, Hash: e.Hash, Editable: true}
	id, validID := StemOf(file)
	if validID {
		v.ID = id
	}
	if e.Problem != nil {
		v.Problem = &FileViewProblem{Line: e.Problem.Line, Message: e.Problem.Message}
		v.LastGood = e.Value != nil
	}
	if !validID {
		v.Editable = false
		return v, nil
	}
	raw, hash, err := l.folder.Read(file)
	if err != nil || !utf8.Valid(raw) {
		v.Editable = false
		return v, nil
	}
	v.Text, v.Hash = string(raw), hash
	return v, nil
}

// ErrConflict marks a save built on a file text that changed since it was loaded.
var ErrConflict = configfs.ErrConflict

// SaveFile writes text as the file, valid or not: the user's text is never refused and the last good
// value keeps running while the file is invalid. baseHash must match the file's current text.
func (l *Loader) SaveFile(file, text, baseHash string) (FileView, error) {
	if _, ok := l.folder.Get(file); !ok {
		return FileView{}, fmt.Errorf("scripts: file %s: %w", file, sql.ErrNoRows)
	}
	if _, err := l.folder.Save(file, baseHash, []byte(text)); err != nil {
		return FileView{}, err
	}
	l.Reload()
	return l.View(file)
}
