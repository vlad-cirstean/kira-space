package ade

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aymanbagabas/go-udiff"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// panelforward.go collects the user's task panel edits and forwards them, as diffs, to the agents
// running in the task's current step. The pending set lives in memory: a restart ends every
// interactive session, so nothing pending could still reach its target.

const (
	panelItemKindNote     = "note"
	panelItemKindHandover = "handover"
	panelItemKindLink     = "link"
	panelItemKindProof    = "proof"
	panelItemKindField    = "field"

	panelDiffContext    = 2
	panelMaxItemLines   = 200
	panelMaxMessageSize = 32 << 10
	panelMessageReserve = 1 << 10 // the message's fixed text
	panelMaxPending     = 32
)

type pendingItem struct {
	key, kind, label string
	before, after    string
	at               int64
}

type pendingSet struct {
	stepID  string
	items   map[string]*pendingItem
	order   []string
	dropped int
}

type panelEdits struct {
	mu     sync.Mutex
	byTask map[string]*pendingSet
}

// forwardTargets lists the sessions running in the task's current step: reachable ones hold a bound
// terminal; the rest (headless runs, a TUI row without a terminal) take no message. Mirrors
// stageSessions: branch sessions never qualify.
func (b *TaskBoard) forwardTargets(tc *taskCtx, step string) (reach, unreach []model.AdeSession, err error) {
	if step == "" || step == entryDone {
		return nil, nil, nil
	}
	match := func(r model.AdeSession) bool {
		if r.TaskID != tc.task.ID || r.State != model.AdeSessionStateRunning {
			return false
		}
		if r.Purpose == model.AdeSessionPurposeReview {
			return step == reviewStepID
		}
		return r.StepID == step
	}
	tui, err := b.deps.Sessions.ListRunningTUI()
	if err != nil {
		return nil, nil, err
	}
	for _, r := range tui {
		if !match(r) {
			continue
		}
		if b.deps.Tracker != nil {
			if _, ok := b.deps.Tracker.TerminalOf(r.ID); ok {
				reach = append(reach, r)
				continue
			}
		}
		unreach = append(unreach, r)
	}
	all, err := b.deps.Sessions.ListTask()
	if err != nil {
		return nil, nil, err
	}
	for _, r := range all {
		if r.Mode == model.AdeSessionModeHeadless && match(r) {
			unreach = append(unreach, r)
		}
	}
	byStart := func(rows []model.AdeSession) {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].StartedAt != rows[j].StartedAt {
				return rows[i].StartedAt < rows[j].StartedAt
			}
			return rows[i].ID < rows[j].ID
		})
	}
	byStart(reach)
	byStart(unreach)
	return reach, unreach, nil
}

func panelBadge(tc *taskCtx, r model.AdeSession) string {
	name := "Review agent"
	if r.Purpose != model.AdeSessionPurposeReview {
		name = cmpNonEmpty(stepName(tc, r.StepID), r.StepID)
	}
	if r.Provider != "" {
		return name + " · " + r.Provider
	}
	return name
}

// userTrigger says the call comes from the user's own action, not from an agent or the app.
func userTrigger(ctx context.Context) bool {
	o, ok := oplog.OriginFrom(ctx)
	return ok && o.Trigger == oplog.TriggerUser
}

// recordPanelEdit adds a user edit to the task's pending set. It is a no-op when no session runs in
// the step. A later edit of the same item keeps its first before value; an edit that undoes itself drops out.
func (b *TaskBoard) recordPanelEdit(taskID, key, kind, label, before, after string) {
	tc, err := b.loadTaskCtx(taskID)
	if err != nil || tc.task.Kind == model.AdeTaskKindReview {
		return
	}
	step := b.currentStepID(tc)
	reach, unreach, err := b.forwardTargets(tc, step)
	if err != nil || len(reach)+len(unreach) == 0 {
		return
	}
	pe := &b.panelEdits
	pe.mu.Lock()
	defer pe.mu.Unlock()
	if pe.byTask == nil {
		pe.byTask = map[string]*pendingSet{}
	}
	set := pe.byTask[taskID]
	if set == nil || set.stepID != step {
		set = &pendingSet{stepID: step, items: map[string]*pendingItem{}}
		pe.byTask[taskID] = set
	}
	now := b.deps.Now().UnixMilli()
	if it, ok := set.items[key]; ok {
		it.after, it.at = after, now
		if it.before == it.after {
			delete(set.items, key)
			set.order = removeString(set.order, key)
		}
		return
	}
	if before == after {
		return
	}
	if len(set.order) >= panelMaxPending {
		oldest := set.order[0]
		delete(set.items, oldest)
		set.order = set.order[1:]
		set.dropped++
	}
	set.items[key] = &pendingItem{key: key, kind: kind, label: label, before: before, after: after, at: now}
	set.order = append(set.order, key)
}

func removeString(s []string, v string) []string {
	out := s[:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

// pendingFor returns the task's pending set when it still matches the step and a session runs in it;
// otherwise it drops the set. The caller holds panelEdits.mu.
func (b *TaskBoard) pendingFor(taskID, step string, running bool) *pendingSet {
	set := b.panelEdits.byTask[taskID]
	if set == nil {
		return nil
	}
	if set.stepID != step || !running {
		delete(b.panelEdits.byTask, taskID)
		return nil
	}
	return set
}

func (b *TaskBoard) clearPending(taskID string) {
	b.panelEdits.mu.Lock()
	delete(b.panelEdits.byTask, taskID)
	b.panelEdits.mu.Unlock()
}

// textDiff is a unified diff of two texts without the file headers.
func textDiff(before, after string) string {
	if before != "" && !strings.HasSuffix(before, "\n") {
		before += "\n"
	}
	if after != "" && !strings.HasSuffix(after, "\n") {
		after += "\n"
	}
	out, err := udiff.ToUnified("a", "b", before, udiff.Lines(before, after), panelDiffContext)
	if err != nil {
		// Lines yields edits consistent with before; fall back to the plain two-sided form.
		return lineDiff(before, after)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for len(lines) > 0 && (strings.HasPrefix(lines[0], "--- ") || strings.HasPrefix(lines[0], "+++ ")) {
		lines = lines[1:]
	}
	return strings.Join(lines, "\n")
}

// lineDiff is the one-line-per-side form of links, proofs and the title.
func lineDiff(before, after string) string {
	var lines []string
	if before != "" {
		lines = append(lines, "- "+before)
	}
	if after != "" {
		lines = append(lines, "+ "+after)
	}
	return strings.Join(lines, "\n")
}

func (it *pendingItem) diff() string {
	if it.kind == panelItemKindNote || it.kind == panelItemKindHandover {
		return textDiff(it.before, it.after)
	}
	return lineDiff(it.before, it.after)
}

// cutDiff keeps at most maxLines lines and maxBytes bytes of diff and ends a cut diff with a marker.
func cutDiff(diff string, maxLines, maxBytes int) string {
	lines := strings.Split(diff, "\n")
	keep, size := 0, 0
	for keep < len(lines) && keep < maxLines && size+len(lines[keep])+1 <= maxBytes {
		size += len(lines[keep]) + 1
		keep++
	}
	if keep == len(lines) {
		return diff
	}
	marker := fmt.Sprintf("[cut: %d more lines; full text in the task panel]", len(lines)-keep)
	return strings.Join(append(lines[:keep:keep], marker), "\n")
}

// composePanelMessage renders the pending items as the message pasted into the agents.
func (b *TaskBoard) composePanelMessage(set *pendingSet) string {
	vars := llmprompts.PanelForwardVars{Items: make([]llmprompts.PanelForwardItemVar, 0, len(set.order))}
	budget := panelMaxMessageSize - panelMessageReserve
	for _, k := range set.order {
		it := set.items[k]
		left := max(budget, 0)
		diff := cutDiff(it.diff(), panelMaxItemLines, left)
		budget -= len(it.label) + len(diff) + 8
		vars.Items = append(vars.Items, llmprompts.PanelForwardItemVar{Label: it.label, Diff: diff})
	}
	return llmprompts.AdePanelForwardMessage.Text(b.prompts(), vars)
}

func toWireTargets(tc *taskCtx, rows []model.AdeSession) []adewire.ForwardTarget {
	out := make([]adewire.ForwardTarget, len(rows))
	for i, r := range rows {
		out[i] = adewire.ForwardTarget{SessionID: r.ID, Badge: panelBadge(tc, r)}
	}
	return out
}

// PanelForward lists the panel edits not yet sent to the agents running in the task's step, the agents
// that would take them, and the exact message.
func (b *TaskBoard) PanelForward(_ context.Context, taskID string) (adewire.PanelForward, error) {
	out := adewire.PanelForward{Items: []adewire.PanelForwardItem{}, Targets: []adewire.ForwardTarget{}}
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return out, nil // archived or gone: nothing to offer
	}
	step := b.currentStepID(tc)
	reach, unreach, err := b.forwardTargets(tc, step)
	if err != nil {
		return out, err
	}
	b.panelEdits.mu.Lock()
	defer b.panelEdits.mu.Unlock()
	if tc.task.Kind == model.AdeTaskKindReview {
		delete(b.panelEdits.byTask, taskID)
		return out, nil
	}
	set := b.pendingFor(taskID, step, len(reach)+len(unreach) > 0)
	if set == nil || len(set.order) == 0 {
		return out, nil
	}
	out.StepID, out.StepName = step, cmpNonEmpty(stepName(tc, step), step)
	for _, k := range set.order {
		it := set.items[k]
		out.Items = append(out.Items, adewire.PanelForwardItem{Key: it.key, Kind: it.kind, Label: it.label, At: it.at})
	}
	out.Targets = toWireTargets(tc, reach)
	out.Unreachable = len(unreach)
	out.Dropped = set.dropped
	out.Preview = b.composePanelMessage(set)
	return out, nil
}

// ForwardPanelEdits pastes the pending panel edits into every reachable agent of the step. One
// delivery clears the set, since the agents share the step and a second forward would paste it again.
func (b *TaskBoard) ForwardPanelEdits(_ context.Context, taskID string) (adewire.PanelForwardResult, error) {
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	res := adewire.PanelForwardResult{Sent: []string{}, Ended: []string{}, Failed: []adewire.PanelForwardFailure{}}
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return res, err
	}
	step := b.currentStepID(tc)
	reach, unreach, err := b.forwardTargets(tc, step)
	if err != nil {
		return res, err
	}
	b.panelEdits.mu.Lock()
	set := b.pendingFor(taskID, step, len(reach)+len(unreach) > 0)
	var message string
	if set != nil && len(set.order) > 0 {
		message = b.composePanelMessage(set)
	}
	b.panelEdits.mu.Unlock()
	switch {
	case len(reach)+len(unreach) == 0:
		return res, invalid("%s", i18n.T("backend.space.ade.panelForward.noAgent.body", nil))
	case message == "":
		return res, invalid("%s", i18n.T("backend.space.ade.panelForward.empty.body", nil))
	case len(reach) == 0:
		return res, invalid("%s", i18n.T("backend.space.ade.panelForward.unreachable.body", nil))
	}
	tr, err := b.tracker()
	if err != nil {
		return res, err
	}
	var errs []error
	for _, r := range reach {
		switch err := tr.Send(r.ID, message); {
		case err == nil:
			res.Sent = append(res.Sent, r.ID)
		case errors.Is(err, ErrSessionNotRunning):
			res.Ended = append(res.Ended, r.ID)
		default:
			res.Failed = append(res.Failed, adewire.PanelForwardFailure{SessionID: r.ID, Error: err.Error()})
			errs = append(errs, fmt.Errorf("%s: %w", panelBadge(tc, r), err))
		}
	}
	switch {
	case len(res.Sent) > 0:
		b.clearPending(taskID)
		b.notifyBoard()
		return res, nil
	case len(errs) == 0:
		b.clearPending(taskID)
		b.notifyBoard()
		return res, invalid("%s", i18n.T("backend.space.ade.panelForward.noAgent.body", nil))
	}
	return res, errors.Join(errs...)
}

// DismissPanelEdits drops the task's pending panel edits without sending them.
func (b *TaskBoard) DismissPanelEdits(_ context.Context, taskID string) error {
	b.clearPending(taskID)
	b.notifyBoard()
	return nil
}

// panelTime formats a millisecond stamp for a label.
func panelTime(ms int64) string { return time.UnixMilli(ms).Format("15:04") }

func noteLabel(h model.AdeHandover) string {
	return fmt.Sprintf("note (%s, %s)", cmpNonEmpty(h.FromStepName, h.FromStepID), panelTime(h.CreatedAt))
}

func handoverLabel(h model.AdeHandover) string {
	if h.State == model.AdeHandoverDraft {
		return fmt.Sprintf("handover draft (%s)", cmpNonEmpty(h.FromStepName, h.FromStepID))
	}
	return fmt.Sprintf("handover (%s to %s)", cmpNonEmpty(h.FromStepName, h.FromStepID), cmpNonEmpty(h.ToStepName, h.ToStepID))
}

// recordHandover records a note or handover body change.
func (b *TaskBoard) recordHandover(h model.AdeHandover, before, after string) {
	kind, label := panelItemKindHandover, handoverLabel(h)
	if h.State == model.AdeHandoverNote {
		kind, label = panelItemKindNote, noteLabel(h)
	}
	b.recordPanelEdit(h.TaskID, kind+":"+h.ID, kind, label, before, after)
}

func linkText(kind, title, url, ref string) string {
	return strings.TrimSpace(kind + " " + title + " " + cmpNonEmpty(url, ref))
}

func artifactText(a model.AdeTaskArtifact) string { return linkText(a.Kind, a.Title, a.URL, a.Ref) }

func (b *TaskBoard) recordArtifact(taskID, id, before, after string) {
	b.recordPanelEdit(taskID, "link:"+id, panelItemKindLink, "link", before, after)
}

func jiraText(key, url string) string {
	if key == "" && url == "" {
		return ""
	}
	return strings.TrimSpace("jira " + key + " " + url)
}

func hostText(url string) string {
	if url == "" {
		return ""
	}
	return "host " + url
}

func (b *TaskBoard) recordJira(taskID, beforeKey, beforeURL, afterKey, afterURL string) {
	b.recordPanelEdit(taskID, "link:jira", panelItemKindLink, "Jira link", jiraText(beforeKey, beforeURL), jiraText(afterKey, afterURL))
}

func (b *TaskBoard) recordHost(taskID, before, after string) {
	b.recordPanelEdit(taskID, "link:host", panelItemKindLink, "host link", hostText(before), hostText(after))
}

// proofText is the line a proof shows in a forward: its name and summary.
func proofText(name, summary string) string {
	if summary == "" {
		return name
	}
	return name + ": " + summary
}

// proofOf reads the task's attached proof proofID, nil when none.
func (b *TaskBoard) proofOf(taskID, proofID string) *adewire.TaskProof {
	rows, err := b.TaskProofs(context.Background(), taskID)
	if err != nil {
		return nil
	}
	for i := range rows {
		if rows[i].ProofID == proofID {
			return &rows[i]
		}
	}
	return nil
}

// recordTaskEdit records the title, Jira and host slot changes of a user task update.
func (b *TaskBoard) recordTaskEdit(prev, next model.AdeTask) {
	if prev.Title != next.Title {
		b.recordPanelEdit(next.ID, "field:title", panelItemKindField, "task title", prev.Title, next.Title)
	}
	if prev.JiraKey != next.JiraKey || prev.JiraURL != next.JiraURL {
		b.recordJira(next.ID, prev.JiraKey, prev.JiraURL, next.JiraKey, next.JiraURL)
	}
	if prev.HostURL != next.HostURL {
		b.recordHost(next.ID, prev.HostURL, next.HostURL)
	}
}
