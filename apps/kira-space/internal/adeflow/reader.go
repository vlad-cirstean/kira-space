package adeflow

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// WorkflowsFolder is the workflows module folder name under <home>/config.
const WorkflowsFolder = "workflows"

// Reader lists and resolves workflows over a configfs.Folder. Only the writer methods (writer.go)
// create the directory or write a workflow file, and WriteDocs.
type Reader struct {
	Dir string
	// Store persists the last valid workflow per file so a file broken on disk keeps its previous
	// version in use across restarts (a configfs.TableProjection over WorkflowsFolder).
	Store configfs.Projection
	Now   func() time.Time
	// Steps is the step library that `step:` items resolve against; nil refuses every reference.
	Steps *StepLibrary
	// Journal is the file a rename records its plan in so a crash can finish it (RecoverRename). It lives
	// outside the config folder; empty disables journaling.
	Journal string

	wmu  sync.Mutex // serializes the writer methods
	once sync.Once
	f    *configfs.Folder[adewire.Workflow]

	gmu  sync.Mutex // guards gets
	gets map[string]getMemo
}

// getMemo is one valid workflow Get resolved, with the files it was resolved from.
type getMemo struct {
	tok getToken
	wf  adewire.Workflow
}

// getToken identifies the inputs of a Get: the workflow file and the step library its `step:` items
// resolve against (newest mtime and total size over the library's files).
type getToken struct {
	wfMtime, wfSize       int64
	stepsMtime, stepsSize int64
	stepsFiles            int
}

func (r *Reader) folder() *configfs.Folder[adewire.Workflow] {
	r.once.Do(func() {
		r.f = &configfs.Folder[adewire.Workflow]{
			Dir:        r.Dir,
			Ext:        ".yaml",
			Name:       fileNamePattern,
			Decode:     r.decode,
			Projection: r.Store,
			Now:        r.Now,
		}
	})
	return r.f
}

// lookup is the StepLookup of the Reader's library; nil without one.
func (r *Reader) lookup() StepLookup { return r.Steps.lookup() }

func (r *Reader) decode(file string, src []byte) (adewire.Workflow, *configfs.Problem) {
	return decodeWorkflow(file, src, r.lookup())
}

func decodeWorkflow(file string, src []byte, lookup StepLookup) (adewire.Workflow, *configfs.Problem) {
	wf, werr := ParseWith(src, lookup)
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
	tok, tokOK := r.getToken(id)
	if tokOK {
		r.gmu.Lock()
		m, hit := r.gets[id]
		r.gmu.Unlock()
		if hit && m.tok == tok {
			return cloneWorkflow(m.wf), true
		}
	}
	if e, ok := r.folder().Get(id + ".yaml"); ok && e.Value != nil && e.Value.ID == id {
		// Only a file that parses now is memoised: a broken file answers with its last good version,
		// which must keep re-reading the file so the fix is seen.
		if tokOK && e.Problem == nil {
			r.gmu.Lock()
			if r.gets == nil {
				r.gets = map[string]getMemo{}
			}
			r.gets[id] = getMemo{tok: tok, wf: *e.Value}
			r.gmu.Unlock()
			return cloneWorkflow(*e.Value), true
		}
		return *e.Value, true
	}
	return adewire.Workflow{}, false
}

// cloneWorkflow lets a caller reorder or replace steps without touching the memo. Map and slice
// fields inside a step stay shared; callers read them.
func cloneWorkflow(wf adewire.Workflow) adewire.Workflow {
	wf.Steps = slices.Clone(wf.Steps)
	return wf
}

// getToken stats the workflow file and lists the step library once. ok is false when the file is
// missing or unreadable: Get then takes the uncached path.
func (r *Reader) getToken(id string) (getToken, bool) {
	fi, err := os.Stat(filepath.Join(r.Dir, id+".yaml"))
	if err != nil {
		return getToken{}, false
	}
	tok := getToken{wfMtime: fi.ModTime().UnixNano(), wfSize: fi.Size()}
	if r.Steps == nil || r.Steps.Dir == "" {
		return tok, true
	}
	ents, err := os.ReadDir(r.Steps.Dir)
	if err != nil {
		return tok, os.IsNotExist(err)
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return getToken{}, false
		}
		tok.stepsFiles++
		tok.stepsSize += info.Size()
		tok.stepsMtime = max(tok.stepsMtime, info.ModTime().UnixNano())
	}
	return tok, true
}

// WriteDocs writes the folder's AGENTS.md, creating the folder when missing.
func (r *Reader) WriteDocs() error { return r.folder().WriteDocs(DocModel()) }

// Validate parses src as the Reader does, resolving library references.
func (r *Reader) Validate(src string) adewire.WorkflowValidation {
	wf, werr := ParseWith([]byte(src), r.lookup())
	if werr != nil {
		return adewire.WorkflowValidation{Error: werr}
	}
	return adewire.WorkflowValidation{Workflow: &wf}
}
