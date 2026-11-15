package ade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/leases"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/kiratime"
)

// statefile.go: state.yaml and task.yaml, the Kira-written read-only mirrors of a task (P643 Part 5).
// One writer goroutine per task coalesces triggers, skips a rewrite whose body is unchanged and bumps
// rev only on a real change. The rendered bytes are cached so a restore of an edited file needs no
// render inside the taskfiles transaction.

const (
	stateCoalesce   = 150 * time.Millisecond
	stateIdle       = 30 * time.Minute
	maxStateAnswers = 10
	maxStateRuns    = 3
	maxRunOutcome   = 2 << 10
)

// Answer statuses: the 9.4 words a late result or user answer can carry.
const (
	answerDone   = "done"
	answerDenied = "denied"
	answerFailed = "failed"
)

// stateAnswer is one user answer or late result for a call whose wait ended.
type stateAnswer struct {
	ID      string `yaml:"id"`
	Kind    string `yaml:"kind,omitempty"`
	Subject string `yaml:"subject,omitempty"`
	Status  string `yaml:"status"`
	Message string `yaml:"note,omitempty"`
	At      string `yaml:"at"`
}

func answerOfBranch(rb agentmcp.RequestedBranch) stateAnswer {
	id := rb.Repo + "/" + rb.Branch
	a := stateAnswer{ID: id, Kind: "branch.request", Subject: id, Status: answerDone, Message: rb.Next}
	switch rb.Outcome {
	case agentmcp.BranchDenied:
		a.Status = answerDenied
	case agentmcp.BranchFailed:
		a.Status = answerFailed
	}
	return a
}

// answerOfOutcome maps a rename or change outcome: ok is the success word, denied and failed reuse
// the branch.request words.
func answerOfOutcome(kind, id, outcome, ok, next string) stateAnswer {
	a := stateAnswer{ID: id, Kind: kind, Subject: id, Status: answerDone, Message: next}
	switch outcome {
	case agentmcp.BranchDenied:
		a.Status = answerDenied
	case ok:
	default:
		a.Status = answerFailed
	}
	return a
}

type stateStep struct {
	ID       string `yaml:"id"`
	Name     string `yaml:"name,omitempty"`
	Position int    `yaml:"position,omitempty"`
	Of       int    `yaml:"of,omitempty"`
}

type stateTaskInfo struct {
	ID       string    `yaml:"id"`
	Kind     string    `yaml:"kind"`
	Status   string    `yaml:"status"`
	Step     stateStep `yaml:"step"`
	Workflow string    `yaml:"workflow,omitempty"`
}

type stateSetup struct {
	Status string `yaml:"status"`
	Note   string `yaml:"note,omitempty"`
}

type stateRepo struct {
	Repo     string                     `yaml:"repo"`
	Branch   string                     `yaml:"branch,omitempty"`
	Base     string                     `yaml:"base,omitempty"`
	Worktree string                     `yaml:"worktree,omitempty"`
	Setup    *stateSetup                `yaml:"setup,omitempty"`
	Mismatch *agentmcp.WorktreeMismatch `yaml:"mismatch,omitempty"`
}

type stateRegistered struct {
	Name     string `yaml:"name"`
	Nickname string `yaml:"nickname,omitempty"`
	Path     string `yaml:"path"`
}

type stateLease struct {
	Queue    string `yaml:"queue"`
	Status   string `yaml:"status"`
	Position int    `yaml:"position,omitempty"`
	Holder   string `yaml:"holder,omitempty"`
}

type stateOp struct {
	OpID      string `yaml:"opId"`
	Kind      string `yaml:"kind"`
	Subject   string `yaml:"subject,omitempty"`
	Status    string `yaml:"status"`
	StartedAt string `yaml:"startedAt"`
}

type stateRun struct {
	RunID   string `yaml:"runId"`
	Kind    string `yaml:"kind"`
	Step    string `yaml:"step,omitempty"`
	Status  string `yaml:"status"`
	Outcome string `yaml:"outcome,omitempty"`
}

type stateParent struct {
	ID    string `yaml:"id"`
	Title string `yaml:"title"`
	Path  string `yaml:"path"`
}

type stateChild struct {
	ID           string `yaml:"id"`
	Title        string `yaml:"title"`
	Status       string `yaml:"status"`
	Step         string `yaml:"step,omitempty"`
	Finished     bool   `yaml:"finished"`
	Archived     bool   `yaml:"archived"`
	LastHandover string `yaml:"lastHandover,omitempty"`
	Preview      string `yaml:"preview,omitempty"`
}

type stateChain struct {
	Parent   *stateParent `yaml:"parent,omitempty"`
	Children []stateChild `yaml:"children"`
}

type statePending struct {
	ID      string `yaml:"id"`
	Kind    string `yaml:"kind"`
	Subject string `yaml:"subject"`
	Since   string `yaml:"since,omitempty"`
}

type stateDoc struct {
	Rev        int               `yaml:"rev"`
	Task       stateTaskInfo     `yaml:"task"`
	Repos      []stateRepo       `yaml:"repos"`
	Registered []stateRegistered `yaml:"registered"`
	Leases     []stateLease      `yaml:"leases"`
	Operations []stateOp         `yaml:"operations"`
	Runs       []stateRun        `yaml:"runs"`
	Chain      *stateChain       `yaml:"chain,omitempty"`
	Pending    []statePending    `yaml:"pending"`
	Answers    []stateAnswer     `yaml:"answers"`
}

// stateWriter is the writer of one task's state.yaml.
type stateWriter struct {
	mu    sync.Mutex    // serializes the writes of one task
	dirty chan struct{} // buffered 1
	// seen is the last agent-side trigger; allChanged does not move it, so an untouched task goes quiet.
	seen   time.Time
	rev    int
	loaded bool
	last   []byte // the last written body, rev line as 0
	// archived is set under mu by the final archive write: later renders of this writer write nothing.
	archived bool
}

type stateFiles struct {
	mu      sync.Mutex
	writers map[string]*stateWriter
	answers map[string][]stateAnswer
	// kira holds the last rendered bytes of the Kira-owned files, by task and path: a restore of an
	// edited one returns them.
	kira map[string]map[string][]byte
	// attach is the attach cache the writers share until the next board notification: one inventory
	// read per repo serves every writer that renders for the same change.
	attach *attachCache
}

var revLine = regexp.MustCompile(`(?m)^rev: (\d+)$`)

// recordAnswer keeps a late result in state.yaml answers; a session also gets a push, since no call
// waits for it.
func (b *TaskBoard) recordAnswer(call agentmcp.SpaceCall, a stateAnswer) {
	if a.At == "" {
		a.At = kiratime.FormatISO(b.deps.Now())
	}
	s := &b.stateFiles
	s.mu.Lock()
	if s.answers == nil {
		s.answers = map[string][]stateAnswer{}
	}
	list := slices.Concat(s.answers[call.TaskID], []stateAnswer{a})
	if len(list) > maxStateAnswers {
		list = list[len(list)-maxStateAnswers:]
	}
	s.answers[call.TaskID] = list
	s.mu.Unlock()
	if call.RunID == "" && call.SessionID != "" {
		msg := "Kira: " + cmpNonEmpty(a.Kind, "call") + " " + a.Subject + " is " + a.Status + "."
		if a.Message != "" {
			msg += " " + a.Message
		}
		b.deliverNotice(leases.Key(call.SessionID), msg)
	}
	b.markStateDirty(call.TaskID, true)
}

// markStateDirty schedules a rewrite. touch marks agent-side activity, which keeps the writer alive;
// a board-wide change only reaches writers that are alive.
func (b *TaskBoard) markStateDirty(taskID string, touch bool) {
	if b.files == nil || taskID == "" {
		return
	}
	s := &b.stateFiles
	s.mu.Lock()
	s.attach = nil // facts read before this change must not serve a render after it
	w := s.writers[taskID]
	if w == nil {
		if !touch {
			s.mu.Unlock()
			return
		}
		if s.writers == nil {
			s.writers = map[string]*stateWriter{}
		}
		w = &stateWriter{dirty: make(chan struct{}, 1), seen: b.deps.Now()}
		s.writers[taskID] = w
		if !b.track() {
			delete(s.writers, taskID)
			s.mu.Unlock()
			return
		}
		go func() {
			defer b.wg.Done()
			b.runStateWriter(taskID, w)
		}()
	}
	if touch {
		w.seen = b.deps.Now()
	}
	s.mu.Unlock()
	select {
	case w.dirty <- struct{}{}:
	default:
	}
}

// markStateFor is the board trigger: the named tasks' writers, or every live writer for a change that
// names none.
func (b *TaskBoard) markStateFor(ch adewire.BoardChangedEvent) {
	if len(ch.TaskIDs) == 0 {
		b.markAllStateDirty()
		return
	}
	for _, id := range ch.TaskIDs {
		b.markStateDirty(id, false)
	}
}

// markAllStateDirty is the board-wide trigger: every live writer rewrites.
func (b *TaskBoard) markAllStateDirty() {
	s := &b.stateFiles
	s.mu.Lock()
	ids := make([]string, 0, len(s.writers))
	for id := range s.writers {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		b.markStateDirty(id, false)
	}
}

// agentLive reports whether the task has a running session or run. On a lookup error it answers true:
// a writer kept too long costs one idle render, a dropped one a stale file.
func (b *TaskBoard) agentLive(taskID string) bool {
	if b.deps.Sessions == nil {
		return false
	}
	rows, err := b.deps.Sessions.ListRunning(taskID)
	return err != nil || len(rows) > 0
}

func (b *TaskBoard) runStateWriter(taskID string, w *stateWriter) {
	s := &b.stateFiles
	idle := time.NewTimer(stateIdle)
	defer idle.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-w.dirty:
			select {
			case <-b.ctx.Done():
				return
			case <-time.After(stateCoalesce):
			}
			select {
			case <-w.dirty:
			default:
			}
			b.writeState(taskID, w)
		case <-idle.C:
			s.mu.Lock()
			if s.writers[taskID] != w { // archived: the writer was dropped
				s.mu.Unlock()
				return
			}
			if quiet := b.deps.Now().Sub(w.seen); quiet < stateIdle || len(w.dirty) > 0 {
				s.mu.Unlock()
				idle.Reset(max(stateIdle-quiet, stateCoalesce))
				continue
			}
			s.mu.Unlock()
			// A live session or run reads state.yaml with no call that touches it: keep the writer.
			if b.agentLive(taskID) {
				idle.Reset(stateIdle)
				continue
			}
			s.mu.Lock()
			if len(w.dirty) > 0 || b.deps.Now().Sub(w.seen) < stateIdle {
				s.mu.Unlock()
				idle.Reset(stateCoalesce)
				continue
			}
			if s.writers[taskID] == w {
				delete(s.writers, taskID)
			}
			s.mu.Unlock()
			return
		}
	}
}

// writeStateNow writes state.yaml before it returns, so the file exists when an agent starts, and keeps
// the task's writer alive.
func (b *TaskBoard) writeStateNow(taskID string) {
	if b.files == nil {
		return
	}
	b.markStateDirty(taskID, true)
	w := b.stateWriterOf(taskID)
	if w == nil {
		return
	}
	b.writeState(taskID, w)
}

func (b *TaskBoard) stateWriterOf(taskID string) *stateWriter {
	s := &b.stateFiles
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writers[taskID]
}

// writeState renders and writes the file when its body changed. Calls for one task are serialized by
// the writer's own lock.
func (b *TaskBoard) writeState(taskID string, w *stateWriter) {
	df := b.files
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := b.writeStateLocked(df, taskID, w); err != nil && !errors.Is(err, context.Canceled) {
		slog.Warn("ade: write state.yaml", "scope", "ade", "task", taskID, "err", err)
	}
}

func (b *TaskBoard) writeStateLocked(df *detailsFiles, taskID string, w *stateWriter) error {
	if w.archived {
		return nil
	}
	doc, err := b.renderState(b.ctx, taskID)
	if err != nil {
		return nil // archived or gone: the final write at archive stands
	}
	b.stateFiles.mu.Lock()
	doc.Answers = slices.Clone(b.stateFiles.answers[taskID])
	b.stateFiles.mu.Unlock()
	if doc.Answers == nil {
		doc.Answers = []stateAnswer{}
	}
	body, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	if !w.loaded {
		w.loaded = true
		w.rev = df.diskRev(b.ctx, taskID)
	}
	if bytes.Equal(body, w.last) {
		return nil
	}
	w.rev++
	doc.Rev = w.rev
	full, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	data := append([]byte(taskfiles.Header(taskfiles.KindState)), full...)
	if err := df.writeKira(b.ctx, taskID, taskfiles.StateFile, taskfiles.KindState, data); err != nil {
		w.rev--
		return err
	}
	w.last = body
	return nil
}

// writeArchivedState is the one final write at archive: the task is no longer live.
func (b *TaskBoard) writeArchivedState(taskID string) {
	if b.files == nil {
		return
	}
	t, err := b.deps.Tasks.GetTask(taskID)
	if err != nil {
		return
	}
	s := &b.stateFiles
	s.mu.Lock()
	w := s.writers[taskID]
	delete(s.writers, taskID)
	s.mu.Unlock()
	rev := 0
	if w != nil {
		// Wait out a live render in flight so its rename cannot land after the archived file.
		w.mu.Lock()
		defer w.mu.Unlock()
		w.archived = true
		rev = w.rev
	}
	if rev == 0 {
		rev = b.files.diskRev(b.ctx, taskID)
	}
	doc := stateDoc{Rev: rev + 1, Task: stateTaskInfo{ID: t.ID, Kind: t.Kind, Status: "archived", Step: stateStep{ID: t.EntryStepID}},
		Repos: []stateRepo{}, Registered: []stateRegistered{}, Leases: []stateLease{}, Operations: []stateOp{}, Runs: []stateRun{},
		Pending: []statePending{}, Answers: []stateAnswer{}}
	full, err := yaml.Marshal(doc)
	if err != nil {
		return
	}
	data := append([]byte(taskfiles.Header(taskfiles.KindState)), full...)
	if err := b.files.writeKira(b.ctx, taskID, taskfiles.StateFile, taskfiles.KindState, data); err != nil {
		slog.Warn("ade: write final state.yaml", "scope", "ade", "task", taskID, "err", err)
	}
	s.mu.Lock()
	delete(s.answers, taskID)
	delete(s.kira, taskID)
	s.mu.Unlock()
}

// diskRev is the rev of the file on disk, 0 when there is none; a restart continues the count.
func (df *detailsFiles) diskRev(ctx context.Context, taskID string) int {
	st, err := df.open(taskID)
	if err != nil {
		return 0
	}
	raw, _, err := st.Read(ctx, taskfiles.StateFile, taskfiles.MaxKiraFileBytes)
	if err != nil {
		return 0
	}
	m := revLine.FindSubmatch(raw)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(string(m[1]))
	return n
}

// writeKira writes one Kira-owned file through the accepted-hash path and remembers its bytes.
func (df *detailsFiles) writeKira(ctx context.Context, taskID, rel string, kind taskfiles.Kind, data []byte) error {
	df.setKira(taskID, rel, data)
	st, err := df.open(taskID)
	if err != nil {
		return err
	}
	rows, err := df.idx.Files(taskID)
	if err != nil {
		return err
	}
	var row model.TaskFile
	for _, r := range rows {
		if r.RelPath == rel {
			row = r
		}
	}
	return df.writeWanted(ctx, st, taskID, wantFile{rel: rel, kind: kind, data: data, force: true}, row, string(taskfiles.RoleKira))
}

func (df *detailsFiles) setKira(taskID, rel string, data []byte) {
	s := &df.b.stateFiles
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kira == nil {
		s.kira = map[string]map[string][]byte{}
	}
	if s.kira[taskID] == nil {
		s.kira[taskID] = map[string][]byte{}
	}
	s.kira[taskID][rel] = data
}

// kiraBytes is the last rendered content of a Kira-owned file; nil before the first write.
func (df *detailsFiles) kiraBytes(taskID, rel string) []byte {
	s := &df.b.stateFiles
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kira[taskID][rel]
}

// sharedAttach is the attach cache of the current notification epoch, created on first use.
func (b *TaskBoard) sharedAttach() *attachCache {
	s := &b.stateFiles
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attach == nil {
		s.attach = b.newAttachCache()
	}
	return s.attach
}

// renderState reads the task as state.yaml shows it. It runs outside every taskfiles transaction.
func (b *TaskBoard) renderState(ctx context.Context, taskID string) (stateDoc, error) {
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return stateDoc{}, err
	}
	info, err := b.spaceInfoWith(ctx, tc, true, b.sharedAttach())
	if err != nil {
		return stateDoc{}, err
	}
	statuses, err := b.deps.Statuses.List()
	if err != nil {
		return stateDoc{}, err
	}
	statusName := make(map[string]string, len(statuses))
	for _, st := range statuses {
		statusName[st.ID] = st.Name
	}
	doc := stateDoc{
		Task:  stateTaskInfo{ID: tc.task.ID, Kind: tc.task.Kind, Status: cmpNonEmpty(statusName[tc.task.StatusID], tc.task.StatusID), Workflow: info.Workflow},
		Repos: make([]stateRepo, 0, len(info.Repos)), Registered: make([]stateRegistered, 0, len(info.Registered)),
		Leases: []stateLease{}, Operations: []stateOp{}, Runs: []stateRun{},
	}
	doc.Task.Step = b.stateStepOf(tc)
	for i, r := range info.Repos {
		sr := stateRepo{Repo: r.Repo, Worktree: r.Worktree, Mismatch: r.Mismatch}
		if r.Branch != nil {
			sr.Branch = *r.Branch
		}
		if i < len(tc.branches) {
			sr.Base = tc.branches[i].Base
		}
		sr.Setup = setupOfInfo(r.Setup)
		doc.Repos = append(doc.Repos, sr)
	}
	for _, r := range info.Registered {
		doc.Registered = append(doc.Registered, stateRegistered{Name: r.Name, Nickname: r.Nickname, Path: r.Path})
	}
	doc.Leases = b.stateLeases(taskID)
	ops, _ := b.ListOperations(ctx, taskID)
	for _, o := range ops {
		doc.Operations = append(doc.Operations, stateOp{OpID: o.OpID, Kind: o.Kind, Subject: o.Subject, Status: "waiting", StartedAt: o.StartedAt})
	}
	if runs, err := b.recentRuns(taskID, maxStateRuns); err == nil {
		for _, r := range runs {
			doc.Runs = append(doc.Runs, stateRun{RunID: r.RunID, Kind: r.Kind, Step: r.Step, Status: runStatus(r.State), Outcome: outcomeText(r.Outcome)})
		}
	}
	if doc.Chain, err = b.stateChain(tc); err != nil {
		return stateDoc{}, err
	}
	doc.Pending = b.statePending(taskID)
	return doc, nil
}

func (b *TaskBoard) stateStepOf(tc *taskCtx) stateStep {
	if tc.wf == nil {
		return stateStep{ID: cmpNonEmpty(tc.task.EntryStepID, "")}
	}
	plan, err := b.plan(tc)
	if err != nil {
		return stateStep{}
	}
	entry := tc.entryIndex(plan)
	if entry < 0 {
		return stateStep{}
	}
	cur := currentStep(plan, entry)
	if cur < 0 {
		return stateStep{ID: entryDone, Of: len(plan)}
	}
	return stateStep{ID: plan[cur].Def.ID, Name: plan[cur].Def.Name, Position: cur + 1, Of: len(plan)}
}

// setupOfInfo maps the setup state to the 9.4 words; a branch with no worktree has no setup.
func setupOfInfo(s agentmcp.SetupInfo) *stateSetup {
	switch s.State {
	case model.AdeSetupReady:
		return &stateSetup{Status: agentmcp.StatusDone, Note: s.Note}
	case model.AdeSetupRunning:
		return &stateSetup{Status: agentmcp.StatusWaiting, Note: s.Note}
	case model.AdeSetupFailed:
		return &stateSetup{Status: agentmcp.StatusFailed, Note: s.Note}
	}
	return nil
}

// runStatus maps a run state: a review run waits for the user, a live or queued one for Kira.
func runStatus(state string) string {
	switch state {
	case model.AdeRunDone, model.AdeRunBack:
		return agentmcp.StatusDone
	case model.AdeRunFailed:
		return agentmcp.StatusFailed
	case model.AdeRunReview:
		return agentmcp.StatusPending
	}
	return agentmcp.StatusWaiting
}

// outcomeText is a run outcome as compact JSON, cut at maxRunOutcome bytes.
func outcomeText(o any) string {
	if o == nil {
		return ""
	}
	raw, err := json.Marshal(o)
	if err != nil {
		return ""
	}
	return capBytes(string(raw), maxRunOutcome)
}

// stateLeases lists the queues the task holds or waits in.
func (b *TaskBoard) stateLeases(taskID string) []stateLease {
	out := []stateLease{}
	for _, q := range b.LeaseSnapshot().Queues {
		holder := ""
		if q.Holder != nil {
			holder = q.Holder.TaskTitle
			if q.Holder.TaskID == taskID {
				out = append(out, stateLease{Queue: q.Name, Status: agentmcp.StatusDone, Holder: holder})
				continue
			}
		}
		for _, w := range q.Waiters {
			if w.TaskID == taskID {
				out = append(out, stateLease{Queue: q.Name, Status: agentmcp.StatusWaiting, Position: w.Position, Holder: holder})
				break
			}
		}
	}
	return out
}

// stateChain is the task's parent and children; nil for a task with neither.
func (b *TaskBoard) stateChain(tc *taskCtx) (*stateChain, error) {
	chain := &stateChain{Children: []stateChild{}}
	if tc.task.ParentID != "" {
		if p, err := b.deps.Tasks.GetTask(tc.task.ParentID); err == nil {
			chain.Parent = &stateParent{ID: p.ID, Title: cmpNonEmpty(p.Title, p.JiraKey), Path: "../" + pathSegment(p.ID) + "/"}
		}
	}
	children, _, err := b.deps.Tasks.ChildTasks(tc.task.ID, "all", maxChildTasks)
	if err != nil {
		return nil, err
	}
	if len(children) > 0 {
		kids, err := b.stateChildren(children)
		if err != nil {
			return nil, err
		}
		chain.Children = kids
	}
	if chain.Parent == nil && len(chain.Children) == 0 {
		return nil, nil
	}
	return chain, nil
}

func (b *TaskBoard) stateChildren(children []model.AdeTask) ([]stateChild, error) {
	statuses, err := b.deps.Statuses.List()
	if err != nil {
		return nil, err
	}
	statusName := make(map[string]string, len(statuses))
	for _, s := range statuses {
		statusName[s.ID] = s.Name
	}
	facts, err := b.loadSpaceFacts()
	if err != nil {
		return nil, err
	}
	taskIDs := make([]string, len(children))
	for i, t := range children {
		taskIDs[i] = t.ID
	}
	lastByTask, err := b.deps.Tasks.LatestHandovers(taskIDs)
	if err != nil {
		return nil, err
	}
	runsByTask, err := b.deps.Tasks.LatestRunsOf(taskIDs)
	if err != nil {
		return nil, err
	}
	out := make([]stateChild, 0, len(children))
	for _, t := range children {
		c := stateChild{ID: t.ID, Title: cmpNonEmpty(cmpNonEmpty(t.Title, t.JiraKey), "New task"), Status: cmpNonEmpty(statusName[t.StatusID], t.StatusID), Archived: t.ArchivedAt != nil}
		c.Step, c.Finished = b.childStep(t, facts, runsByTask[t.ID])
		if last, ok := lastByTask[t.ID]; ok {
			if last.RelPath != "" {
				c.LastHandover = "../" + pathSegment(t.ID) + "/" + last.RelPath
			}
			c.Preview = capBytes(last.Body, lastHandoverBytes)
		}
		out = append(out, c)
	}
	return out, nil
}

// childStep is a child's current step name and whether it is done.
func (b *TaskBoard) childStep(t model.AdeTask, facts spaceFacts, runs []model.AdeRun) (step string, finished bool) {
	if t.ArchivedAt != nil {
		if finished = t.EntryStepID == entryDone; !finished {
			step = t.EntryStepID
			if wf, ok := b.taskWorkflow(t); ok {
				step = cmpNonEmpty(stepName(&taskCtx{wf: &wf}, t.EntryStepID), t.EntryStepID)
			}
		}
		return step, finished
	}
	tc, err := b.buildTaskCtx(t, facts)
	if err != nil {
		return "", false
	}
	tc.runs = &runs
	cur := b.currentStepID(tc)
	if finished = cur == entryDone; !finished {
		step = cmpNonEmpty(stepName(tc, cur), cur)
	}
	return step, finished
}

// statePending lists everything waiting on the user for the task.
func (b *TaskBoard) statePending(taskID string) []statePending {
	out := []statePending{}
	q := &b.branchReqs
	q.mu.Lock()
	if r := q.byID[q.byTask[taskID]]; r != nil {
		for _, it := range r.view.Items {
			subject := it.Repo + "/" + it.Branch
			out = append(out, statePending{ID: subject, Kind: branchPendingKind(it.Kind), Subject: subject, Since: kiratime.MsToISO(r.view.CreatedAt)})
		}
	}
	q.mu.Unlock()
	tq := &b.taskReqs
	tq.mu.Lock()
	for ref, r := range tq.byID {
		if r.parentID == taskID {
			out = append(out, statePending{ID: ref, Kind: "task.create", Subject: r.item.Target.Name, Since: kiratime.MsToISO(r.item.CreatedAt)})
		}
	}
	tq.mu.Unlock()
	if df := b.files; df != nil {
		df.mu.Lock()
		for path, p := range df.pending[taskID] {
			out = append(out, statePending{ID: path, Kind: "submit", Subject: path, Since: kiratime.MsToISO(p.at)})
		}
		df.mu.Unlock()
	}
	slices.SortFunc(out, func(a, c statePending) int {
		if d := cmpString(a.Kind, c.Kind); d != 0 {
			return d
		}
		return cmpString(a.ID, c.ID)
	})
	return out
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func branchPendingKind(kind string) string {
	switch kind {
	case branchKindRename:
		return "branch.rename"
	case branchKindSwitch:
		return "branch.change"
	}
	return "branch.request"
}

// taskYAML is task.yaml: the task definition, read-only for the agent.
func (df *detailsFiles) taskYAML(tc *taskCtx) []byte {
	type repo struct {
		Repo   string `yaml:"repo"`
		Base   string `yaml:"base,omitempty"`
		Branch string `yaml:"branch,omitempty"`
	}
	doc := struct {
		ID       string `yaml:"id"`
		Title    string `yaml:"title"`
		Kind     string `yaml:"kind"`
		Workflow string `yaml:"workflow,omitempty"`
		Parent   string `yaml:"parent,omitempty"`
		Repos    []repo `yaml:"repos"`
		Created  string `yaml:"created"`
	}{
		ID: tc.task.ID, Title: taskTitle(tc.task, tc.branches), Kind: tc.task.Kind, Parent: tc.task.ParentID,
		Repos: []repo{}, Created: kiratime.MsToISO(tc.task.CreatedAt),
	}
	if tc.wf != nil {
		doc.Workflow = tc.wf.Name
	}
	for _, br := range tc.branches {
		doc.Repos = append(doc.Repos, repo{Repo: tc.nick[br.CodeRepoID], Base: br.Base, Branch: br.Name})
	}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		panic(fmt.Sprintf("ade: task.yaml: %v", err)) // plain strings only: a failure is a programming error
	}
	return append([]byte(taskfiles.Header(taskfiles.KindTask)), raw...)
}
