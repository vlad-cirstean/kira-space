package notifyflow_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentnotify"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

var ctx = context.Background()

const (
	waitFor = 20 * time.Second
	window  = "w-notify"
)

// recSink records every note the Notifier posts.
type recSink struct {
	mu    sync.Mutex
	notes []agentnotify.Note
}

func (s *recSink) Send(n agentnotify.Note) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notes = append(s.notes, n)
	return nil
}

func (s *recSink) all() []agentnotify.Note {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]agentnotify.Note(nil), s.notes...)
}

// ofKind returns the notes of one kind.
func (s *recSink) ofKind(k agentnotify.Kind) []agentnotify.Note {
	var out []agentnotify.Note
	for _, n := range s.all() {
		if n.Kind == k {
			out = append(out, n)
		}
	}
	return out
}

// only fails unless exactly one note was posted, and returns it.
func (s *recSink) only(t *testing.T) agentnotify.Note {
	t.Helper()
	got := s.all()
	if len(got) != 1 {
		t.Fatalf("notes = %+v, want exactly one", got)
	}
	return got[0]
}

func (s *recSink) none(t *testing.T) {
	t.Helper()
	if got := s.all(); len(got) != 0 {
		t.Fatalf("notes = %+v, want none", got)
	}
}

// newApp boots the app with a recording sink and the window known to the window manager.
func newApp(t *testing.T, opts ...flowharness.Opt) (*flowharness.App, *recSink) {
	t.Helper()
	app := flowharness.New(t, opts...)
	sink := &recSink{}
	app.W.AgentNotify.SetSink(sink)
	app.WindowMgr.Known[window] = true
	return app, sink
}

func setCooldown(t *testing.T, d time.Duration) {
	t.Helper()
	old := agentnotify.Cooldown
	agentnotify.Cooldown = d
	t.Cleanup(func() { agentnotify.Cooldown = old })
}

func importRepo(t *testing.T, app *flowharness.App, dir string) model.CodeRepo {
	t.Helper()
	rec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: dir})
	if err != nil {
		t.Fatalf("ImportRepo %s: %v", dir, err)
	}
	return rec
}

// openAgent opens a Claude Code terminal tab in window, as the renderer does.
func openAgent(t *testing.T, app *flowharness.App, id, cwd string) {
	t.Helper()
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: id, WindowKey: window, Cwd: cwd, Cols: 100, Rows: 30,
		Command: "claude", LaunchKind: terminal.LaunchKindAgent,
	}); err != nil {
		t.Fatalf("Terminal.Open %s: %v", id, err)
	}
}

// hook pipes one hook payload through the real shim with the terminal's launch env, the way a
// running claude does. curl returns after the listener answered, so the event is handled on return.
func hook(t *testing.T, app *flowharness.App, terminalID, payload string) {
	t.Helper()
	_, env, _ := app.W.AgentHooks.Compose("claude", terminalID, "", "claude", "")
	st := app.W.AgentHooks.Status("claude")
	if !st.Running || len(env) == 0 {
		t.Fatalf("hooks not running: %+v", st)
	}
	cmd := exec.Command(filepath.Join(filepath.Dir(st.SettingsPath), "hook"))
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(payload)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook shim: %v\n%s", err, out)
	}
}

func stop(t *testing.T, app *flowharness.App, id, cwd, reply string) {
	t.Helper()
	hook(t, app, id, `{"hook_event_name":"Stop","session_id":"s","cwd":`+quote(cwd)+`,"last_assistant_message":`+quote(reply)+`}`)
}

func event(t *testing.T, app *flowharness.App, id, name string) {
	t.Helper()
	hook(t, app, id, `{"hook_event_name":"`+name+`","session_id":"s"}`)
}

func quote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func focus(t *testing.T, app *flowharness.App, f bridge.ReportFocusArgs) {
	t.Helper()
	f.WindowKey = window
	if err := app.W.AgentNotifySvc.ReportFocus(f); err != nil {
		t.Fatalf("ReportFocus: %v", err)
	}
}

func setNotify(t *testing.T, app *flowharness.App, p model.AgentPatch) {
	t.Helper()
	if _, err := app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{Agent: &p}}); err != nil {
		t.Fatalf("Settings.Set: %v", err)
	}
}

func off() *bool { v := false; return &v }

func claude(app *flowharness.App, byRepo map[string][]fakeagent.Action) {
	app.Scenario(fakeagent.Scenario{Claude: byRepo})
}

func cloneWithMain(app *flowharness.App, dir string) *flowharness.Repo {
	name := filepath.Base(dir)
	bare := app.NewBare(name + "-remote")
	seed := app.NewRepo(name + "-seed")
	seed.Commit("base", map[string]string{"base.txt": "base\n"})
	bare.PushFrom(seed)
	return bare.Clone(dir)
}

const flowSteps = "id: flow\nname: Flow\nsteps:\n" +
	"  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: work on {branch} in {repo}\n" +
	"  - id: work\n    name: Work\n    kind: manual\n    status: in-progress\n    session: true\n"

// adeTask imports a clone, saves the two-step workflow and creates the task "Fix login".
func adeTask(t *testing.T, app *flowharness.App) (adewire.Task, model.CodeRepo) {
	t.Helper()
	repo := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	entry, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "flow.yaml", Yaml: flowSteps})
	if err != nil || entry.Error != nil {
		t.Fatalf("SaveWorkflowYaml: %v %+v", err, entry.Error)
	}
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Fix login", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task, rec
}

func waitRunDone(t *testing.T, app *flowharness.App, taskID string) {
	t.Helper()
	testx.WaitUntil(t, waitFor, func() bool {
		b, err := app.W.AdeTask.Board(ctx)
		if err != nil {
			return false
		}
		for _, tk := range b.Tasks {
			if tk.ID != taskID {
				continue
			}
			for _, r := range tk.Runs {
				if r.State == "done" {
					return true
				}
			}
		}
		return false
	})
}

func startRun(t *testing.T, app *flowharness.App, task adewire.Task, repoID string) {
	t.Helper()
	b, err := app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, br := range b.Branches {
		if br.TaskID == task.ID && br.CodeRepoID == repoID {
			names[br.ID] = "feat/api-work"
		}
	}
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: names}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
}

// openAgent2 opens a manual step or take-over launch's terminal, as the renderer does.
func openAgent2(t *testing.T, app *flowharness.App, l adewire.Launch) {
	t.Helper()
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: l.TerminalID, WindowKey: window, Cwd: l.Cwd, Cols: 100, Rows: 30,
		Command: l.Command, LaunchKind: terminal.LaunchKindAgent,
	}); err != nil {
		t.Fatalf("Terminal.Open %s: %v", l.Command, err)
	}
}
