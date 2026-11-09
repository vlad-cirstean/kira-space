package adeflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/fsmove"
)

// A config file's stem is its id. A structured save renames the file when the name's slug stops matching
// the stem, and rewrites what references the old id in one journaled operation (see Reader.Journal).

const maxStem = 60

// reservedStepIDs are route words and the finished-task marker; no step takes them as id.
var reservedStepIDs = map[string]bool{"next": true, "end": true, "stop": true, "done": true}

// legacyStepID is the handle the editor minted before step ids followed names.
var legacyStepID = regexp.MustCompile(`^stp_[0-9a-hjkmnp-tv-z]{12}$`)

// stemWithSuffix is base with the -n suffix (none for n < 2), cut so the whole stays within maxStem.
func stemWithSuffix(base string, n int) string {
	if n < 2 {
		return base
	}
	suffix := "-" + strconv.Itoa(n)
	if len(base)+len(suffix) > maxStem {
		base = strings.Trim(base[:maxStem-len(suffix)], "-")
	}
	return base + suffix
}

// freeStem returns base, else base-2, base-3 ... (capped at maxStem with the suffix) for the first stem
// taken reports free.
func freeStem(base string, taken func(stem string) bool) string {
	for n := 1; ; n++ {
		if s := stemWithSuffix(base, n); !taken(s) {
			return s
		}
	}
}

// stemMatches reports whether stem is what freeStem gives for name: slug(name), or slug(name)-n.
func stemMatches(stem, name, fallback string) bool {
	base := slug(name, fallback)
	if stem == base {
		return true
	}
	i := strings.LastIndex(stem, "-")
	if i < 0 {
		return false
	}
	n, err := strconv.Atoi(stem[i+1:])
	if err != nil || n < 2 || strconv.Itoa(n) != stem[i+1:] {
		return false
	}
	return stem == stemWithSuffix(base, n)
}

func scalarNode(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

// setID sets the mapping's id in place, keeping the node's comments.
func setID(m *yaml.Node, id string, order []string) {
	if _, n := find(m, "id"); n != nil && n.Kind == yaml.ScalarNode {
		n.Value, n.Tag = id, "!!str"
		return
	}
	put(m, "id", scalarNode(id), order)
}

func encodeDoc(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("adeflow: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("adeflow: encode: %w", err)
	}
	return buf.Bytes(), nil
}

// stepItems returns the mapping items of a workflow document's steps list.
func stepItems(root *yaml.Node) []*yaml.Node {
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	_, steps := find(root, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return nil
	}
	var out []*yaml.Node
	for _, it := range steps.Content {
		if it.Kind == yaml.MappingNode {
			out = append(out, it)
		}
	}
	return out
}

func docRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return nil
}

// rewriteRefs points every reference to step id old (result next, routes values, review implementer)
// at new; true when anything changed.
func rewriteRefs(items []*yaml.Node, old, new string) bool {
	changed := false
	swap := func(n *yaml.Node) {
		if n != nil && n.Kind == yaml.ScalarNode && strings.TrimSpace(n.Value) == old {
			n.Value, changed = new, true
		}
	}
	for _, it := range items {
		if _, res := find(it, "results"); res != nil && res.Kind == yaml.SequenceNode {
			for _, r := range res.Content {
				if r.Kind == yaml.MappingNode {
					_, next := find(r, "next")
					swap(next)
				}
			}
		}
		if _, routes := find(it, "routes"); routes != nil && routes.Kind == yaml.MappingNode {
			for i := 1; i < len(routes.Content); i += 2 {
				swap(routes.Content[i])
			}
		}
		if _, rv := find(it, "review"); rv != nil && rv.Kind == yaml.MappingNode {
			_, impl := find(rv, "implementer")
			swap(impl)
		}
	}
	return changed
}

// renameStepID renames the step whose explicit id is old to new, with every reference to it. An item
// without an id takes its id from `step:` and is left to retargetLibrary.
func renameStepID(root *yaml.Node, old, new string) bool {
	items := stepItems(root)
	changed := false
	for _, it := range items {
		if _, id := find(it, "id"); id != nil && id.Kind == yaml.ScalarNode && strings.TrimSpace(id.Value) == old {
			id.Value, changed = new, true
		}
	}
	if changed {
		rewriteRefs(items, old, new)
	}
	return changed
}

// retargetLibrary points every item `step: old` at library step new. An item without an id had old as
// step id: it follows to new with its references, unless new is already a step id of the file, then
// the item pins `id: old`. True when the document changed.
func retargetLibrary(root *yaml.Node, old, new string) bool {
	items := stepItems(root)
	taken := map[string]bool{}
	for _, it := range items {
		taken[itemID(it)] = true
	}
	changed, follow := false, false
	for _, it := range items {
		_, sv := find(it, "step")
		if sv == nil || sv.Kind != yaml.ScalarNode || strings.TrimSpace(sv.Value) != old {
			continue
		}
		sv.Value, changed = new, true
		if hasKey(it, "id") {
			continue
		}
		if taken[new] {
			put(it, "id", scalarNode(old), refOrder)
		} else {
			follow = true
		}
	}
	if follow {
		rewriteRefs(items, old, new)
	}
	return changed
}

// --- journal --------------------------------------------------------------------------------------

type renameJournal struct {
	Kind      string   `json:"kind"` // "step" or "workflow"
	Old       string   `json:"old"`  // file names
	New       string   `json:"new"`
	OldHash   string   `json:"oldHash"`
	Workflows []string `json:"workflows,omitempty"` // file names rewritten by a step rename
}

func (r *Reader) writeJournal(j renameJournal) error {
	if r.Journal == "" {
		return nil
	}
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return configfs.WriteAtomic(r.Journal, b, 0o600)
}

func (r *Reader) dropJournal() {
	if r.Journal == "" {
		return
	}
	if err := os.Remove(r.Journal); err != nil && !os.IsNotExist(err) {
		slog.Warn("ade rename: remove journal", "scope", "ade", "err", err)
	}
}

// backupper copies a file to dir/<module>/<file> before its first overwrite or removal. Nil does nothing.
type backupper struct {
	dir  string
	done map[string]bool
	n    int
}

func (b *backupper) save(module, path, file string) error {
	if b == nil || b.dir == "" {
		return nil
	}
	key := module + "/" + file
	if b.done[key] {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	dst := filepath.Join(b.dir, module, file)
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("adeflow: backup %s: %w", file, err)
	}
	if err := fsmove.CopyVerified(path, dst); err != nil {
		return fmt.Errorf("adeflow: backup %s: %w", file, err)
	}
	if b.done == nil {
		b.done = map[string]bool{}
	}
	b.done[key] = true
	b.n++
	return nil
}

// --- library step rename -------------------------------------------------------------------------

// SaveLibraryStep saves a library step from the Form editor. When the name changed, its slug no longer
// matches the file stem and keep is false, the file is renamed to the new stem and every workflow that references the
// step follows. keep marks a file that came from the shared repo: it never moves.
func (r *Reader) SaveLibraryStep(file, baseHash string, st adewire.WorkflowStep, keep bool) (adewire.StepEntry, error) {
	l := r.Steps
	if l == nil {
		return adewire.StepEntry{}, invalidf("the step library is not available")
	}
	stem := strings.TrimSuffix(file, ".yaml")
	if keep || stemMatches(stem, st.Name, "step") {
		return l.Save(file, baseHash, st)
	}
	p, err := l.path(file)
	if err != nil {
		return adewire.StepEntry{}, err
	}
	if st.ID != stem {
		return adewire.StepEntry{}, invalidf("id must equal the file name without .yaml")
	}
	if !nameChanged(p, st.Name) {
		return l.Save(file, baseHash, st)
	}
	r.wmu.Lock()
	defer r.wmu.Unlock()
	l.wmu.Lock()
	defer l.wmu.Unlock()
	newFile, err := r.renameStep(file, baseHash, st.Name, func(newStem string, doc *yaml.Node) ([]byte, error) {
		st.ID = newStem
		return encodeStep(doc, st)
	}, nil)
	if err != nil {
		return adewire.StepEntry{}, err
	}
	return l.entry(newFile)
}

// nameChanged reports whether the file at path exists, parses and holds a different `name`. A save that
// leaves the name alone never moves a file, so a hand-named file keeps its name until it is renamed.
func nameChanged(path, name string) bool {
	src, err := configfs.ReadCapped(path, configfs.MaxFileBytes)
	if err != nil {
		return false
	}
	var doc yaml.Node
	if yaml.Unmarshal(src, &doc) != nil {
		return false
	}
	root := docRoot(&doc)
	if root == nil || root.Kind != yaml.MappingNode {
		return false
	}
	_, n := find(root, "name")
	return n == nil || strings.TrimSpace(n.Value) != strings.TrimSpace(name)
}

func stepTakenIn(dir string) func(string) bool {
	return func(s string) bool {
		if reservedStepIDs[s] {
			return true
		}
		_, err := os.Lstat(filepath.Join(dir, s+".yaml"))
		return err == nil
	}
}

// renameStep renames library file old for name. build produces the new file's bytes for the new stem from
// the old document. Both wmu locks are held.
func (r *Reader) renameStep(old, baseHash, name string, build func(newStem string, doc *yaml.Node) ([]byte, error), bk *backupper) (string, error) {
	l := r.Steps
	oldStem := strings.TrimSuffix(old, ".yaml")
	oldPath := filepath.Join(l.Dir, old)
	src, err := configfs.ReadCapped(oldPath, configfs.MaxFileBytes)
	if err != nil {
		return "", fmt.Errorf("adeflow: read %s: %w", old, err)
	}
	if baseHash != "" && configfs.TextHash(src) != baseHash {
		return "", fmt.Errorf("%w: the file changed since it was loaded", ErrConflict)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return "", invalidf("fix the YAML error on line %d first", yamlErrLineOf(err))
	}
	users := r.StepUsers()[oldStem]
	sort.Strings(users)
	files := make([]string, 0, len(users))
	for _, u := range users {
		wfile := u + ".yaml"
		wsrc, err := configfs.ReadCapped(filepath.Join(r.Dir, wfile), configfs.MaxFileBytes)
		if err != nil {
			return "", invalidf("workflow %s cannot be read; fix it before renaming step %q", u, oldStem)
		}
		var wdoc yaml.Node
		if err := yaml.Unmarshal(wsrc, &wdoc); err != nil {
			return "", invalidf("workflow %s has a YAML error on line %d; fix it before renaming step %q", u, yamlErrLineOf(err), oldStem)
		}
		files = append(files, wfile)
	}
	newStem := freeStem(slug(name, "step"), stepTakenIn(l.Dir))
	out, err := build(newStem, &doc)
	if err != nil {
		return "", err
	}
	newFile := newStem + ".yaml"
	if err := bk.save(StepsFolder, oldPath, old); err != nil {
		return "", err
	}
	for _, f := range files {
		if err := bk.save(WorkflowsFolder, filepath.Join(r.Dir, f), f); err != nil {
			return "", err
		}
	}
	j := renameJournal{Kind: "step", Old: old, New: newFile, OldHash: configfs.TextHash(src), Workflows: files}
	if err := r.writeJournal(j); err != nil {
		return "", fmt.Errorf("adeflow: write rename journal: %w", err)
	}
	if err := configfs.WriteAtomic(filepath.Join(l.Dir, newFile), out, 0o644); err != nil {
		r.dropJournal()
		return "", err
	}
	if err := r.finishStepRename(j); err != nil {
		return "", err
	}
	return newFile, nil
}

// finishStepRename redoes the steps after the new file exists: retarget the listed workflows, remove the
// old file when untouched since, drop the journal. Idempotent. A workflow it cannot rewrite keeps the old
// file and the journal, so nothing ever points at a missing step.
func (r *Reader) finishStepRename(j renameJournal) error {
	oldStem, newStem := strings.TrimSuffix(j.Old, ".yaml"), strings.TrimSuffix(j.New, ".yaml")
	for _, f := range j.Workflows {
		if err := r.retargetWorkflowFile(f, oldStem, newStem); err != nil {
			return err
		}
	}
	r.removeOld(r.Steps.Dir, j)
	r.dropJournal()
	return nil
}

// retargetWorkflowFile applies retargetLibrary to one workflow file; a missing file is skipped.
func (r *Reader) retargetWorkflowFile(file, oldStem, newStem string) error {
	p := filepath.Join(r.Dir, file)
	src, err := configfs.ReadCapped(p, configfs.MaxFileBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("adeflow: read %s: %w", file, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return invalidf("workflow %s has a YAML error on line %d", strings.TrimSuffix(file, ".yaml"), yamlErrLineOf(err))
	}
	if !retargetLibrary(docRoot(&doc), oldStem, newStem) {
		return nil
	}
	out, err := encodeDoc(&doc)
	if err != nil {
		return err
	}
	return configfs.WriteAtomic(p, out, 0o644)
}

// removeOld removes the renamed-away file only while its text is still the one the rename read.
func (r *Reader) removeOld(dir string, j renameJournal) {
	p := filepath.Join(dir, j.Old)
	cur, err := configfs.ReadCapped(p, configfs.MaxFileBytes)
	switch {
	case err == nil && configfs.TextHash(cur) == j.OldHash:
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			slog.Warn("ade rename: remove old file", "scope", "ade", "file", j.Old, "err", err)
		}
	case err == nil:
		slog.Warn("ade rename: old file changed since the rename began, kept", "scope", "ade", "file", j.Old)
	case !os.IsNotExist(err):
		slog.Warn("ade rename: read old file", "scope", "ade", "file", j.Old, "err", err)
	}
}

// --- workflow rename -----------------------------------------------------------------------------

// SaveRenaming saves a workflow from the graph editor. When the name changed, its slug no longer matches
// the file stem and keep is false, the file moves to the new stem and retarget(oldID, newID) updates what points at
// the id (the task rows). keep marks a shared file, which never moves.
func (r *Reader) SaveRenaming(file, baseHash string, wf adewire.Workflow, keep bool, retarget func(oldID, newID string) error) (adewire.WorkflowEntry, error) {
	stem := strings.TrimSuffix(file, ".yaml")
	if keep || stemMatches(stem, wf.Name, "workflow") {
		return r.Save(file, baseHash, wf)
	}
	p, err := r.path(file)
	if err != nil {
		return adewire.WorkflowEntry{}, err
	}
	if wf.ID != stem {
		return adewire.WorkflowEntry{}, invalidf("id must equal the file name without .yaml")
	}
	if !nameChanged(p, wf.Name) {
		return r.Save(file, baseHash, wf)
	}
	r.wmu.Lock()
	defer r.wmu.Unlock()
	newFile, err := r.renameWorkflow(file, baseHash, wf.Name, retarget, nil, func(newStem string, doc *yaml.Node) ([]byte, error) {
		wf.ID = newStem
		return encodeWorkflow(doc, wf, r.Steps)
	})
	if err != nil {
		return adewire.WorkflowEntry{}, err
	}
	return r.entry(newFile, filepath.Join(r.Dir, newFile))
}

// renameWorkflow renames workflow file old for name; wmu is held.
func (r *Reader) renameWorkflow(old, baseHash, name string, retarget func(oldID, newID string) error, bk *backupper, build func(newStem string, doc *yaml.Node) ([]byte, error)) (string, error) {
	oldPath := filepath.Join(r.Dir, old)
	src, err := configfs.ReadCapped(oldPath, configfs.MaxFileBytes)
	if err != nil {
		return "", fmt.Errorf("adeflow: read %s: %w", old, err)
	}
	if baseHash != "" && configfs.TextHash(src) != baseHash {
		return "", fmt.Errorf("%w: the file changed since it was loaded", ErrConflict)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return "", invalidf("fix the YAML error on line %d first", yamlErrLineOf(err))
	}
	newStem := freeStem(slug(name, "workflow"), func(s string) bool {
		_, err := os.Lstat(filepath.Join(r.Dir, s+".yaml"))
		return err == nil
	})
	out, err := build(newStem, &doc)
	if err != nil {
		return "", err
	}
	newFile := newStem + ".yaml"
	if err := bk.save(WorkflowsFolder, oldPath, old); err != nil {
		return "", err
	}
	j := renameJournal{Kind: "workflow", Old: old, New: newFile, OldHash: configfs.TextHash(src)}
	if err := r.writeJournal(j); err != nil {
		return "", fmt.Errorf("adeflow: write rename journal: %w", err)
	}
	newPath := filepath.Join(r.Dir, newFile)
	if err := configfs.WriteAtomic(newPath, out, 0o644); err != nil {
		r.dropJournal()
		return "", err
	}
	if err := r.finishWorkflowRename(j, retarget); err != nil {
		// Nothing outside the files changed yet in the common failure (retarget): back out.
		if rmErr := os.Remove(newPath); rmErr == nil || os.IsNotExist(rmErr) {
			r.dropJournal()
		}
		return "", err
	}
	return newFile, nil
}

func (r *Reader) finishWorkflowRename(j renameJournal, retarget func(oldID, newID string) error) error {
	if retarget != nil {
		if err := retarget(strings.TrimSuffix(j.Old, ".yaml"), strings.TrimSuffix(j.New, ".yaml")); err != nil {
			return fmt.Errorf("adeflow: retarget workflow id: %w", err)
		}
	}
	r.removeOld(r.Dir, j)
	r.dropJournal()
	return nil
}

// --- recovery ------------------------------------------------------------------------------------

// RecoverRename settles a rename a crash interrupted: new file absent drops the journal, else the rename is
// finished. retarget is the workflow-id update of SaveRenaming. Idempotent; a failure keeps the journal.
func (r *Reader) RecoverRename(retarget func(oldID, newID string) error) {
	if r.Journal == "" {
		return
	}
	b, err := os.ReadFile(r.Journal)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("ade rename: read journal", "scope", "ade", "err", err)
		}
		return
	}
	var j renameJournal
	if err := json.Unmarshal(b, &j); err != nil || (j.Kind != "step" && j.Kind != "workflow") || !fileNamePattern.MatchString(j.Old) || !fileNamePattern.MatchString(j.New) {
		slog.Warn("ade rename: unreadable journal dropped", "scope", "ade")
		r.dropJournal()
		return
	}
	r.wmu.Lock()
	defer r.wmu.Unlock()
	if r.Steps != nil {
		r.Steps.wmu.Lock()
		defer r.Steps.wmu.Unlock()
	}
	dir := r.Dir
	if j.Kind == "step" {
		if r.Steps == nil {
			slog.Warn("ade rename: step journal without a library", "scope", "ade")
			return
		}
		dir = r.Steps.Dir
	}
	if _, err := os.Lstat(filepath.Join(dir, j.New)); errors.Is(err, os.ErrNotExist) {
		r.dropJournal()
		return
	}
	if j.Kind == "step" {
		err = r.finishStepRename(j)
	} else {
		err = r.finishWorkflowRename(j, retarget)
	}
	if err != nil {
		slog.Warn("ade rename: recovery incomplete, retried at next start", "scope", "ade", "old", j.Old, "new", j.New, "err", err)
	}
}

// --- one-time migration --------------------------------------------------------------------------

// MigrateOptions configures MigrateNames.
type MigrateOptions struct {
	// Marker is the file whose presence says the migration ran; empty disables the migration.
	Marker string
	// BackupDir receives a copy of every file before its first change.
	BackupDir string
	// Keep reports a file that came from the shared repo; it keeps its stem.
	Keep func(module, file string) bool
	// Retarget is SaveRenaming's workflow-id update.
	Retarget func(oldID, newID string) error
	Now      func() time.Time
}

// MigrateNames converts existing files to the readable scheme once: legacy `stp_` step ids become the slug
// of the step name, library and workflow stems follow their names. Every step is idempotent, so a crash
// before the marker re-runs safely. Files that cannot be converted are logged and skipped; the marker is
// written only when none was.
func (r *Reader) MigrateNames(o MigrateOptions) error {
	if o.Marker == "" {
		return nil
	}
	if _, err := os.Stat(o.Marker); err == nil {
		return nil
	}
	r.wmu.Lock()
	defer r.wmu.Unlock()
	if r.Steps != nil {
		r.Steps.wmu.Lock()
		defer r.Steps.wmu.Unlock()
	}
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	var bk *backupper
	if o.BackupDir != "" {
		bk = &backupper{dir: o.BackupDir + "-" + now().UTC().Format("20060102T150405Z")}
	}
	keep := func(module, file string) bool { return o.Keep != nil && o.Keep(module, file) }
	skipped := 0
	ids := r.migrateInlineIDs(bk, &skipped)
	steps := 0
	if r.Steps != nil {
		steps = r.migrateStepStems(bk, keep, &skipped)
	}
	workflows := r.migrateWorkflowStems(bk, keep, o.Retarget, &skipped)
	if bk != nil && bk.n > 0 || ids+steps+workflows > 0 {
		slog.Info("ade names migration", "scope", "ade", "inlineIds", ids, "steps", steps, "workflows", workflows, "skipped", skipped, "backup", backupPath(bk))
	}
	if skipped > 0 {
		return nil
	}
	return configfs.WriteAtomic(o.Marker, []byte(now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

func backupPath(b *backupper) string {
	if b == nil {
		return ""
	}
	return b.dir
}

// migrateInlineIDs renames every legacy stp_ step id to the slug of its step name. Returns the count.
func (r *Reader) migrateInlineIDs(bk *backupper, skipped *int) int {
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("ade names migration: read workflows", "scope", "ade", "err", err)
			*skipped++
		}
		return 0
	}
	count := 0
	for _, de := range entries {
		file := de.Name()
		if !de.Type().IsRegular() || !fileNamePattern.MatchString(file) {
			continue
		}
		n, ok := r.migrateInlineIDsFile(bk, file)
		if !ok {
			*skipped++
			continue
		}
		count += n
	}
	return count
}

// migrateInlineIDsFile rewrites legacy step ids in one workflow file. ok is false when the file was skipped.
func (r *Reader) migrateInlineIDsFile(bk *backupper, file string) (changed int, ok bool) {
	p := filepath.Join(r.Dir, file)
	src, err := configfs.ReadCapped(p, configfs.MaxFileBytes)
	if err != nil {
		slog.Warn("ade names migration: read", "scope", "ade", "file", file, "err", err)
		return 0, false
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		slog.Warn("ade names migration: workflow skipped, YAML error", "scope", "ade", "file", file)
		return 0, false
	}
	changed = renameLegacyStepIDs(&doc)
	if changed == 0 {
		return 0, true
	}
	out, err := encodeDoc(&doc)
	if err == nil {
		err = bk.save(WorkflowsFolder, p, file)
	}
	if err == nil {
		err = configfs.WriteAtomic(p, out, 0o644)
	}
	if err != nil {
		slog.Warn("ade names migration: workflow ids", "scope", "ade", "file", file, "err", err)
		return 0, false
	}
	return changed, true
}

// renameLegacyStepIDs renames each stp_ step id in doc to a free slug of its step name. Returns the count.
func renameLegacyStepIDs(doc *yaml.Node) int {
	root := docRoot(doc)
	items := stepItems(root)
	taken := map[string]bool{}
	for _, it := range items {
		taken[itemID(it)] = true
	}
	changed := 0
	for _, it := range items {
		_, idn := find(it, "id")
		if idn == nil || idn.Kind != yaml.ScalarNode || !legacyStepID.MatchString(strings.TrimSpace(idn.Value)) {
			continue
		}
		old := strings.TrimSpace(idn.Value)
		fresh := freeStem(slug(stepItemName(it), "step"), func(s string) bool { return reservedStepIDs[s] || taken[s] })
		if renameStepID(root, old, fresh) {
			delete(taken, old)
			taken[fresh] = true
			changed++
		}
	}
	return changed
}

func stepItemName(it *yaml.Node) string {
	if _, n := find(it, "name"); n != nil && n.Kind == yaml.ScalarNode {
		return n.Value
	}
	if _, s := find(it, "step"); s != nil && s.Kind == yaml.ScalarNode {
		return s.Value
	}
	return ""
}

func (r *Reader) migrateStepStems(bk *backupper, keep func(module, file string) bool, skipped *int) int {
	count := 0
	for _, e := range r.Steps.folder().List() {
		if e.Problem != nil || e.Value == nil {
			continue
		}
		stem := strings.TrimSuffix(e.File, ".yaml")
		if stemMatches(stem, e.Value.Name, "step") || keep(StepsFolder, e.File) {
			continue
		}
		_, err := r.renameStep(e.File, "", e.Value.Name, func(newStem string, doc *yaml.Node) ([]byte, error) {
			root := docRoot(doc)
			if root == nil {
				return nil, invalidf("the step file is empty")
			}
			setID(root, newStem, stepOrder)
			out, err := encodeDoc(doc)
			if err != nil {
				return nil, err
			}
			if _, _, werr := ParseStep(out); werr != nil {
				return nil, invalidf("%s", werr.Message)
			}
			return out, nil
		}, bk)
		if err != nil {
			slog.Warn("ade names migration: step not renamed", "scope", "ade", "file", e.File, "err", err)
			*skipped++
			continue
		}
		count++
	}
	return count
}

func (r *Reader) migrateWorkflowStems(bk *backupper, keep func(module, file string) bool, retarget func(oldID, newID string) error, skipped *int) int {
	count := 0
	for _, e := range r.folder().List() {
		if e.Problem != nil || e.Value == nil {
			continue
		}
		stem := strings.TrimSuffix(e.File, ".yaml")
		if stemMatches(stem, e.Value.Name, "workflow") || keep(WorkflowsFolder, e.File) {
			continue
		}
		_, err := r.renameWorkflow(e.File, "", e.Value.Name, retarget, bk, func(newStem string, doc *yaml.Node) ([]byte, error) {
			root := docRoot(doc)
			if root == nil {
				return nil, invalidf("the workflow file is empty")
			}
			setID(root, newStem, topOrder)
			out, err := encodeDoc(doc)
			if err != nil {
				return nil, err
			}
			if _, werr := ParseWith(out, r.lookup()); werr != nil {
				return nil, invalidf("%s", werr.Message)
			}
			return out, nil
		})
		if err != nil {
			slog.Warn("ade names migration: workflow not renamed", "scope", "ade", "file", e.File, "err", err)
			*skipped++
			continue
		}
		count++
	}
	return count
}
