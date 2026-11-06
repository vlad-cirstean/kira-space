package connfiles

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// Header is the comment that opens a file Kira writes.
const Header = "Kira connection. Options: AGENTS.md in this folder."

// Changes is what a load or a write made stale.
type Changes struct {
	Items []Change
	// Config is set when a file's problem set changed.
	Config bool
}

// Empty reports whether nothing changed.
func (c Changes) Empty() bool { return len(c.Items) == 0 && !c.Config }

// Options builds a Loader.
type Options struct {
	DB *sql.DB
	// Dir is the connections folder, <home>/connections.
	Dir     string
	Secrets *Secrets
	// OnChange receives what a load or a write made stale. It runs outside the loader's lock.
	OnChange func(Changes)
	// OnForget runs for a connection whose file is gone, before its row is deleted.
	OnForget func(id string)
	// NewID mints a row id; a uuid when nil.
	NewID func() string
}

func (o Options) newID() string {
	if o.NewID != nil {
		return o.NewID()
	}
	return ids.NewID()
}

// Loader keeps the connections table in step with the files: files to the DB only. It is long-lived.
type Loader struct {
	o      Options
	dec    decoder
	folder *configfs.Folder[File]

	mu       sync.Mutex // serializes loads and writes, and guards the fields below
	pend     []Change
	cfg      bool
	problems map[string]string
	stop     func()
}

// New builds the loader; it creates nothing on disk.
func New(o Options) (*Loader, error) {
	dec, err := newDecoder()
	if err != nil {
		return nil, err
	}
	l := &Loader{o: o, dec: dec, problems: map[string]string{}}
	l.folder = &configfs.Folder[File]{Dir: o.Dir, Ext: Ext, Name: fileNameRe, Decode: dec.decode, Projection: &projection{l: l}}
	return l, nil
}

// Dir is the connections folder.
func (l *Loader) Dir() string { return l.o.Dir }

// Dir is the connections folder under an app home.
func Dir(home string) string { return filepath.Join(home, "connections") }

// Start migrates the legacy rows once, loads the folder, writes AGENTS.md and watches the folder. A
// failed migration skips the rest: an unfinished export never projects. A failure of the docs or the
// watch is logged and never fatal.
func (l *Loader) Start() {
	if err := configfs.MigrateOnce(l.o.DB, "connections", l.migrate); err != nil {
		slog.Warn("connections: migrate to files", "scope", "startup", "err", err)
		return
	}
	l.Reload()
	model, err := DocModel()
	if err == nil {
		err = l.folder.WriteDocs(model)
	}
	if err != nil {
		slog.Warn("connections: write AGENTS.md", "scope", "startup", "err", err)
	}
	stop, err := configfs.WatchDir(l.o.Dir, Ext, l.Reload)
	if err != nil {
		slog.Warn("connections: watch folder", "scope", "startup", "err", err)
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

// Reload reads the folder into the DB and reports what changed.
func (l *Loader) Reload() {
	l.mu.Lock()
	l.noteProblems(l.folder.List())
	l.mu.Unlock()
	l.flush()
}

// flush hands the pending changes to OnChange, outside the lock.
func (l *Loader) flush() {
	l.mu.Lock()
	ch := Changes{Items: l.pend, Config: l.cfg}
	l.pend, l.cfg = nil, false
	l.mu.Unlock()
	if !ch.Empty() && l.o.OnChange != nil {
		l.o.OnChange(ch)
	}
}

func problemSig(p *configfs.Problem) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%d:%s", p.Line, p.Message)
}

// noteProblems records each file's problem and flags a Config change when the set differs.
func (l *Loader) noteProblems(entries []configfs.Entry[File]) {
	next := map[string]string{}
	for _, e := range entries {
		if e.Problem != nil {
			next[e.File] = problemSig(e.Problem)
		}
	}
	if len(next) != len(l.problems) {
		l.cfg = true
	} else {
		for k, v := range next {
			if l.problems[k] != v {
				l.cfg = true
				break
			}
		}
	}
	l.problems = next
}

func (l *Loader) noteProblem(file string, p *configfs.Problem) {
	sig := problemSig(p)
	if l.problems[file] != sig {
		l.cfg = true
	}
	if sig == "" {
		delete(l.problems, file)
		return
	}
	l.problems[file] = sig
}

// ---- reads ----

// Record is a connection as its file last said it.
type Record struct {
	ID, Stem string
	File     File
}

// RecordOf returns the last valid file content of connection id.
func (l *Loader) RecordOf(id string) (Record, bool) {
	var stem sql.NullString
	var js string
	err := l.o.DB.QueryRow(`SELECT stem, config_json FROM connections WHERE id = ?`, id).Scan(&stem, &js)
	if err != nil || !stem.Valid || js == "" {
		return Record{}, false
	}
	r := Record{ID: id, Stem: stem.String}
	if json.Unmarshal([]byte(js), &r.File) != nil {
		return Record{}, false
	}
	return r, true
}

// ProblemView is why a file is invalid.
type ProblemView struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

func problemView(p *configfs.Problem) *ProblemView {
	if p == nil {
		return nil
	}
	return &ProblemView{Line: p.Line, Message: p.Message}
}

// FileView is one connection file as the list shows it.
type FileView struct {
	File     string       `json:"file"`
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Problem  *ProblemView `json:"problem"`
	LastGood bool         `json:"lastGood"`
}

// SecretView is one secret: saved, referenced by a file, or both. Never the value.
type SecretView struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Set        bool   `json:"set"`
	Referenced bool   `json:"referenced"`
}

// View is everything the connection file panels show.
type View struct {
	Dir     string         `json:"dir"`
	Files   []FileView     `json:"files"`
	Secrets []SecretView   `json:"secrets"`
	Storage secrets.Status `json:"storage"`
}

func (l *Loader) idsByStem() (map[string]string, error) {
	rows, err := l.o.DB.Query(`SELECT stem, id FROM connections WHERE stem IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("connfiles: list ids: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var stem, id string
		if err := rows.Scan(&stem, &id); err != nil {
			return nil, err
		}
		out[stem] = id
	}
	return out, rows.Err()
}

// View loads the folder and answers every file's state and the secret names.
func (l *Loader) View() (View, error) {
	l.mu.Lock()
	entries := l.folder.List()
	l.noteProblems(entries)
	l.mu.Unlock()
	l.flush()

	ids, err := l.idsByStem()
	if err != nil {
		return View{}, err
	}
	v := View{Dir: l.o.Dir, Files: []FileView{}, Secrets: []SecretView{}}
	for _, e := range entries {
		fv := FileView{File: e.File, LastGood: e.Problem != nil && e.Value != nil, Problem: problemView(e.Problem)}
		if stem, ok := StemOf(e.File); ok {
			fv.ID = ids[stem]
		}
		if e.Value != nil {
			fv.Name = e.Value.Name
		}
		v.Files = append(v.Files, fv)
	}
	if v.Secrets, err = l.secretViews(nil); err != nil {
		return View{}, err
	}
	v.Storage = l.o.Secrets.Status()
	return v, nil
}

// secretViews lists the saved and the referenced secrets of both kinds. only, when set, limits the
// list to those (kind, name) pairs.
func (l *Loader) secretViews(only map[[2]string]bool) ([]SecretView, error) {
	referenced, err := l.referenced()
	if err != nil {
		return nil, err
	}
	out := []SecretView{}
	for _, kind := range Kinds {
		saved, err := l.o.Secrets.Names(kind)
		if err != nil {
			return nil, fmt.Errorf("connfiles: list saved secrets: %w", err)
		}
		set := map[string]bool{}
		all := map[string]bool{}
		for _, n := range saved {
			set[n], all[n] = true, true
		}
		for n := range referenced[kind] {
			all[n] = true
		}
		names := make([]string, 0, len(all))
		for n := range all {
			if only == nil || only[[2]string{kind, n}] {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		for _, n := range names {
			out = append(out, SecretView{Kind: kind, Name: n, Set: set[n], Referenced: referenced[kind][n]})
		}
	}
	return out, nil
}

// referenced collects the secret names every projected file names, per kind.
func (l *Loader) referenced() (map[string]map[string]bool, error) {
	rows, err := l.o.DB.Query(`SELECT config_json FROM connections WHERE config_json <> ''`)
	if err != nil {
		return nil, fmt.Errorf("connfiles: list referenced secrets: %w", err)
	}
	defer rows.Close()
	out := map[string]map[string]bool{KindPassword: {}, KindURI: {}}
	for rows.Next() {
		var js string
		if err := rows.Scan(&js); err != nil {
			return nil, err
		}
		var f File
		if json.Unmarshal([]byte(js), &f) != nil {
			continue
		}
		for kind, name := range refsOf(f) {
			out[kind][name] = true
		}
	}
	return out, rows.Err()
}

// refsOf maps secret kind to the name the file references.
func refsOf(f File) map[string]string {
	out := map[string]string{}
	if n := SecretName(f.Password); n != "" {
		out[KindPassword] = n
	}
	if n := SecretName(f.URI); n != "" {
		out[KindURI] = n
	}
	return out
}

// Refs maps secret kind to the name the file references.
func (f File) Refs() map[string]string { return refsOf(f) }

// FileDetail is one connection file with its text, for the editor.
type FileDetail struct {
	File     string       `json:"file"`
	ID       string       `json:"id"`
	Path     string       `json:"path"`
	Text     string       `json:"text"`
	Hash     string       `json:"hash"`
	Problem  *ProblemView `json:"problem"`
	LastGood bool         `json:"lastGood"`
	Editable bool         `json:"editable"`
	Secrets  []SecretView `json:"secrets"`
}

// ErrNotFound marks a file that is not in the folder.
var ErrNotFound = errors.New("connfiles: not found")

// BrokenError means the file has a problem, so Kira does not edit it.
type BrokenError struct {
	File string
	Line int
}

func (e *BrokenError) Error() string {
	return fmt.Sprintf("connfiles: %s is invalid at line %d", e.File, e.Line)
}

// InvalidError means Kira's own edit would make the file invalid, so nothing was written.
type InvalidError struct{ Problem *configfs.Problem }

func (e *InvalidError) Error() string {
	return "connfiles: the edit makes the file invalid: " + e.Problem.Message
}

// ErrExists marks a create over a file that is already there.
var ErrExists = errors.New("connfiles: the file exists")

// File answers one file's text and state.
func (l *Loader) File(file string) (FileDetail, error) {
	l.mu.Lock()
	e, ok := l.folder.Get(file)
	l.mu.Unlock()
	l.flush()
	if !ok {
		return FileDetail{}, fmt.Errorf("%w: %s", ErrNotFound, file)
	}
	return l.detail(e)
}

func (l *Loader) detail(e configfs.Entry[File]) (FileDetail, error) {
	src, hash, err := l.folder.Read(e.File)
	if err != nil {
		return FileDetail{}, err
	}
	d := FileDetail{
		File: e.File, Path: e.Path, Text: string(src), Hash: hash, Problem: problemView(e.Problem),
		LastGood: e.Problem != nil && e.Value != nil, Editable: true, Secrets: []SecretView{},
	}
	ids, err := l.idsByStem()
	if err != nil {
		return FileDetail{}, err
	}
	if stem, ok := StemOf(e.File); ok {
		d.ID = ids[stem]
	}
	if e.Value != nil {
		only := map[[2]string]bool{}
		for kind, name := range refsOf(*e.Value) {
			only[[2]string{kind, name}] = true
		}
		if len(only) > 0 {
			if d.Secrets, err = l.secretViews(only); err != nil {
				return FileDetail{}, err
			}
		}
	}
	return d, nil
}

// SaveFile writes text as the file's content, valid or not; the last good value keeps serving while the
// file is broken. baseHash must match the file's current text, else configfs.ErrConflict.
func (l *Loader) SaveFile(file, baseHash, text string) (FileDetail, error) {
	l.mu.Lock()
	if _, ok := l.folder.Get(file); !ok {
		l.mu.Unlock()
		l.flush()
		return FileDetail{}, fmt.Errorf("%w: %s", ErrNotFound, file)
	}
	e, err := l.folder.Save(file, baseHash, []byte(text))
	if err == nil {
		l.noteProblem(file, e.Problem)
	}
	l.mu.Unlock()
	l.flush()
	if err != nil {
		return FileDetail{}, err
	}
	return l.detail(e)
}

// ---- writes ----

var (
	nonSlug = regexp.MustCompile(`[^a-z0-9]+`)
	slugMax = 50
)

// Slug lowercases s into a stem: letters and digits, single dashes. "connection" stands in for a name with
// no usable character.
func Slug(s string) string {
	out := trimDash(nonSlug.ReplaceAllString(toLower(s), "-"))
	if len(out) > slugMax {
		out = trimDash(out[:slugMax])
	}
	if out == "" {
		return "connection"
	}
	return out
}

func trimDash(s string) string {
	for len(s) > 0 && s[0] == '-' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == '-' {
		s = s[:len(s)-1]
	}
	return s
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// stemTaken reports whether a file or a row already uses the stem.
func (l *Loader) stemTaken(stem string) bool {
	if _, err := os.Stat(filepath.Join(l.o.Dir, FileName(stem))); err == nil {
		return true
	}
	var one int
	return l.o.DB.QueryRow(`SELECT 1 FROM connections WHERE stem = ?`, stem).Scan(&one) == nil
}

// stemClaimed adds the stem-named secrets to stemTaken: a secret name outlives a file rename, so a free
// file name alone does not make its secret names free. Only stem picking asks; create must not, since
// its caller has already sealed the stem's secrets.
func (l *Loader) stemClaimed(stem string) bool {
	if l.stemTaken(stem) {
		return true
	}
	for _, k := range Kinds {
		if l.o.Secrets.Taken(stem + "." + k) {
			return true
		}
	}
	return false
}

// FreeStem returns base, or base-2, base-3 ... the first no file or row uses.
func (l *Loader) FreeStem(base string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.freeStemLocked(base)
}

func (l *Loader) freeStemLocked(base string) string {
	if !l.stemClaimed(base) {
		return base
	}
	for n := 2; ; n++ {
		suffix := fmt.Sprintf("-%d", n)
		head := base
		if len(head)+len(suffix) > 60 {
			head = trimDash(head[:60-len(suffix)])
		}
		if cand := head + suffix; !l.stemClaimed(cand) {
			return cand
		}
	}
}

// CreateAt writes a new file for stem and returns the new row id. The stem must be free.
func (l *Loader) CreateAt(stem string, f File) (string, error) {
	text, err := configfs.RenderYAML(f, Header)
	if err != nil {
		return "", err
	}
	return l.create(stem, text)
}

func (l *Loader) create(stem string, text []byte) (string, error) {
	file := FileName(stem)
	l.mu.Lock()
	var err error
	if l.stemTaken(stem) {
		err = fmt.Errorf("%w: %s", ErrExists, file)
	} else if e, serr := l.folder.Save(file, "", text); serr != nil {
		err = serr
	} else {
		l.noteProblem(file, e.Problem)
	}
	l.mu.Unlock()
	l.flush()
	if err != nil {
		return "", err
	}
	ids, err := l.idsByStem()
	if err != nil {
		return "", err
	}
	id, ok := ids[stem]
	if !ok {
		return "", fmt.Errorf("connfiles: %s was written but is not valid: %w", file, ErrNotFound)
	}
	return id, nil
}

// Duplicate copies connection id's file to a free stem next to it, with name as its name. The copy gets
// its own sealed password and URI under its stem, so changing one connection's credentials never changes
// the other's.
func (l *Loader) Duplicate(id, name string) (string, error) {
	rec, ok := l.RecordOf(id)
	if !ok {
		return "", fmt.Errorf("%w: connection %s", ErrNotFound, id)
	}
	src, _, err := l.folder.Read(FileName(rec.Stem))
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrNotFound, FileName(rec.Stem))
	}
	for attempt := 0; ; attempt++ {
		l.mu.Lock()
		stem := l.freeStemLocked(rec.Stem + "-copy")
		l.mu.Unlock()
		newID, err := l.duplicateAt(stem, src, rec, name)
		if (errors.Is(err, ErrSecretExists) || errors.Is(err, ErrExists)) && attempt < 3 {
			continue
		}
		return newID, err
	}
}

// duplicateAt copies the secrets under stem, then writes the file. It deletes only the copies it inserted.
func (l *Loader) duplicateAt(stem string, src []byte, rec Record, name string) (string, error) {
	sets := []configfs.NodeSet{{Path: []string{"name"}, Value: name}}
	var copied []movedSecret
	for _, f := range []struct{ kind, field, ref string }{
		{KindPassword, "password", rec.File.Password},
		{KindURI, "uri", rec.File.URI},
	} {
		from := SecretName(f.ref)
		if from == "" {
			continue
		}
		to := stem + "." + f.kind
		inserted, err := l.o.Secrets.Copy(f.kind, from, to)
		if err != nil {
			l.dropCopies(copied)
			return "", err
		}
		if inserted {
			copied = append(copied, movedSecret{f.kind, to})
		}
		sets = append(sets, configfs.NodeSet{Path: []string{f.field}, Value: RefOf(to)})
	}
	text, err := configfs.EditYAML(src, sets)
	if err == nil {
		var newID string
		if newID, err = l.create(stem, text); err == nil {
			return newID, nil
		}
	}
	l.dropCopies(copied)
	return "", err
}

type movedSecret struct{ kind, name string }

func (l *Loader) dropCopies(copied []movedSecret) {
	for _, c := range copied {
		_ = l.o.Secrets.Delete(c.kind, c.name)
	}
}

// Delete removes connection id's file; the row and its state go with it.
func (l *Loader) Delete(id string) error {
	rec, ok := l.RecordOf(id)
	if !ok {
		return fmt.Errorf("%w: connection %s", ErrNotFound, id)
	}
	if err := os.Remove(filepath.Join(l.o.Dir, FileName(rec.Stem))); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("connfiles: remove %s: %w", FileName(rec.Stem), err)
	}
	l.Reload()
	return nil
}

// Patch applies leaf edits to connection id's file, keeping comments and key order. A file with a
// problem is never edited, and an edit that would make the file invalid is not written.
func (l *Loader) Patch(id string, sets ...configfs.NodeSet) error {
	rec, ok := l.RecordOf(id)
	if !ok {
		return fmt.Errorf("%w: connection %s", ErrNotFound, id)
	}
	file := FileName(rec.Stem)
	l.mu.Lock()
	err := l.editLocked(file, func(src []byte) ([]byte, error) { return configfs.EditYAML(src, sets) })
	l.mu.Unlock()
	l.flush()
	return err
}

func (l *Loader) editLocked(file string, fn func(src []byte) ([]byte, error)) error {
	for attempt := 0; ; attempt++ {
		e, ok := l.folder.Get(file)
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, file)
		}
		if e.Problem != nil {
			l.noteProblem(file, e.Problem)
			return &BrokenError{File: file, Line: e.Problem.Line}
		}
		src, hash, err := l.folder.Read(file)
		if err != nil {
			return err
		}
		out, err := fn(src)
		if err != nil {
			return err
		}
		if _, p := l.dec.decode(file, out); p != nil {
			return &InvalidError{Problem: p}
		}
		saved, err := l.folder.Save(file, hash, out)
		if errors.Is(err, configfs.ErrConflict) && attempt == 0 {
			continue
		}
		if err != nil {
			return err
		}
		l.noteProblem(file, saved.Problem)
		return nil
	}
}

// ---- secrets ----

// PutNewSecret seals value as name only when the name is free, else ErrSecretExists.
func (l *Loader) PutNewSecret(kind, name, value string) error {
	return l.o.Secrets.PutNew(kind, name, value)
}

// PutSecret seals value as the secret name of kind.
func (l *Loader) PutSecret(kind, name, value string) error { return l.o.Secrets.Put(kind, name, value) }

// DeleteSecret removes the secret name of kind; an absent name is not an error.
func (l *Loader) DeleteSecret(kind, name string) error { return l.o.Secrets.Delete(kind, name) }

// Secrets is the loader's secret store.
func (l *Loader) Secrets() *Secrets { return l.o.Secrets }

// ValidSecretName reports whether name fits the secret name pattern.
func ValidSecretName(name string) bool { return SecretName(RefOf(name)) == name }
