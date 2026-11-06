package ade

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

// fakeLive is a test double for Registry.AgentSessions/Write — a terminal is "live" once added,
// until removed, with no real PTY involved.
type fakeLive struct {
	mu   sync.Mutex
	ids  map[string]bool
	logs []string
}

func newFakeLive() *fakeLive { return &fakeLive{ids: map[string]bool{}} }

func (f *fakeLive) add(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids[id] = true
}

func (f *fakeLive) remove(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.ids, id)
}

func (f *fakeLive) AgentSessions() []terminal.AgentSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]terminal.AgentSession, 0, len(f.ids))
	for id := range f.ids {
		out = append(out, terminal.AgentSession{ID: id, Cwd: "/repo"})
	}
	return out
}

func (f *fakeLive) Write(id string, b []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, id+":"+string(b))
	return nil
}

// fakeClock is a manually-advanced time source, so grace-window behaviour is observable without a
// real sleep.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(1_700_000_000, 0)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestTracker(t *testing.T) (*Tracker, *repos.AdeSessionsRepo, *fakeLive, *fakeClock) {
	t.Helper()
	db, err := storage.OpenAt(t.TempDir())
	if err != nil {
		t.Fatalf("storage.OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.DB.Exec(`INSERT INTO ade_tasks (id, kind, color, created_at) VALUES ('t1', 'task', 0, 1), ('other', 'task', 0, 1)`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	repositories, err := repos.New(db.DB)
	if err != nil {
		t.Fatalf("repos.New: %v", err)
	}

	live := newFakeLive()
	clock := newFakeClock()
	// AgentBin points Prepare's own CLI preflight check at this test binary (always present and
	// executable) rather than the real "claude" — these tests mock the launch itself via live/
	// WriteTerminal, so what AgentBin resolves to past the preflight check never runs.
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	tr := NewTracker(TrackerDeps{
		Agent: testAgents,
		Store: repositories.AdeSessions, LiveAgents: live.AgentSessions, WriteTerminal: live.Write,
		Now: clock.Now, Grace: 20 * time.Millisecond, PendingTTL: time.Minute, AgentBin: self,
	})
	return tr, repositories.AdeSessions, live, clock
}

// composeAndSpawn drives the full launch flow a real BoundService.Open would (Prepare then
// Compose, live registered right after), returning Prepare's own result.
func composeAndSpawn(t *testing.T, tr *Tracker, live *fakeLive, args PrepareArgs) PrepareResult {
	t.Helper()
	res, err := tr.Prepare(args)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, _, err := tr.Compose(res.TerminalID, "", res.Command); err != nil {
		t.Fatalf("Compose: %v", err)
	}
	live.add(res.TerminalID)
	return res
}

func listSessions(tr *Tracker) ([]model.AdeSession, error) {
	return tr.deps.Store.ListTask()
}

func TestTracker_PrepareComposeReconcileLive(t *testing.T) {
	tr, _, live, _ := newTestTracker(t)

	res := composeAndSpawn(t, tr, live, PrepareArgs{TaskID: "t1", Cwd: "/repo"})

	sessions, err := listSessions(tr)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 1 || sessions[0].State != model.AdeSessionStateRunning {
		t.Fatalf("sessions = %+v, want one running", sessions)
	}
	if sessions[0].TerminalID != res.TerminalID {
		t.Fatalf("terminalId = %q, want %q", sessions[0].TerminalID, res.TerminalID)
	}
	if sessions[0].AgentSessionID != res.SessionID {
		t.Fatalf("agentSessionId = %q, want %q", sessions[0].AgentSessionID, res.SessionID)
	}

	// Still live: Reconcile must not stop it, grace or not.
	tr.Reconcile()
	sessions, _ = listSessions(tr)
	if sessions[0].State != model.AdeSessionStateRunning {
		t.Fatalf("state after Reconcile while live = %q, want running", sessions[0].State)
	}
}

func TestTracker_ReconcileStopsOnExitAfterGrace(t *testing.T) {
	tr, _, live, clock := newTestTracker(t)
	res := composeAndSpawn(t, tr, live, PrepareArgs{TaskID: "t1", Cwd: "/repo"})

	live.remove(res.TerminalID)
	// Within the grace window: Reconcile must not stop it yet (Compose runs before the PTY
	// registers, so a Reconcile racing the spawn must never stop a record that only just started).
	tr.Reconcile()
	sessions, _ := listSessions(tr)
	if sessions[0].State != model.AdeSessionStateRunning {
		t.Fatalf("state within grace = %q, want still running", sessions[0].State)
	}

	clock.advance(50 * time.Millisecond)
	tr.Reconcile()
	sessions, _ = listSessions(tr)
	if sessions[0].State != model.AdeSessionStateStopped {
		t.Fatalf("state past grace = %q, want stopped", sessions[0].State)
	}
	if sessions[0].TerminalID != "" {
		t.Fatalf("terminalId after stop = %q, want empty", sessions[0].TerminalID)
	}
}

func TestTracker_ComposeMismatchedCommandRefused(t *testing.T) {
	tr, _, _, _ := newTestTracker(t)
	res, err := tr.Prepare(PrepareArgs{TaskID: "t1", Cwd: "/repo"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, _, err := tr.Compose(res.TerminalID, "", res.Command+" extra"); err != ErrCommandMismatch {
		t.Fatalf("Compose with mismatched command: err = %v, want ErrCommandMismatch", err)
	}
}

func TestTracker_SpawnNeverRegisteredStopsAfterGrace(t *testing.T) {
	tr, _, _, clock := newTestTracker(t)
	res, err := tr.Prepare(PrepareArgs{TaskID: "t1", Cwd: "/repo"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, _, err := tr.Compose(res.TerminalID, "", res.Command); err != nil {
		t.Fatalf("Compose: %v", err)
	}
	// Never added to `live` — the spawn itself failed after Compose returned.

	clock.advance(50 * time.Millisecond)
	tr.Reconcile()
	sessions, _ := listSessions(tr)
	if len(sessions) != 1 || sessions[0].State != model.AdeSessionStateStopped {
		t.Fatalf("sessions = %+v, want one stopped", sessions)
	}
}

func TestTracker_ResumeRejectsRunningRecord(t *testing.T) {
	tr, _, live, _ := newTestTracker(t)
	composeAndSpawn(t, tr, live, PrepareArgs{TaskID: "t1", Cwd: "/repo"})

	// The record id, not the returned SessionID (Claude session id) — recover it via List since
	// Prepare/Compose never hand the record id back directly.
	sessions, _ := listSessions(tr)
	recordID := sessions[0].ID

	if _, err := tr.Prepare(PrepareArgs{TaskID: "t1", Cwd: "/repo", Resume: recordID}); err != ErrSessionRunning {
		t.Fatalf("resume of a running record: err = %v, want ErrSessionRunning", err)
	}
}

func TestTracker_ResumeUnknownRecordRefused(t *testing.T) {
	tr, _, _, _ := newTestTracker(t)
	if _, err := tr.Prepare(PrepareArgs{TaskID: "t1", Resume: "does-not-exist"}); err != ErrSessionNotFound {
		t.Fatalf("resume of an unknown record: err = %v, want ErrSessionNotFound", err)
	}
}

func TestTracker_ResumeUsesRecordedCwdAndReusesAgentSessionID(t *testing.T) {
	tr, _, live, clock := newTestTracker(t)
	// A real, existing directory (unlike the bare "/repo" other cases in this file use): the resume
	// branch now os.Stat()s the recorded cwd (§0.12), so this needs to be an actual path.
	original := filepath.Join(t.TempDir(), "original")
	if err := os.MkdirAll(original, 0o755); err != nil {
		t.Fatalf("mkdir original: %v", err)
	}
	res := composeAndSpawn(t, tr, live, PrepareArgs{TaskID: "t1", Cwd: original})
	sessions, _ := listSessions(tr)
	recordID := sessions[0].ID

	live.remove(res.TerminalID)
	clock.advance(50 * time.Millisecond)
	tr.Reconcile()

	resumeRes, err := tr.Prepare(PrepareArgs{TaskID: "t1", Cwd: "/somewhere/else", Resume: recordID})
	if err != nil {
		t.Fatalf("Prepare resume: %v", err)
	}
	if resumeRes.SessionID != res.SessionID {
		t.Fatalf("resume claude session id = %q, want reused %q", resumeRes.SessionID, res.SessionID)
	}
	if resumeRes.Cwd != original {
		t.Fatalf("resume result cwd = %q, want the originally recorded cwd %q", resumeRes.Cwd, original)
	}
	if _, _, err := tr.Compose(resumeRes.TerminalID, "", resumeRes.Command); err != nil {
		t.Fatalf("Compose resume: %v", err)
	}
	sessions, _ = listSessions(tr)
	if sessions[0].Cwd != original {
		t.Fatalf("cwd after resume = %q, want the originally recorded cwd", sessions[0].Cwd)
	}
	if sessions[0].ID != recordID {
		t.Fatalf("record id changed across resume: got %q, want %q", sessions[0].ID, recordID)
	}
}

func TestTracker_HandleEventUpdatesAgentSessionIDOnClear(t *testing.T) {
	tr, store, live, _ := newTestTracker(t)
	res := composeAndSpawn(t, tr, live, PrepareArgs{TaskID: "t1", Cwd: "/repo"})
	sessions, _ := listSessions(tr)
	recordID := sessions[0].ID

	newClaudeID := "11111111-1111-1111-1111-111111111111"
	tr.HandleEvent(harness.AgentEvent{TerminalID: res.TerminalID, Event: "SessionStart", SessionID: newClaudeID})

	rec, err := store.Get(recordID)
	if err != nil || rec == nil {
		t.Fatalf("Get after /clear: %v %v", rec, err)
	}
	if rec.AgentSessionID != newClaudeID {
		t.Fatalf("agentSessionId after /clear = %q, want %q", rec.AgentSessionID, newClaudeID)
	}
}

func TestTracker_HandleEventUnknownTerminalIgnored(t *testing.T) {
	tr, _, _, _ := newTestTracker(t)
	// Must not panic or error for a terminal this tracker never Composed.
	tr.HandleEvent(harness.AgentEvent{TerminalID: "unknown", Event: "Stop"})
}

func TestTracker_SendWritesBracketedPasteThenEnter(t *testing.T) {
	tr, _, live, _ := newTestTracker(t)
	composeAndSpawn(t, tr, live, PrepareArgs{TaskID: "t1", Cwd: "/repo"})
	sessions, _ := listSessions(tr)
	recordID := sessions[0].ID

	if err := tr.Send(recordID, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(live.logs) != 2 {
		t.Fatalf("writes = %d, want 2 (paste, then enter)", len(live.logs))
	}
	if live.logs[1][len(live.logs[1])-1:] != "\r" {
		t.Fatalf("second write = %q, want to end with \\r", live.logs[1])
	}
}

func TestTracker_SendRefusedWhenNotRunning(t *testing.T) {
	tr, _, _, _ := newTestTracker(t)
	if err := tr.Send("no-such-record", "hello"); err != ErrSessionNotRunning {
		t.Fatalf("Send to an unknown record: err = %v, want ErrSessionNotRunning", err)
	}
}

// TestTracker_ConcurrentComposeReconcileHandleEvent is the race guard: three call sources
// (BoundService.Open's own goroutine via Compose, Registry.OnChange's own goroutine via Reconcile,
// the hooks listener's request-handler goroutine via HandleEvent) genuinely run concurrently in
// production. Run with -race.
func TestTracker_ConcurrentComposeReconcileHandleEvent(t *testing.T) {
	tr, _, live, _ := newTestTracker(t)

	const n = 20
	results := make([]PrepareResult, n)
	for i := 0; i < n; i++ {
		res, err := tr.Prepare(PrepareArgs{TaskID: "t1", Cwd: "/repo"})
		if err != nil {
			t.Fatalf("Prepare %d: %v", i, err)
		}
		results[i] = res
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		res := results[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := tr.Compose(res.TerminalID, "", res.Command); err != nil {
				t.Errorf("Compose: %v", err)
				return
			}
			live.add(res.TerminalID)
		}()
	}
	wg.Wait()

	var wg2 sync.WaitGroup
	for i := 0; i < n; i++ {
		res := results[i]
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			tr.HandleEvent(harness.AgentEvent{TerminalID: res.TerminalID, Event: "PreToolUse", ToolName: "Bash"})
			tr.Reconcile()
		}()
	}
	wg2.Wait()

	sessions, err := listSessions(tr)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != n {
		t.Fatalf("sessions = %d, want %d", len(sessions), n)
	}
}

func TestTracker_ResumeAfterAbandonedLaunchAndDoubleComposeGuard(t *testing.T) {
	tr, _, live, clock := newTestTracker(t)
	repo := t.TempDir()
	first := composeAndSpawn(t, tr, live, PrepareArgs{TaskID: "t1", Cwd: repo})
	sessions, _ := listSessions(tr)
	recordID := sessions[0].ID
	live.remove(first.TerminalID)
	clock.advance(50 * time.Millisecond)
	tr.Reconcile()

	args := PrepareArgs{TaskID: "t1", Cwd: repo, Resume: recordID}
	abandoned, err := tr.Prepare(args) // never composed: Open failed before ComposeAgent
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	a, err := tr.Prepare(args)
	if err != nil {
		t.Fatalf("retry after abandoned launch: err = %v, want nil", err)
	}
	if _, _, err := tr.Compose(abandoned.TerminalID, "", abandoned.Command); err != nil {
		t.Fatalf("superseded intent composed without error path: %v", err)
	}
	// The superseded intent is gone, so it took the no-intent hooks-only path and held nothing.
	if _, _, err := tr.Compose(a.TerminalID, "", a.Command); err != nil {
		t.Fatalf("Compose retry: %v", err)
	}
	live.add(a.TerminalID)

	// Two prepared before either composes: second Compose must refuse.
	live.remove(a.TerminalID)
	clock.advance(50 * time.Millisecond)
	tr.Reconcile()
	p1, _ := tr.Prepare(args)
	tr.mu.Lock()
	tr.pending["second"] = pendingIntent{RecordID: recordID, AgentSessionID: p1.SessionID, Command: p1.Command, Resume: true, CreatedAt: clock.Now()}
	tr.mu.Unlock()
	if _, _, err := tr.Compose(p1.TerminalID, "", p1.Command); err != nil {
		t.Fatalf("first Compose: %v", err)
	}
	if _, _, err := tr.Compose("second", "", p1.Command); err != ErrSessionRunning {
		t.Fatalf("second Compose: err = %v, want ErrSessionRunning", err)
	}
}

func TestTracker_TaskSessionComposeResumeAndStopped(t *testing.T) {
	tr, store, live, clock := newTestTracker(t)
	var mu sync.Mutex
	var stopped []string
	tr.deps.OnStopped = func(id string) { mu.Lock(); stopped = append(stopped, id); mu.Unlock() }
	cwd := t.TempDir()

	res, err := tr.Prepare(PrepareArgs{
		TaskID: "t1", BranchID: "b1", StepID: "s1", Cwd: cwd, Resumes: "claude-9",
		AddDirs: []string{"/x y"}, Message: "go on",
	})
	if err != nil {
		t.Fatal(err)
	}
	composed, _, err := tr.Compose(res.TerminalID, "", res.Command)
	if err != nil {
		t.Fatal(err)
	}
	if want := tr.deps.AgentBin + " --resume 'claude-9' '--add-dir' '/x y' -- 'go on'"; composed != want {
		t.Fatalf("composed = %q, want %q", composed, want)
	}
	rec, err := store.Get(res.RecordID)
	if err != nil || rec == nil {
		t.Fatalf("record = %v, %v", rec, err)
	}
	if rec.Mode != model.AdeSessionModeTUI || rec.TaskID != "t1" || rec.BranchID != "b1" ||
		rec.StepID != "s1" || rec.Resumes != "claude-9" || rec.AgentSessionID != "claude-9" || rec.RunID != "" {
		t.Fatalf("record = %+v", rec)
	}

	live.add(res.TerminalID)
	live.remove(res.TerminalID)
	clock.advance(time.Minute)
	tr.Reconcile()
	mu.Lock()
	got := append([]string(nil), stopped...)
	mu.Unlock()
	if len(got) != 1 || got[0] != res.RecordID {
		t.Fatalf("OnStopped = %v, want [%s]", got, res.RecordID)
	}

	if _, err := tr.Prepare(PrepareArgs{TaskID: "other", Resume: res.RecordID}); err != ErrSessionWrongTask {
		t.Fatalf("resume under another task: %v", err)
	}
	again, err := tr.Prepare(PrepareArgs{TaskID: "t1", Resume: res.RecordID})
	if err != nil {
		t.Fatal(err)
	}
	if again.RecordID != res.RecordID || again.Command != tr.deps.AgentBin+" --resume 'claude-9'" {
		t.Fatalf("resume = %+v", again)
	}
	if _, _, err := tr.Compose(again.TerminalID, "", again.Command); err != nil {
		t.Fatal(err)
	}
	if rec, _ = store.Get(res.RecordID); rec.State != model.AdeSessionStateRunning {
		t.Fatalf("resumed record state = %s", rec.State)
	}
}

func TestTracker_ResumeRefusesMissingOrNonDirectoryCwd(t *testing.T) {
	tr, _, live, clock := newTestTracker(t)
	missing := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(missing, 0o755); err != nil {
		t.Fatal(err)
	}
	res := composeAndSpawn(t, tr, live, PrepareArgs{TaskID: "t1", Cwd: missing})
	sessions, _ := listSessions(tr)
	recordID := sessions[0].ID
	live.remove(res.TerminalID)
	clock.advance(50 * time.Millisecond)
	tr.Reconcile()

	if err := os.RemoveAll(missing); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Prepare(PrepareArgs{TaskID: "t1", Resume: recordID}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("resume with a missing cwd: err = %v, want ErrInvalidInput", err)
	}

	fileCwd := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(fileCwd, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res2 := composeAndSpawn(t, tr, live, PrepareArgs{TaskID: "t1", Cwd: fileCwd})
	live.remove(res2.TerminalID)
	clock.advance(50 * time.Millisecond)
	tr.Reconcile()
	sessions, _ = listSessions(tr)
	var recordID2 string
	for _, s := range sessions {
		if s.ID != recordID {
			recordID2 = s.ID
		}
	}
	if _, err := tr.Prepare(PrepareArgs{TaskID: "t1", Resume: recordID2}); err != ErrResumeCwdNotDir {
		t.Fatalf("resume with a file cwd: err = %v, want ErrResumeCwdNotDir", err)
	}
}

func TestTracker_PrepareRequiresTask(t *testing.T) {
	tr, _, _, _ := newTestTracker(t)
	if _, err := tr.Prepare(PrepareArgs{Cwd: "/repo"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Prepare without a task: err = %v, want ErrInvalidInput", err)
	}
}

func TestTracker_AbortDeletesFreshAndStopsResumed(t *testing.T) {
	tr, _, live, clock := newTestTracker(t)
	repo := t.TempDir()
	var stopped []string
	tr.deps.OnStopped = func(id string) { stopped = append(stopped, id) }

	fresh, err := tr.Prepare(PrepareArgs{TaskID: "t1", Cwd: repo})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, _, err := tr.Compose(fresh.TerminalID, "", fresh.Command); err != nil {
		t.Fatalf("Compose: %v", err)
	}
	tr.Abort(fresh.TerminalID)
	if sessions, _ := listSessions(tr); len(sessions) != 0 {
		t.Fatalf("sessions after aborting a fresh launch = %d, want 0", len(sessions))
	}
	if len(stopped) != 1 || stopped[0] != fresh.RecordID {
		t.Fatalf("OnStopped after aborting a fresh launch = %v, want [%s]", stopped, fresh.RecordID)
	}

	first := composeAndSpawn(t, tr, live, PrepareArgs{TaskID: "t1", Cwd: repo})
	sessions, _ := listSessions(tr)
	recordID := sessions[0].ID
	live.remove(first.TerminalID)
	clock.advance(50 * time.Millisecond)
	tr.Reconcile()

	resumed, err := tr.Prepare(PrepareArgs{TaskID: "t1", Cwd: repo, Resume: recordID})
	if err != nil {
		t.Fatalf("Prepare resume: %v", err)
	}
	if _, _, err := tr.Compose(resumed.TerminalID, "", resumed.Command); err != nil {
		t.Fatalf("Compose resume: %v", err)
	}
	tr.Abort(resumed.TerminalID)
	sessions, _ = listSessions(tr)
	if len(sessions) != 1 || sessions[0].State != model.AdeSessionStateStopped {
		t.Fatalf("after aborting a resume: %+v, want one stopped row", sessions)
	}
	if _, _, err := tr.Compose("unknown", "", "claude"); err != nil {
		t.Fatalf("Compose: %v", err)
	}
	tr.Abort("unknown")
}

func TestTracker_HasPendingIgnoresAbandonedLaunch(t *testing.T) {
	tr, _, _, clock := newTestTracker(t)
	if _, err := tr.Prepare(PrepareArgs{TaskID: "t1", BranchID: "b1", Cwd: t.TempDir()}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	onBranch := func(p pendingIntent) bool { return p.BranchID == "b1" }
	if !tr.hasPending(onBranch) {
		t.Fatal("fresh intent not reported pending")
	}
	clock.advance(launchGuardWindow + time.Second)
	if tr.hasPending(onBranch) {
		t.Fatal("abandoned intent still blocks a retry")
	}
}

func TestTracker_ExpiredPendingIntentReleasesItsRecord(t *testing.T) {
	tr, _, _, clock := newTestTracker(t)
	var stopped []string
	tr.SetStoppedHandler(func(id string) { stopped = append(stopped, id) })

	abandoned, err := tr.Prepare(PrepareArgs{TaskID: "t1", Cwd: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(2 * time.Minute)
	if _, err := tr.Prepare(PrepareArgs{TaskID: "t1", Cwd: "/repo"}); err != nil {
		t.Fatal(err)
	}
	if len(stopped) != 1 || stopped[0] != abandoned.RecordID {
		t.Fatalf("OnStopped calls = %v, want only %s", stopped, abandoned.RecordID)
	}
}
