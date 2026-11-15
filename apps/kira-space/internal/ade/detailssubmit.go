package ade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles/store"
)

const detailsEditKind = approvals.KindDetailsEdit

func detailsItemID(taskID string) string { return detailsEditKind + ":" + taskID }

// detailsEvents routes a submit's pending files into the approvals queue.
type detailsEvents struct{ b *TaskBoard }

func (e detailsEvents) Pending(_ context.Context, p taskfiles.PendingSubmit) error {
	e.b.files.addPending(e.b, p)
	return nil
}

// Accepted needs no action in this part: a silent accept has no notification yet.
func (detailsEvents) Accepted(context.Context, string, string, taskfiles.Notify) error { return nil }

type detailsItemFile struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Cause     string `json:"cause"`
	Text      string `json:"text"`
	Reason    string `json:"reason"`
	Collision string `json:"collision,omitempty"`
}

type detailsPayload struct {
	TaskID    string            `json:"taskId"`
	TaskTitle string            `json:"taskTitle"`
	Reason    string            `json:"reason"`
	Items     []detailsItemFile `json:"items"`
}

// addPending merges the files of one submit into the task's one open item.
func (df *detailsFiles) addPending(b *TaskBoard, p taskfiles.PendingSubmit) {
	df.mu.Lock()
	m := df.pending[p.TaskID]
	if m == nil {
		m = map[string]detailsPending{}
		df.pending[p.TaskID] = m
	}
	for _, it := range p.Items {
		m[it.Path] = detailsPending{item: it, reason: p.Reason, by: string(p.Session.Role)}
	}
	df.mu.Unlock()
	df.putItem(b, p.TaskID)
}

func (df *detailsFiles) putItem(b *TaskBoard, taskID string) {
	if b.deps.Approvals == nil {
		return
	}
	df.mu.Lock()
	m := df.pending[taskID]
	paths := make([]string, 0, len(m))
	for p := range m {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	pl := detailsPayload{TaskID: taskID}
	var last string
	for _, path := range paths {
		d := m[path]
		var texts []string
		for _, c := range d.item.Changes {
			if c.Text != "" {
				texts = append(texts, c.Text)
			}
		}
		f := detailsItemFile{Path: path, Kind: string(d.item.Kind), Cause: string(d.item.Cause), Text: strings.Join(texts, "\n"), Reason: d.reason}
		if c := d.item.Collision; c != nil {
			f.Collision = c.By
		}
		pl.Items = append(pl.Items, f)
		pl.Reason, last = d.reason, d.by
	}
	df.mu.Unlock()
	if len(paths) == 0 {
		b.deps.Approvals.Remove(detailsItemID(taskID))
		return
	}
	pl.TaskTitle = b.titleOf(taskID)
	raw, err := json.Marshal(pl)
	if err != nil {
		panic("ade: details approval payload: " + err.Error())
	}
	b.deps.Approvals.Put(approvals.Item{
		ID: detailsItemID(taskID), Kind: detailsEditKind, CreatedAt: b.deps.Now().UnixMilli(),
		Origin:    approvals.Origin{Task: pl.TaskTitle, Source: last},
		Target:    approvals.Target{Name: pl.TaskTitle, Kind: "task"},
		Operation: approvals.Operation{Class: "review", Tone: "caution"},
		Reason:    "review", Payload: raw,
		Title: i18n.T("backend.prompt.detailsEdit.title", map[string]string{"task": pl.TaskTitle}),
	})
}

func (b *TaskBoard) titleOf(taskID string) string {
	t, err := b.deps.Tasks.GetTask(taskID)
	if err != nil {
		return taskID
	}
	br, _ := b.deps.Tasks.BranchesOfTask(taskID)
	return taskTitle(t, br)
}

// detailsOwner answers details.edit items: the ref is the task id.
type detailsOwner struct{ b *TaskBoard }

// Answer approves or denies the pending files. Options are {approved: [paths]}; none approves every file.
// A file left out of approved, and every file of a deny, is restored from the accepted copy.
func (o detailsOwner) Answer(taskID string, a approvals.Answer) error {
	var opts struct {
		Approved []string `json:"approved"`
	}
	if len(a.Options) > 0 {
		if err := json.Unmarshal(a.Options, &opts); err != nil {
			return ipcerr.Contract("details edit options: %v", err)
		}
	}
	df := o.b.files
	if df == nil {
		return ipcerr.NotFoundNamed("approval")
	}
	df.mu.Lock()
	m := df.pending[taskID]
	paths := make([]string, 0, len(m))
	for p := range m {
		paths = append(paths, p)
	}
	df.mu.Unlock()
	if len(paths) == 0 {
		return ipcerr.NotFoundNamed("approval")
	}
	sort.Strings(paths)
	mu := o.b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	ctx := withDetailsSession(o.b.ctxWithTask(o.b.ctx, taskID), detailsSession{role: taskfiles.RoleUser})
	var errs []error
	for _, p := range paths {
		var err error
		if a.Approve && (opts.Approved == nil || slices.Contains(opts.Approved, p)) {
			err = df.svc.Adopt(ctx, taskID, p, string(taskfiles.RoleUser))
		} else {
			err = df.svc.Restore(ctx, taskID, p)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			continue
		}
		df.mu.Lock()
		delete(df.pending[taskID], p)
		df.mu.Unlock()
	}
	df.putItem(o.b, taskID)
	o.b.mirrorTask(taskID, userMirror)
	o.b.notifyBoardNow(taskChanged(taskID))
	return errors.Join(errs...)
}

// SubmitFiles is the submit tool: it walks the named paths (all when empty) of the task folder as the
// calling session and returns every file's outcome.
func (b *TaskBoard) SubmitFiles(ctx context.Context, taskID string, s detailsSession, paths []string, reason string) (taskfiles.Result, error) {
	if b.files == nil {
		return taskfiles.Result{}, invalid("task files are off")
	}
	df := b.files
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return taskfiles.Result{}, err
	}
	df.mu.Lock()
	last := df.lastSync[s.key]
	df.mu.Unlock()
	call := b.taskCallOf(tc)
	ctx = withDetailsSession(withTaskCall(ctx, call), s)
	res, err := df.svc.Submit(ctx, taskfiles.Request{
		TaskID: taskID, Paths: paths, Reason: reason,
		Session: taskfiles.Session{Key: s.key, Role: s.role, LastSync: last, Step: fileSlug(call.step)},
	})
	if err != nil {
		return res, err
	}
	b.mirrorTask(taskID, mirrorOpts{})
	// After the mirror: Kira's own re-render of what the agent just submitted is not a collision for it.
	df.mu.Lock()
	df.lastSync[s.key] = b.deps.Now()
	df.mu.Unlock()
	b.notifyBoardNow(taskChanged(taskID))
	return res, nil
}

// MarkSynced records a session's launch: later submits compare collisions against it.
func (b *TaskBoard) markSynced(key string) {
	if b.files == nil || key == "" {
		return
	}
	b.files.mu.Lock()
	b.files.lastSync[key] = b.deps.Now()
	b.files.mu.Unlock()
}

// DiskChanges lists the files of the task folder that differ from what the app accepted, with the diff
// of each text file. It stats; only a changed file is read.
func (b *TaskBoard) DiskChanges(ctx context.Context, taskID string) ([]adewire.DiskChange, error) {
	df := b.files
	if df == nil {
		return nil, nil
	}
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	paths, err := df.svc.Changed(b.ctxWithTask(ctx, taskID), taskID)
	if err != nil {
		return nil, err
	}
	st, err := df.open(taskID)
	if err != nil {
		return nil, err
	}
	rows, err := df.idx.Files(taskID)
	if err != nil {
		return nil, err
	}
	byRel := map[string]string{}
	pend := map[string]bool{}
	for _, r := range rows {
		byRel[r.RelPath] = r.Problem
		pend[r.RelPath] = r.Review == "pending"
	}
	h := &detailsHandlers{b: b, df: df}
	out := make([]adewire.DiskChange, 0, len(paths))
	for _, p := range paths {
		c := taskfiles.Classify(p)
		if c.Kind == taskfiles.KindIgnored {
			continue
		}
		dc := adewire.DiskChange{Path: p, Kind: string(c.Kind), Pending: pend[p], Problem: byRel[p]}
		raw, _, rerr := st.Read(ctx, p, taskfiles.MaxBytes(c.Kind, p))
		var accepted []byte
		_ = df.idx.InTx(func(tx taskfiles.Tx) error {
			accepted, _, _ = h.Render(ctx, tx, taskID, p, c)
			return nil
		})
		switch {
		case errors.Is(rerr, store.ErrNotExist):
			dc.State = "missing"
			dc.Diff = taskfiles.TextDiff(p, string(accepted), "")
		case rerr != nil:
			dc.State, dc.Problem = "edited", rerr.Error()
		case len(accepted) == 0:
			dc.State, dc.Diff = "new", taskfiles.TextDiff(p, "", string(raw))
		default:
			dc.State, dc.Diff = "edited", taskfiles.TextDiff(p, string(accepted), string(raw))
		}
		out = append(out, dc)
	}
	return out, nil
}

// AcceptDisk makes the named changes the accepted copy (Kira-written files are restored instead: only
// Kira writes them). Empty paths means every change.
func (b *TaskBoard) AcceptDisk(ctx context.Context, taskID string, paths []string) error {
	return b.resolveDisk(ctx, taskID, paths, true)
}

// DiscardDisk restores the named files from the accepted copy.
func (b *TaskBoard) DiscardDisk(ctx context.Context, taskID string, paths []string) error {
	return b.resolveDisk(ctx, taskID, paths, false)
}

func (b *TaskBoard) resolveDisk(ctx context.Context, taskID string, paths []string, accept bool) error {
	df := b.files
	if df == nil {
		return nil
	}
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	if len(paths) == 0 {
		var err error
		if paths, err = df.svc.Changed(ctx, taskID); err != nil {
			return err
		}
	}
	ctx = withDetailsSession(b.ctxWithTask(ctx, taskID), detailsSession{role: taskfiles.RoleUser})
	var errs []error
	for _, p := range paths {
		c := taskfiles.Classify(p)
		if c.Kind == taskfiles.KindIgnored {
			continue
		}
		var err error
		if accept && !c.Kind.KiraOwned() {
			err = df.svc.Adopt(ctx, taskID, p, string(taskfiles.RoleUser))
		} else {
			err = df.svc.Restore(ctx, taskID, p)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			continue
		}
		df.mu.Lock()
		delete(df.pending[taskID], p)
		df.mu.Unlock()
	}
	df.putItem(b, taskID)
	b.mirrorTask(taskID, userMirror)
	b.notifyBoardNow(taskChanged(taskID))
	return errors.Join(errs...)
}

// TaskFolder is the absolute task folder, created on demand; "" when files are off.
func (b *TaskBoard) TaskFolder(taskID string) (string, error) {
	if b.files == nil {
		return "", nil
	}
	st, err := b.files.open(taskID)
	if err != nil {
		return "", err
	}
	return st.(*store.Local).Root(), nil
}
