package ade

import (
	"context"
	"log/slog"
	"slices"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Agent restore offer: what a quit or crash cut, offered once per boot. Runs come from boot recovery
// (a stuck run with a conversation to resume), TUI sessions from the marks Tracker.MarkQuit and
// StopAllTaskRunning leave on their rows.

// Agent item kinds, modes, continue support and states on the wire.
const (
	AgentKindRun     = "run"
	AgentKindSession = "session"

	AgentModeHeadless = "headless"
	AgentModeTUI      = "tui"

	AgentContinueOptional = "optional"
	AgentContinueRequired = "required"

	AgentReady       = "ready"
	AgentUnavailable = "unavailable"
)

// AgentItem is one agent offered again. ID is `run:<runId>` or `session:<sessionId>`.
type AgentItem struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	RunID       string `json:"runId"`
	SessionID   string `json:"sessionId"`
	TaskID      string `json:"taskId"`
	TaskTitle   string `json:"taskTitle"`
	BranchID    string `json:"branchId"`
	BranchLabel string `json:"branchLabel"`
	Step        string `json:"step"`
	Provider    string `json:"provider"`
	Mode        string `json:"mode"`
	// Working says the agent was mid-turn when Kira stopped; false when idle, waiting for input or unknown.
	Working bool `json:"working"`
	// Continue is whether the user may ask the agent to carry on: optional, or required (a review round).
	Continue        string `json:"continue"`
	ContinueDefault bool   `json:"continueDefault"`
	State           string `json:"state"`
	Reason          string `json:"reason"`
}

// AgentChoice names an offered agent and whether the user ticked Continue.
type AgentChoice struct {
	ID       string `json:"id"`
	Continue bool   `json:"continue"`
}

// AgentOutcome is one agent's restore result: a TUI launch to open, or the run queued in the background.
type AgentOutcome struct {
	ID     string
	Launch *adewire.Launch
	RunID  string
	Err    error
}

type offerItem struct {
	run     *model.AdeRun // set for a run
	session *repos.RestoreMark
}

func (o offerItem) id() string {
	if o.run != nil {
		return AgentKindRun + ":" + o.run.ID
	}
	return AgentKindSession + ":" + o.session.Session.ID
}

// restoreOffer holds the agent offer of this boot. The zero value is empty and usable.
type restoreOffer struct {
	mu    sync.Mutex
	items []offerItem
}

func (o *restoreOffer) set(items []offerItem) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.items = items
}

func (o *restoreOffer) list() []offerItem {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.items)
}

func (o *restoreOffer) find(id string) (offerItem, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, it := range o.items {
		if it.id() == id {
			return it, true
		}
	}
	return offerItem{}, false
}

func (o *restoreOffer) drop(id string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.items = slices.DeleteFunc(o.items, func(it offerItem) bool { return it.id() == id })
}

// fillRestoreOffer builds the offer after boot recovery: resumable runs, then marked TUI rows. A run
// whose conversation a TUI row also holds (a taken-over run) is dropped, the TUI row wins.
func (b *TaskBoard) fillRestoreOffer(runs []model.AdeRun) {
	marks, err := b.deps.Sessions.TakeRestoreSessions()
	if err != nil {
		slog.Warn("ade: recover: take restore sessions", "scope", "ade", "err", err)
	}
	held := map[string]bool{}
	for _, m := range marks {
		if m.Session.AgentSessionID != "" {
			held[m.Session.AgentSessionID] = true
		}
	}
	var items []offerItem
	for _, r := range runs {
		if runResumable(r) && !held[r.Launch.ResumeID] {
			items = append(items, offerItem{run: &r})
		}
	}
	for i := range marks {
		items = append(items, offerItem{session: &marks[i]})
	}
	b.restore.set(items)
}

// CloseAgentOffer forgets the agent offer. Every agent stays reachable on the board.
func (b *TaskBoard) CloseAgentOffer() { b.restore.set(nil) }

// AgentRestoreState plans every offered agent against the board as it is now. An item the user
// already took over or resumed leaves the offer.
func (b *TaskBoard) AgentRestoreState() []AgentItem {
	var out []AgentItem
	ctxs := map[string]*taskCtx{}
	for _, it := range b.restore.list() {
		item, keep := b.restoreItem(it, ctxs)
		if !keep {
			b.restore.drop(it.id())
			continue
		}
		out = append(out, item)
	}
	return out
}

// restoreItem plans one offered agent; keep is false when it is no longer pending (resumed elsewhere).
func (b *TaskBoard) restoreItem(it offerItem, ctxs map[string]*taskCtx) (item AgentItem, keep bool) {
	var taskID, branchID, providerID, stepID string
	if it.run != nil {
		run, err := b.deps.Tasks.GetRun(it.run.ID)
		if err != nil || !runResumable(run) {
			return AgentItem{}, false
		}
		taskID, branchID, stepID = run.TaskID, run.BranchID, run.StepID
		item = AgentItem{
			ID: it.id(), Kind: AgentKindRun, RunID: run.ID, SessionID: run.SessionID, Mode: AgentModeHeadless,
			Working: true, Continue: AgentContinueOptional, ContinueDefault: true,
		}
		if run.Purpose == model.AdeRunPurposeReview {
			item.Continue = AgentContinueRequired
		}
		providerID, _ = b.deps.Sessions.ProviderOfAgentSession(run.Launch.ResumeID)
	} else {
		rec, err := b.deps.Sessions.Get(it.session.Session.ID)
		if err != nil || rec == nil || rec.State != model.AdeSessionStateStopped {
			return AgentItem{}, false
		}
		taskID, branchID, stepID, providerID = rec.TaskID, rec.BranchID, rec.StepID, rec.Provider
		item = AgentItem{
			ID: it.id(), Kind: AgentKindSession, SessionID: rec.ID, Mode: AgentModeTUI,
			Working: it.session.Working, Continue: AgentContinueOptional, ContinueDefault: it.session.Working,
		}
	}
	item.TaskID, item.BranchID = taskID, branchID
	item.State = AgentReady
	tc, ok := ctxs[taskID]
	if !ok {
		var err error
		if tc, err = b.loadTaskCtx(taskID); err != nil {
			tc = nil
		}
		ctxs[taskID] = tc
	}
	if tc == nil {
		item.State, item.Reason = AgentUnavailable, i18n.T("backend.space.ade.restore.taskGone.body", nil)
		return item, true
	}
	item.TaskTitle = tc.task.Title
	if branchID != "" {
		br, found := tc.branch(branchID)
		if !found {
			item.State, item.Reason = AgentUnavailable, i18n.T("backend.space.ade.restore.branchGone.body", nil)
			return item, true
		}
		item.BranchLabel = br.Name
	}
	if tc.wf != nil {
		for _, s := range tc.wf.Steps {
			if s.ID == stepID {
				item.Step = s.Name
			}
		}
	}
	agent, err := b.agentFor(providerID)
	if err != nil {
		item.State, item.Reason = AgentUnavailable, i18n.T("backend.space.ade.restore.providerGone.body", nil)
		return item, true
	}
	item.Provider = agent.Label()
	if it.run != nil && it.run.Purpose != model.AdeRunPurposeReview && !b.isLatestAttempt(tc, *it.run) {
		item.State, item.Reason = AgentUnavailable, i18n.T("backend.space.ade.restore.notLatest.body", nil)
	}
	return item, true
}

// isLatestAttempt says run is the newest attempt of its step on its branch, the check a resume makes.
func (b *TaskBoard) isLatestAttempt(tc *taskCtx, run model.AdeRun) bool {
	plan, err := b.plan(tc)
	if err != nil {
		return false
	}
	idx := stepIndex(plan, run.StepID)
	if idx < 0 {
		return false
	}
	latest, ok := plan[idx].Runs[run.BranchID]
	return ok && latest.ID == run.ID
}

// RestoreAgent restores one offered agent. A step run goes back to work in the background when the
// user ticked Continue (a review round always does) and opens in a terminal without a message
// otherwise; a TUI session reopens in its terminal, with the continue line submitted when ticked. A
// restored or failed item stays out of or in the offer: only success drops it.
func (b *TaskBoard) RestoreAgent(ctx context.Context, c AgentChoice) AgentOutcome {
	out := AgentOutcome{ID: c.ID}
	it, ok := b.restore.find(c.ID)
	if !ok {
		out.Err = invalid("%s", i18n.T("backend.space.ade.restore.notOffered.body", nil))
		return out
	}
	switch {
	case it.run != nil && (c.Continue || it.run.Purpose == model.AdeRunPurposeReview):
		out.RunID = it.run.ID
		out.Err = b.ResumeRun(ctx, it.run.ID)
	case it.run != nil:
		out.RunID = it.run.ID
		out.Launch, out.Err = b.resumeRunInTUI(ctx, it.run.ID)
	default:
		out.Launch, out.Err = b.reopenSession(ctx, it.session.Session.ID, c.Continue)
	}
	if out.Err == nil {
		b.restore.drop(c.ID)
	}
	return out
}

// reopenSession resumes a stopped TUI row in a terminal.
func (b *TaskBoard) reopenSession(ctx context.Context, sessionID string, cont bool) (*adewire.Launch, error) {
	tr, err := b.tracker()
	if err != nil {
		return nil, err
	}
	rec, err := b.deps.Sessions.Get(sessionID)
	if err != nil {
		return nil, err
	}
	if rec == nil || rec.TaskID == "" {
		return nil, invalid("session %s is not a task session", sessionID)
	}
	mu := b.taskMu(rec.TaskID)
	mu.Lock()
	defer mu.Unlock()
	message := ""
	if cont {
		message = llmprompts.RestoreContinueLine.Text(b.prompts(), llmprompts.NoVars{})
	}
	l, err := b.takeOverLocked(ctx, tr, rec, opTakeOver, message)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// resumeRunInTUI opens the conversation of a run a restart stopped in a terminal, without a message.
// The run's resume spec is cleared after, so the board cannot resume the same conversation twice.
func (b *TaskBoard) resumeRunInTUI(ctx context.Context, runID string) (*adewire.Launch, error) {
	tr, err := b.tracker()
	if err != nil {
		return nil, err
	}
	run, err := b.deps.Tasks.GetRun(runID)
	if err != nil {
		return nil, err
	}
	mu := b.taskMu(run.TaskID)
	mu.Lock()
	defer mu.Unlock()
	if run, err = b.deps.Tasks.GetRun(runID); err != nil {
		return nil, err
	}
	if !runResumable(run) {
		return nil, invalid("%s", i18n.T("backend.space.ade.resumeNotResumable.body", nil))
	}
	rec, err := b.deps.Sessions.Get(run.SessionID)
	if err != nil {
		return nil, err
	}
	if rec == nil || rec.TaskID == "" {
		return nil, invalid("session %s is not a task session", run.SessionID)
	}
	l, err := b.takeOverLocked(ctx, tr, rec, opTakeOver, "")
	if err != nil {
		return nil, err
	}
	cleared, err := b.deps.Tasks.UpdateRun(run.ID, model.AdeRunPatch{Launch: &model.AdeRunLaunch{}})
	if err != nil {
		return nil, err
	}
	b.emitRuns(cleared)
	return &l, nil
}
