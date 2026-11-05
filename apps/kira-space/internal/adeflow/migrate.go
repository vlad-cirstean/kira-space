package adeflow

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/renameio/v2"
	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// backupDir is the folder of the pre-library workflow files under the workflows dir. Its existence is
// the marker that the one-time migration ran. Folder never lists dot entries.
const backupDir = ".p526-backup"

// MigrateInlineSteps moves the inline steps of every existing workflow into the step library once. It
// copies the workflow files to <workflows>/.p526-backup first. A step becomes a library file named
// after its id (`-2`, `-3` ... when another body already holds the id) and the workflow item becomes
// a reference, so the resolved workflow stays identical. A workflow that does not parse stays untouched.
// With the backup folder present it does nothing.
func (r *Reader) MigrateInlineSteps() error {
	if r.Steps == nil {
		return nil
	}
	backup := filepath.Join(r.Dir, backupDir)
	if _, err := os.Stat(backup); err == nil {
		return nil
	}
	names, err := r.workflowFiles()
	if err != nil {
		return err
	}
	if err := backupFiles(r.Dir, backup, names); err != nil {
		return err
	}
	r.wmu.Lock()
	defer r.wmu.Unlock()
	m := &migration{r: r, taken: r.existingLibrary()}
	for _, name := range names {
		if err := m.workflow(name); err != nil {
			slog.Warn("ade workflows: step library migration skipped a workflow", "scope", "ade", "file", name, "err", err)
		}
	}
	return nil
}

// workflowFiles lists the workflow file names, sorted. A missing dir is an empty list.
func (r *Reader) workflowFiles() ([]string, error) {
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("adeflow: read %s: %w", r.Dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() && fileNamePattern.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// backupFiles copies names into backup. The copy lands in a temporary folder renamed at the end, so a
// crash never leaves a partial backup that reads as the marker.
func backupFiles(dir, backup string, names []string) error {
	tmp := backup + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("adeflow: clear %s: %w", tmp, err)
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return fmt.Errorf("adeflow: create %s: %w", tmp, err)
	}
	for _, name := range names {
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("adeflow: backup %s: %w", name, err)
		}
		if err := renameio.WriteFile(filepath.Join(tmp, name), src, 0o644); err != nil {
			return fmt.Errorf("adeflow: backup %s: %w", name, err)
		}
	}
	if err := os.Rename(tmp, backup); err != nil {
		return fmt.Errorf("adeflow: finish backup: %w", err)
	}
	return nil
}

// existingLibrary maps the library files already on disk to their canonical content.
func (r *Reader) existingLibrary() map[string]string {
	out := map[string]string{}
	for _, e := range r.Steps.folder().List() {
		id := strings.TrimSuffix(e.File, ".yaml")
		if e.Value != nil {
			out[id] = canonical(*e.Value)
		} else {
			out[id] = ""
		}
	}
	return out
}

// canonical is the comparison form of a library step.
func canonical(st adewire.WorkflowStep) string {
	js, _ := json.Marshal(libraryForm(st))
	return string(js)
}

type migration struct {
	r     *Reader
	taken map[string]string // library id -> canonical content
}

// workflow migrates one file; an error leaves the file as it was.
func (m *migration) workflow(name string) error {
	p := filepath.Join(m.r.Dir, name)
	src, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	wf, werr := Parse(src)
	if werr != nil {
		return fmt.Errorf("does not parse (line %d): %s", werr.Line, werr.Message)
	}
	before, err := resolvedJSON(wf)
	if err != nil {
		return err
	}
	refs := make([]adewire.WorkflowStep, len(wf.Steps))
	copy(refs, wf.Steps)
	created := map[string]adewire.WorkflowStep{}
	for i, st := range wf.Steps {
		libID := m.libraryID(st)
		if _, known := created[libID]; !known && m.taken[libID] == "" {
			lib := libraryForm(st)
			lib.ID = libID
			created[libID] = lib
			m.taken[libID] = canonical(lib)
		}
		refs[i].Use = libID
	}
	var written []string
	// rollback removes the library files this workflow added, so a skipped workflow leaves no orphans
	// and later workflows cannot reference them.
	rollback := func() {
		for _, id := range written {
			_ = m.r.Steps.Delete(id + ".yaml")
		}
		for id := range created {
			delete(m.taken, id)
		}
	}
	for id, st := range created {
		if err := m.r.Steps.create(id, st); err != nil {
			rollback()
			return err
		}
		written = append(written, id)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		rollback()
		return err
	}
	wf.Steps = refs
	out, err := encodeWorkflow(&doc, wf, m.r.Steps)
	if err != nil {
		rollback()
		return err
	}
	after, err := parseResolved(out, m.r)
	if err != nil || after != before {
		rollback()
		return fmt.Errorf("the migrated file does not resolve to the original workflow: %v", err)
	}
	if err := configfs.WriteAtomic(p, out, 0o644); err != nil {
		rollback()
		return err
	}
	return nil
}

// libraryID picks the library id for a step: its own id when free or already holding the same body,
// else the first free id with a numeric suffix.
func (m *migration) libraryID(st adewire.WorkflowStep) string {
	lib := libraryForm(st)
	for n := 1; ; n++ {
		id := st.ID
		if n > 1 {
			id = fmt.Sprintf("%s-%d", st.ID, n)
		}
		lib.ID = id
		have, used := m.taken[id]
		if !used || have == canonical(lib) {
			return id
		}
	}
}

// SnapshotJSON is the canonical JSON of wf as hashed and stored on a task: the library reference is
// cleared, so moving a step into the library changes nothing a started task compares.
func SnapshotJSON(wf adewire.Workflow) ([]byte, error) {
	steps := make([]adewire.WorkflowStep, len(wf.Steps))
	for i, s := range wf.Steps {
		s.Use = ""
		steps[i] = s
	}
	wf.Steps = steps
	return json.Marshal(wf)
}

func resolvedJSON(wf adewire.Workflow) (string, error) {
	js, err := SnapshotJSON(wf)
	return string(js), err
}

func parseResolved(src []byte, r *Reader) (string, error) {
	wf, werr := ParseWith(src, r.lookup())
	if werr != nil {
		return "", fmt.Errorf("line %d: %s", werr.Line, werr.Message)
	}
	return resolvedJSON(wf)
}
