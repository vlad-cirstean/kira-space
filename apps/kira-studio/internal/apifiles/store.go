package apifiles

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// Options builds a Store.
type Options struct {
	DB *sql.DB
	// Dir is the api folder, <home>/api.
	Dir     string
	Secrets *Secrets
	// OnChange receives what a load or a write made stale. It runs outside the store's lock.
	OnChange func(Changes)
}

// Store keeps the api_* tables in step with the files: files to the DB only. It is long-lived.
type Store struct {
	o            Options
	dec          decoders
	collections  *configfs.Folder[CollectionFile]
	environments *configfs.Folder[EnvironmentFile]

	mu       sync.Mutex // serializes loads and writes, and guards the fields below
	pend     *Changes
	problems map[string]string
	origins  map[string]string
	stop     func()
}

// New builds the store; it creates nothing on disk.
func New(o Options) (*Store, error) {
	dec, err := newDecoders()
	if err != nil {
		return nil, err
	}
	s := &Store{o: o, dec: dec, pend: &Changes{}, problems: map[string]string{}, origins: map[string]string{}}
	s.collections = &configfs.Folder[CollectionFile]{
		Dir: o.Dir, Ext: CollectionExt, Name: collectionNameRe, Decode: dec.collection,
		Projection: &projection{s: s, folder: folderCollections, ext: CollectionExt, record: s.recordCollection, forget: s.forgetCollection},
	}
	s.environments = &configfs.Folder[EnvironmentFile]{
		Dir: o.Dir, Ext: EnvironmentExt, Name: environmentNameRe, Decode: dec.environment,
		Projection: &projection{s: s, folder: folderEnvironments, ext: EnvironmentExt, record: s.recordEnvironment, forget: s.forgetEnvironment},
	}
	return s, nil
}

// Dir is the api folder.
func (s *Store) Dir() string { return s.o.Dir }

// Start migrates the legacy rows once, loads the folder, writes AGENTS.md and watches the folder. A
// failed migration skips the rest: an unfinished export never projects. A failure of the docs or the
// watch is logged and never fatal.
func (s *Store) Start() {
	if err := configfs.MigrateOnce(s.o.DB, "api", s.migrate); err != nil {
		slog.Warn("api: migrate to files", "scope", "startup", "err", err)
		return
	}
	s.Sync()
	model, err := DocModel()
	if err == nil {
		err = s.collections.WriteDocs(model)
	}
	if err != nil {
		slog.Warn("api: write AGENTS.md", "scope", "startup", "err", err)
	}
	stop, err := configfs.WatchDir(s.o.Dir, ".yaml", s.Sync)
	if err != nil {
		slog.Warn("api: watch folder", "scope", "startup", "err", err)
		return
	}
	s.mu.Lock()
	s.stop = stop
	s.mu.Unlock()
}

// Close stops the watch.
func (s *Store) Close() {
	s.mu.Lock()
	stop := s.stop
	s.stop = nil
	s.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// Sync reads both folders into the DB and reports what changed.
func (s *Store) Sync() {
	s.mu.Lock()
	s.pend = &Changes{}
	s.syncLocked()
	ch := *s.pend
	s.mu.Unlock()
	s.emit(ch)
}

func (s *Store) emit(ch Changes) {
	if !ch.Empty() && s.o.OnChange != nil {
		s.o.OnChange(ch)
	}
}

func (s *Store) syncLocked() ([]configfs.Entry[CollectionFile], []configfs.Entry[EnvironmentFile]) {
	ce := s.collections.List()
	ee := s.environments.List()
	s.noteProblems(ce, ee)
	if err := s.pruneOrphans(ce, ee); err != nil {
		slog.Warn("api: prune rows without a file", "scope", "config", "err", err)
	}
	return ce, ee
}

// pruneOrphans drops projected rows whose file is gone.
func (s *Store) pruneOrphans(ce []configfs.Entry[CollectionFile], ee []configfs.Entry[EnvironmentFile]) error {
	return s.tx(func(tx *sql.Tx, ch *Changes) error {
		for _, t := range []struct {
			table, ext string
			files      []string
			scope      string
			changed    *bool
		}{
			{"api_collections", CollectionExt, collectionFiles(ce), OwnerCollection, &ch.Tree},
			{"api_environments", EnvironmentExt, environmentFiles(ee), OwnerEnvironment, &ch.Environments},
		} {
			present := map[string]bool{}
			for _, f := range t.files {
				if stem, ok := StemOf(f, t.ext); ok {
					present[stem] = true
				}
			}
			rows, err := tx.Query(`SELECT id, stem FROM ` + t.table + ` WHERE stem IS NOT NULL`)
			if err != nil {
				return err
			}
			var gone []string
			for rows.Next() {
				var id, stem string
				if err := rows.Scan(&id, &stem); err != nil {
					rows.Close()
					return err
				}
				if !present[stem] {
					gone = append(gone, id)
				}
			}
			rows.Close()
			for _, id := range gone {
				if _, err := tx.Exec(`DELETE FROM `+t.table+` WHERE id = ?`, id); err != nil {
					return err
				}
				*t.changed = true
				ch.variables(t.scope, id)
			}
			if len(gone) > 0 {
				if err := reindex(tx, t.table); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func collectionFiles(es []configfs.Entry[CollectionFile]) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.File
	}
	return out
}

func environmentFiles(es []configfs.Entry[EnvironmentFile]) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.File
	}
	return out
}

// noteProblems records each file's problem and flags a Config change when the set differs.
func (s *Store) noteProblems(ce []configfs.Entry[CollectionFile], ee []configfs.Entry[EnvironmentFile]) {
	next := map[string]string{}
	for _, e := range ce {
		if e.Problem != nil {
			next[e.File] = fmt.Sprintf("%d:%s", e.Problem.Line, e.Problem.Message)
		}
	}
	for _, e := range ee {
		if e.Problem != nil {
			next[e.File] = fmt.Sprintf("%d:%s", e.Problem.Line, e.Problem.Message)
		}
	}
	if len(next) != len(s.problems) {
		s.pend.Config = true
	} else {
		for k, v := range next {
			if s.problems[k] != v {
				s.pend.Config = true
				break
			}
		}
	}
	s.problems = next
}

// FileView is one config file as the Api module shows it.
type FileView struct {
	File     string       `json:"file"`
	Kind     string       `json:"kind"`
	Path     string       `json:"path"`
	Name     string       `json:"name"`
	Problem  *ProblemView `json:"problem"`
	LastGood bool         `json:"lastGood"`
}

// ProblemView is why a file is invalid.
type ProblemView struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// SecretView is one secret name: saved, referenced by a file, or both. Never the value.
type SecretView struct {
	Name       string `json:"name"`
	Set        bool   `json:"set"`
	Referenced bool   `json:"referenced"`
}

// View is everything the Api config panels show.
type View struct {
	Dir     string         `json:"dir"`
	Files   []FileView     `json:"files"`
	Secrets []SecretView   `json:"secrets"`
	Storage secrets.Status `json:"storage"`
}

// View loads the folder and answers every file's state and the secret names.
func (s *Store) View() (View, error) {
	s.mu.Lock()
	s.pend = &Changes{}
	ce, ee := s.syncLocked()
	ch := *s.pend
	s.mu.Unlock()
	s.emit(ch)

	v := View{Dir: s.o.Dir, Files: []FileView{}, Secrets: []SecretView{}}
	for _, e := range ce {
		fv := FileView{File: e.File, Kind: "collection", Path: e.Path, LastGood: e.Problem != nil && e.Value != nil}
		if e.Value != nil {
			fv.Name = e.Value.Name
		}
		fv.Problem = problemView(e.Problem)
		v.Files = append(v.Files, fv)
	}
	for _, e := range ee {
		fv := FileView{File: e.File, Kind: "environment", Path: e.Path, LastGood: e.Problem != nil && e.Value != nil}
		if e.Value != nil {
			fv.Name = e.Value.Name
		}
		fv.Problem = problemView(e.Problem)
		v.Files = append(v.Files, fv)
	}
	names, referenced, err := s.secretNames()
	if err != nil {
		return View{}, err
	}
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	all := map[string]bool{}
	for n := range set {
		all[n] = true
	}
	for n := range referenced {
		all[n] = true
	}
	sorted := make([]string, 0, len(all))
	for n := range all {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	for _, n := range sorted {
		v.Secrets = append(v.Secrets, SecretView{Name: n, Set: set[n], Referenced: referenced[n]})
	}
	v.Storage = s.o.Secrets.Status()
	return v, nil
}

func problemView(p *configfs.Problem) *ProblemView {
	if p == nil {
		return nil
	}
	return &ProblemView{Line: p.Line, Message: p.Message}
}

func (s *Store) secretNames() (saved []string, referenced map[string]bool, err error) {
	saved, err = s.o.Secrets.Names()
	if err != nil {
		return nil, nil, fmt.Errorf("apifiles: list saved secrets: %w", err)
	}
	rows, err := s.o.DB.Query(`SELECT DISTINCT secret_ref FROM api_variables WHERE is_secret = 1 AND secret_ref IS NOT NULL`)
	if err != nil {
		return nil, nil, fmt.Errorf("apifiles: list referenced secrets: %w", err)
	}
	defer rows.Close()
	referenced = map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, nil, err
		}
		referenced[n] = true
	}
	return saved, referenced, rows.Err()
}

// ---- origins of imported items ----

func originKey(stem, key string) string { return stem + "\x00" + key }

func (s *Store) setOrigin(stem, key, originJSON string) { s.origins[originKey(stem, key)] = originJSON }

func (s *Store) takeOrigin(stem, key string) string {
	k := originKey(stem, key)
	if v, ok := s.origins[k]; ok {
		delete(s.origins, k)
		return v
	}
	return "{}"
}

// ---- writes ----

// Write errors, matched with errors.Is or errors.As.
var ErrNotFound = errors.New("apifiles: not found")

// BrokenError means the file has a problem, so Kira does not edit it.
type BrokenError struct {
	File string
	Line int
}

func (e *BrokenError) Error() string {
	return fmt.Sprintf("apifiles: %s is invalid at line %d", e.File, e.Line)
}

// InvalidError means Kira's own edit would make the file invalid, so nothing was written.
type InvalidError struct{ Problem *configfs.Problem }

func (e *InvalidError) Error() string {
	return "apifiles: the edit makes the file invalid: " + e.Problem.Message
}

var (
	collectionNameRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}\.collection\.yaml$`)
	environmentNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}\.environment\.yaml$`)
)

func fileExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// locate finds the collection file and item id of an item row.
func (s *Store) locate(itemID string) (file, key, collectionID string, err error) {
	var stem string
	err = s.o.DB.QueryRow(`SELECT c.stem, i.item_key, c.id FROM api_items i JOIN api_collections c ON c.id = i.collection_id
		WHERE i.id = ? AND c.stem IS NOT NULL AND i.item_key IS NOT NULL`, itemID).Scan(&stem, &key, &collectionID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", fmt.Errorf("%w: item %s", ErrNotFound, itemID)
	}
	if err != nil {
		return "", "", "", fmt.Errorf("apifiles: locate item %s: %w", itemID, err)
	}
	return stem + CollectionExt, key, collectionID, nil
}

func (s *Store) collectionFile(collectionID string) (string, error) {
	var stem sql.NullString
	err := s.o.DB.QueryRow(`SELECT stem FROM api_collections WHERE id = ?`, collectionID).Scan(&stem)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !stem.Valid {
		return "", fmt.Errorf("%w: collection %s", ErrNotFound, collectionID)
	}
	if err != nil {
		return "", fmt.Errorf("apifiles: locate collection %s: %w", collectionID, err)
	}
	return stem.String + CollectionExt, nil
}

// editCollection applies fn to the collection file's text and saves it, retrying once when the file
// changed meanwhile. A file with a problem is never edited, and an edit that would make the file
// invalid is not written.
func (s *Store) editCollection(file string, fn func(src []byte) ([]byte, error)) error {
	s.mu.Lock()
	s.pend = &Changes{}
	err := s.editLocked(file, fn)
	ch := *s.pend
	s.mu.Unlock()
	s.emit(ch)
	return err
}

func (s *Store) editLocked(file string, fn func(src []byte) ([]byte, error)) error {
	for attempt := 0; ; attempt++ {
		if e, ok := s.collections.Get(file); !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, file)
		} else if e.Problem != nil {
			s.noteProblems1(file, e.Problem)
			return &BrokenError{File: file, Line: e.Problem.Line}
		}
		src, hash, err := s.collections.Read(file)
		if err != nil {
			return err
		}
		out, err := fn(src)
		if err != nil {
			return err
		}
		if _, p := s.dec.collection(file, out); p != nil {
			return &InvalidError{Problem: p}
		}
		e, err := s.collections.Save(file, hash, out)
		if errors.Is(err, configfs.ErrConflict) && attempt == 0 {
			continue
		}
		if err != nil {
			return err
		}
		s.noteProblems1(file, e.Problem)
		return nil
	}
}

// noteProblems1 records one file's problem after a write.
func (s *Store) noteProblems1(file string, p *configfs.Problem) {
	sig := ""
	if p != nil {
		sig = fmt.Sprintf("%d:%s", p.Line, p.Message)
	}
	if s.problems[file] != sig {
		s.pend.Config = true
	}
	if sig == "" {
		delete(s.problems, file)
		return
	}
	s.problems[file] = sig
}

// createFile writes a new file of folder f under a stem free of both kinds' files.
func createFile[T any](s *Store, f *configfs.Folder[T], ext, base string, render func(stem string) ([]byte, error)) (string, error) {
	s.mu.Lock()
	s.pend = &Changes{}
	stem := s.freeStem(ext, base)
	var err error
	if text, rerr := render(stem); rerr != nil {
		err = rerr
	} else if _, serr := f.Save(stem+ext, "", text); serr != nil {
		err = serr
	}
	ch := *s.pend
	s.mu.Unlock()
	s.emit(ch)
	if err != nil {
		return "", err
	}
	return stem, nil
}

// PutSecret seals value as the secret name.
func (s *Store) PutSecret(name, value string) error { return s.o.Secrets.Put(name, value) }

// DeleteSecret removes the secret name; an absent name is not an error.
func (s *Store) DeleteSecret(name string) error { return s.o.Secrets.Delete(name) }

// CreateCollection writes a new empty collection file named after name and returns the row id.
func (s *Store) CreateCollection(name string) (string, error) {
	stem, err := createFile(s, s.collections, CollectionExt, Slug(name, "collection"), func(string) ([]byte, error) {
		return configfs.RenderYAML(CollectionFile{Name: name, Items: []Item{}}, "")
	})
	if err != nil {
		return "", err
	}
	return s.rowID(`SELECT id FROM api_collections WHERE stem = ?`, stem)
}

// CreateEnvironment writes a new environment file and returns the row id.
func (s *Store) CreateEnvironment(f EnvironmentFile) (string, error) {
	stem, err := createFile(s, s.environments, EnvironmentExt, Slug(f.Name, "environment"), func(string) ([]byte, error) {
		return configfs.RenderYAML(f, "")
	})
	if err != nil {
		return "", err
	}
	return s.rowID(`SELECT id FROM api_environments WHERE stem = ?`, stem)
}

func (s *Store) rowID(query string, args ...any) (string, error) {
	var id string
	if err := s.o.DB.QueryRow(query, args...).Scan(&id); err != nil {
		return "", fmt.Errorf("apifiles: the written row is missing: %w", err)
	}
	return id, nil
}

// SaveRequest replaces an HTTP item's request and name in its file.
func (s *Store) SaveRequest(itemID, name string, req HTTP) error {
	file, key, _, err := s.locate(itemID)
	if err != nil {
		return err
	}
	return s.editCollection(file, func(src []byte) ([]byte, error) { return replaceRequest(src, key, name, &req, nil) })
}

// SaveGrpcRequest replaces a gRPC item's request and name in its file.
func (s *Store) SaveGrpcRequest(itemID, name string, req GRPC) error {
	file, key, _, err := s.locate(itemID)
	if err != nil {
		return err
	}
	return s.editCollection(file, func(src []byte) ([]byte, error) { return replaceRequest(src, key, name, nil, &req) })
}

// NewItem is a folder or a request to add to a collection file; exactly one of Folder, HTTP, GRPC.
type NewItem struct {
	Name   string
	Folder bool
	HTTP   *HTTP
	GRPC   *GRPC
}

// CreateItem appends an item under the folder parentItemID of the collection, or at the root when it is
// "", and returns the new row id.
func (s *Store) CreateItem(collectionID, parentItemID string, n NewItem) (string, error) {
	file, err := s.collectionFile(collectionID)
	if err != nil {
		return "", err
	}
	parentKey := ""
	if parentItemID != "" {
		pf, key, owner, err := s.locate(parentItemID)
		if err != nil {
			return "", err
		}
		if owner != collectionID || pf != file {
			return "", fmt.Errorf("%w: item %s is not in collection %s", ErrNotFolder, parentItemID, collectionID)
		}
		parentKey = key
	}
	newKey := ""
	err = s.editCollection(file, func(src []byte) ([]byte, error) {
		f, _ := s.dec.collection(file, src)
		taken := map[string]bool{}
		var collect func(items []Item)
		collect = func(items []Item) {
			for _, it := range items {
				taken[it.ID] = true
				if it.Items != nil {
					collect(*it.Items)
				}
			}
		}
		collect(f.Items)
		item := Item{ID: freeName(Slug(n.Name, "item"), func(c string) bool { return taken[c] }), Name: n.Name, HTTP: n.HTTP, GRPC: n.GRPC}
		if n.Folder {
			item.Items = &[]Item{}
		}
		newKey = item.ID
		return appendItem(src, parentKey, item)
	})
	if err != nil {
		return "", err
	}
	return s.rowID(`SELECT id FROM api_items WHERE collection_id = ? AND item_key = ?`, collectionID, newKey)
}

// StemOfCollection returns the file stem of a collection row.
func (s *Store) StemOfCollection(collectionID string) (string, error) {
	file, err := s.collectionFile(collectionID)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(file, CollectionExt), nil
}
