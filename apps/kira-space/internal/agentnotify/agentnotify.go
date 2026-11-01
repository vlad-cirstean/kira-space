// Package agentnotify turns Claude Code hook events and ADE headless run changes into desktop
// notifications. It decides whether to notify (preferences, wake tracking, focus, cooldown) and
// builds the text; a Sink posts it (the macOS sink is internal/desknotify).
package agentnotify

import (
	"cmp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/desknotify"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/runoutcome"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Kind is what a notification is about.
type Kind string

const (
	KindFinished   Kind = "finished"
	KindNeedsInput Kind = "needs-input"
	KindRunEnded   Kind = "run-ended"
	KindAutomation Kind = "automation"
	KindTest       Kind = "test"
)

// Note is one notification. The id fields round-trip through the OS notification so a click can
// find its target.
type Note struct {
	ID, Title, Body                         string
	Kind                                    Kind
	TerminalID, WindowKey, RecordID, TaskID string
	// ScriptRunID is set for an automation note; a click opens that run.
	ScriptRunID string
}

// Sink posts a Note to the OS.
type Sink interface{ Send(Note) error }

// SinkFor adapts a shared desktop sink; ids stay in Data under the keys a click reads back.
func SinkFor(d desknotify.Sink) Sink { return deskSink{d} }

type deskSink struct{ d desknotify.Sink }

func (s deskSink) Send(n Note) error {
	return s.d.Send(desknotify.Note{
		ID: n.ID, Title: n.Title, Body: n.Body, Thread: "kira-agents",
		Data: map[string]string{
			"terminalId": n.TerminalID, "windowKey": n.WindowKey, "recordId": n.RecordID,
			"taskId": n.TaskID, "scriptRunId": n.ScriptRunID, "kind": string(n.Kind),
		},
	})
}

// Prefs are the agent.notify* settings.
type Prefs struct {
	Enabled, OnFinished, OnNeedsInput, OnRunEnded, IncludeMessage bool
}

// Target says what a hook event or run belongs to. Name is the task title, else the code
// repository name, else the cwd's basename.
// Agent labels the session's own provider; empty falls back to Deps.AgentLabel.
type Target struct{ Name, WindowKey, RecordID, TaskID, Agent string }

// FocusState is what one window reports about what the user is looking at.
type FocusState struct {
	WindowKey        string
	Focused          bool
	Module           string
	ActiveTerminalID string
	AdeTaskID        string
	// ActiveScriptRunID is the smart script run the window shows, else empty.
	ActiveScriptRunID string
}

// Deps are the seams the Notifier reads.
type Deps struct {
	Prefs     func() Prefs
	Describe  func(terminalID, cwd string) Target
	TaskTitle func(taskID string) string
	// Alive reports whether a window still exists; a closed window's last focus report is ignored.
	Alive func(windowKey string) bool
	// AppFocused reports whether any app window is the key window; nil reads false.
	AppFocused func() bool
	Reveal     func(n Note)
	Now        func() time.Time
	// AgentLabel names the active agent in titles when a target names none; nil reads "Agent".
	AgentLabel func() string
}

// Cooldown is the minimum gap between two notes of one kind for one terminal or run. A var so a
// flow test can shorten it.
var Cooldown = 10 * time.Second

const (
	maxBodyBytes = 200
	// maxTrackedRuns bounds the run-state map; a reset only costs one missed note per live run.
	maxTrackedRuns = 4096
)

// Notifier is safe for concurrent use.
type Notifier struct {
	d Deps

	mu    sync.Mutex
	sink  Sink
	focus map[string]FocusState
	wake  map[string]bool
	runs  map[string]adewire.RunState
	smart map[string]bool
	last  map[string]time.Time
}

func New(d Deps) *Notifier {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.AgentLabel == nil {
		d.AgentLabel = func() string { return "Agent" }
	}
	return &Notifier{
		d: d, focus: map[string]FocusState{}, wake: map[string]bool{},
		runs: map[string]adewire.RunState{}, smart: map[string]bool{}, last: map[string]time.Time{},
	}
}

// SetSink installs the OS sink; nil drops every note.
func (n *Notifier) SetSink(s Sink) {
	n.mu.Lock()
	n.sink = s
	n.mu.Unlock()
}

// ReportFocus records what one window shows now.
func (n *Notifier) ReportFocus(f FocusState) {
	n.mu.Lock()
	n.focus[f.WindowKey] = f
	n.mu.Unlock()
}

// HandleEvent reacts to one hook event.
func (n *Notifier) HandleEvent(ev harness.AgentEvent) {
	n.mu.Lock()
	armed := n.wake[ev.TerminalID]
	switch ev.Event {
	case harness.EventToolStart:
		if ev.Wakes {
			n.wake[ev.TerminalID] = true
		}
		n.mu.Unlock()
		return
	case harness.EventPrompt, harness.EventSessionStart:
		delete(n.wake, ev.TerminalID)
		n.mu.Unlock()
		return
	case harness.EventSessionEnd:
		delete(n.wake, ev.TerminalID)
		n.mu.Unlock()
		return
	case harness.EventTurnEnd:
		delete(n.wake, ev.TerminalID)
	}
	n.mu.Unlock()

	switch ev.Event {
	case harness.EventTurnEnd:
		if armed {
			return
		}
		n.notifyTerminal(KindFinished, ev, ev.LastAssistantMessage)
	case harness.EventNotification:
		if ev.NeedsInput {
			n.notifyTerminal(KindNeedsInput, ev, ev.Message)
		}
	}
}

func (n *Notifier) notifyTerminal(kind Kind, ev harness.AgentEvent, message string) {
	p := n.d.Prefs()
	if !p.Enabled || (kind == KindFinished && !p.OnFinished) || (kind == KindNeedsInput && !p.OnNeedsInput) {
		return
	}
	// Outside n.mu: AppFocused blocks on the main thread.
	if kind == KindFinished && n.d.AppFocused != nil && n.d.AppFocused() {
		return
	}
	t := n.d.Describe(ev.TerminalID, ev.Cwd)
	fallback := i18n.T("backend.notify.agent.finished.body", nil)
	agent := cmp.Or(t.Agent, n.d.AgentLabel())
	title := i18n.T("backend.notify.agent.finished.title", map[string]string{"agent": agent, "name": t.Name})
	if kind == KindNeedsInput {
		fallback = i18n.T("backend.notify.agent.needsInput.body", nil)
		title = i18n.T("backend.notify.agent.needsInput.title", map[string]string{"agent": agent, "name": t.Name})
	}
	note := Note{
		ID: string(kind) + ":" + ev.TerminalID, Kind: kind, Title: title,
		Body: body(p, message, fallback), TerminalID: ev.TerminalID, WindowKey: t.WindowKey,
		RecordID: t.RecordID, TaskID: t.TaskID,
	}
	n.mu.Lock()
	if n.watchedLocked(t, ev.TerminalID) || !n.admitLocked(note.ID) {
		n.mu.Unlock()
		return
	}
	sink := n.sink
	n.mu.Unlock()
	n.send(sink, note)
}

// HandleRuns reacts to changed ADE headless runs.
func (n *Notifier) HandleRuns(runs []adewire.Run) {
	var ended []adewire.Run
	n.mu.Lock()
	if len(n.runs) > maxTrackedRuns {
		n.runs = map[string]adewire.RunState{}
	}
	for _, r := range runs {
		prev, seen := n.runs[r.ID]
		n.runs[r.ID] = r.State
		if seen && !runEnded(prev) && runEnded(r.State) {
			ended = append(ended, r)
		}
	}
	n.mu.Unlock()
	if len(ended) == 0 {
		return
	}
	p := n.d.Prefs()
	if !p.Enabled || !p.OnRunEnded {
		return
	}
	for _, r := range ended {
		name := n.d.TaskTitle(r.TaskID)
		detail := r.Summary
		if detail == "" {
			detail = r.Note
		}
		if r.Purpose == "rebase" && r.Outcome != nil && r.Outcome.Reason != "" {
			detail = r.Outcome.Reason
		}
		note := Note{
			ID: string(KindRunEnded) + ":" + r.ID, Kind: KindRunEnded, TaskID: r.TaskID,
			Title: runEndedTitle(r, name), Body: body(p, detail, i18n.T("backend.notify.adeRun.body", nil)),
		}
		n.mu.Lock()
		if n.watchedRunLocked(r.TaskID) || !n.admitLocked(note.ID) {
			n.mu.Unlock()
			continue
		}
		sink := n.sink
		n.mu.Unlock()
		n.send(sink, note)
	}
}

// HandleScriptRun reacts to a changed script run. A smart run notifies from any trigger: failed
// under Enabled, blocked under OnNeedsInput, done under OnRunEnded, cancelled never. A scheduled
// normal run notifies only when it fails. waiting and skipped runs never notify.
func (n *Notifier) HandleScriptRun(r scriptruns.Run) {
	scheduled := r.Trigger == scriptruns.TriggerScheduled
	if r.Kind != scriptruns.KindSmart && !scheduled {
		return
	}
	n.mu.Lock()
	if len(n.smart) > maxTrackedRuns {
		n.smart = map[string]bool{}
	}
	if r.State == scriptruns.StateRunning {
		n.smart[r.ID] = true
		n.mu.Unlock()
		return
	}
	tracked := n.smart[r.ID]
	delete(n.smart, r.ID)
	n.mu.Unlock()
	if !tracked {
		return
	}
	p := n.d.Prefs()
	var title, detail string
	switch runoutcome.Status(r.State) {
	case runoutcome.StatusFailed:
		if scheduled {
			title = i18n.T("backend.notify.recurringScript.failed.title", map[string]string{"name": r.ScriptName})
		} else {
			title = i18n.T("backend.notify.automation.failed.title", map[string]string{"name": r.ScriptName})
		}
	case runoutcome.StatusBlocked:
		if r.Kind != scriptruns.KindSmart {
			return
		}
		if !p.OnNeedsInput {
			return
		}
		title = i18n.T("backend.notify.automation.needsYou.title", map[string]string{"name": r.ScriptName})
	case runoutcome.StatusDone:
		if r.Kind != scriptruns.KindSmart || !p.OnRunEnded {
			return
		}
		title = i18n.T("backend.notify.automation.done.title", map[string]string{"name": r.ScriptName})
	default:
		return
	}
	if !p.Enabled {
		return
	}
	if o := r.Outcome; o != nil {
		detail = o.Reason
		if detail == "" {
			detail = o.Summary
		}
	}
	note := Note{
		ID: string(KindAutomation) + ":" + r.ID, Kind: KindAutomation, TaskID: r.TaskID, ScriptRunID: r.ID,
		Title: title, Body: body(p, detail, i18n.T("backend.notify.automation.body", nil)),
	}
	n.mu.Lock()
	if n.watchedScriptRunLocked(r.ID) || !n.admitLocked(note.ID) {
		n.mu.Unlock()
		return
	}
	sink := n.sink
	n.mu.Unlock()
	n.send(sink, note)
}

func (n *Notifier) watchedScriptRunLocked(runID string) bool {
	for key, f := range n.focus {
		if f.Focused && f.ActiveScriptRunID == runID && (n.d.Alive == nil || n.d.Alive(key)) {
			return true
		}
	}
	return false
}

// runEndedTitle is the notification title of an ended ADE run.
func runEndedTitle(r adewire.Run, name string) string {
	if r.Purpose == "rebase" {
		switch {
		case r.State == "done":
			return i18n.T("backend.notify.rebase.done.title", map[string]string{"name": name})
		case r.State == "failed":
			return i18n.T("backend.notify.rebase.failed.title", map[string]string{"name": name})
		case r.Outcome != nil && r.Outcome.Status == "cancelled":
			return i18n.T("backend.notify.rebase.stopped.title", map[string]string{"name": name})
		}
		return i18n.T("backend.notify.rebase.needsYou.title", map[string]string{"name": name})
	}
	if res := stepResult(r); res != "" {
		return i18n.T("backend.notify.adeStep.title", map[string]string{"result": res, "name": name})
	}
	switch r.State {
	case "done":
		return i18n.T("backend.notify.adeRun.done.title", map[string]string{"name": name})
	case "failed":
		return i18n.T("backend.notify.adeRun.failed.title", map[string]string{"name": name})
	}
	return i18n.T("backend.notify.adeRun.stuck.title", map[string]string{"name": name})
}

// stepResult is the step result an agent reported, "" for the implicit done and failed.
func stepResult(r adewire.Run) string {
	if r.Outcome == nil || r.Purpose == "rebase" {
		return ""
	}
	if id := r.Outcome.Result; id != "done" && id != "failed" {
		return id
	}
	return ""
}

func runEnded(s adewire.RunState) bool {
	return s == "done" || s == "failed" || s == "stuck"
}

// SendTest posts a test note, ignoring cooldown and focus; it honours the master switch.
func (n *Notifier) SendTest() {
	if !n.d.Prefs().Enabled {
		return
	}
	n.mu.Lock()
	sink := n.sink
	n.mu.Unlock()
	n.send(sink, Note{ID: "test:kira", Kind: KindTest, Title: i18n.T("backend.notify.test.title", nil), Body: i18n.T("backend.notify.test.body", nil)})
}

// Click routes an OS notification click back to the window that should show its target.
func (n *Notifier) Click(data map[string]any) {
	str := func(k string) string {
		s, _ := data[k].(string)
		return s
	}
	note := Note{
		Kind: Kind(str("kind")), TerminalID: str("terminalId"), WindowKey: str("windowKey"),
		RecordID: str("recordId"), TaskID: str("taskId"), ScriptRunID: str("scriptRunId"),
	}
	if n.d.Reveal != nil {
		n.d.Reveal(note)
	}
}

func (n *Notifier) send(sink Sink, note Note) {
	if sink == nil {
		return
	}
	// A failed post (denied permission, missing bundle) is the sink's to log; the next event retries.
	_ = sink.Send(note)
}

// watchedLocked reports whether the user already looks at the session behind t.
func (n *Notifier) watchedLocked(t Target, terminalID string) bool {
	f, ok := n.focus[t.WindowKey]
	if !ok || !f.Focused {
		return false
	}
	if f.ActiveTerminalID != "" && f.ActiveTerminalID == terminalID {
		return true
	}
	return t.TaskID != "" && f.Module == "ade" && f.AdeTaskID == t.TaskID
}

func (n *Notifier) watchedRunLocked(taskID string) bool {
	for key, f := range n.focus {
		if f.Focused && f.Module == "ade" && f.AdeTaskID == taskID && (n.d.Alive == nil || n.d.Alive(key)) {
			return true
		}
	}
	return false
}

// admitLocked applies the cooldown to key and records the post.
func (n *Notifier) admitLocked(key string) bool {
	now := n.d.Now()
	if at, ok := n.last[key]; ok && now.Sub(at) < Cooldown {
		return false
	}
	for k, at := range n.last {
		if now.Sub(at) >= Cooldown {
			delete(n.last, k)
		}
	}
	n.last[key] = now
	return true
}

func body(p Prefs, message, fallback string) string {
	if !p.IncludeMessage {
		return fallback
	}
	if line := oneLine(message); line != "" {
		return line
	}
	return fallback
}

// oneLine collapses whitespace and bounds the result on a rune boundary.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= maxBodyBytes {
		return s
	}
	s = s[:maxBodyBytes]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
