package adeflow

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Store persists the last valid workflow per file so a file broken on disk keeps its previous
// version in use across restarts. It is the workflows Projection (docs/ARCHITECTURE.md, Config files).
type Store interface {
	LastValid() (map[string]string, error)
	RecordLastValid(file, workflowJSON string, now int64) error
	ForgetLastValid(file string) error
}

// Dir is the workflows directory under the app home.
func Dir(home string) string { return filepath.Join(home, "workflows") }

// Reader lists and resolves workflows over a configfs.Folder. Only the writer methods (writer.go)
// create the directory or write a workflow file.
type Reader struct {
	Dir   string
	Store Store
	Now   func() time.Time

	wmu  sync.Mutex // serializes the writer methods
	once sync.Once
	f    *configfs.Folder[adewire.Workflow]
}

type projection struct{ s Store }

func (p projection) Snapshot() (map[string]string, error) { return p.s.LastValid() }
func (p projection) Record(file, js string, now int64) error {
	return p.s.RecordLastValid(file, js, now)
}
func (p projection) Forget(file string) error { return p.s.ForgetLastValid(file) }

func (r *Reader) folder() *configfs.Folder[adewire.Workflow] {
	r.once.Do(func() {
		r.f = &configfs.Folder[adewire.Workflow]{
			Dir:        r.Dir,
			Ext:        ".yaml",
			Name:       fileNamePattern,
			Decode:     decodeWorkflow,
			Projection: projection{r.Store},
			Now:        r.Now,
		}
	})
	return r.f
}

func decodeWorkflow(file string, src []byte) (adewire.Workflow, *configfs.Problem) {
	wf, werr := Parse(src)
	if werr == nil && wf.ID != strings.TrimSuffix(file, ".yaml") {
		werr = &adewire.WorkflowError{Message: i18n.T("backend.space.ade.workflowIdMismatch.body", nil)}
	}
	if werr != nil {
		return adewire.Workflow{}, &configfs.Problem{Line: werr.Line, Message: werr.Message}
	}
	return wf, nil
}

func toEntry(e configfs.Entry[adewire.Workflow]) adewire.WorkflowEntry {
	out := adewire.WorkflowEntry{FileName: e.File, Path: e.Path, Hash: e.Hash, Workflow: e.Value}
	if e.Problem != nil {
		out.Error = &adewire.WorkflowError{Line: e.Problem.Line, Message: e.Problem.Message}
	}
	return out
}

// Watch calls onChange (debounced) when a workflow file changes. stop ends the watch.
func (r *Reader) Watch(onChange func()) (stop func(), err error) {
	return r.folder().Watch(onChange)
}

// List reads every *.yaml file, sorted by name. usedBy reports how many live tasks use a
// workflow id (nil = 0). A missing or empty directory is a valid empty list.
func (r *Reader) List(usedBy func(id string) int) adewire.WorkflowsResult {
	out := adewire.WorkflowsResult{Dir: r.Dir, Workflows: make([]adewire.WorkflowEntry, 0)}
	for _, e := range r.folder().List() {
		entry := toEntry(e)
		if entry.Workflow != nil && usedBy != nil {
			entry.UsedBy = usedBy(entry.Workflow.ID)
		}
		out.Workflows = append(out.Workflows, entry)
	}
	return out
}

// Get returns the effective workflow with the given id: the file's current version when valid,
// else the last valid one. ok is false when no file defines it. Only <id>.yaml is read: a workflow's
// id always equals its file name.
func (r *Reader) Get(id string) (adewire.Workflow, bool) {
	if id == "" {
		return adewire.Workflow{}, false
	}
	if e, ok := r.folder().Get(id + ".yaml"); ok && e.Value != nil && e.Value.ID == id {
		return *e.Value, true
	}
	return adewire.Workflow{}, false
}
