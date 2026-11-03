package termflow_test

import (
	"encoding/base64"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/scripts/scriptstest"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const (
	waitFor = 20 * time.Second
	window  = "w-term"
)

// output is every byte the terminal pushed to its window so far.
func output(app *flowharness.App, terminalID string) string {
	var sb strings.Builder
	for _, ev := range app.Events.Since(0, appevent.ChannelTerminal) {
		e, ok := ev.Data.(terminal.Event)
		if !ok || e.TerminalID != terminalID || ev.Window != window {
			continue
		}
		raw, _ := base64.StdEncoding.DecodeString(e.Data)
		sb.Write(raw)
	}
	return sb.String()
}

var escapes = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// hasLine reports whether out holds want as a whole line once escape sequences are stripped.
func hasLine(out, want string) bool {
	for _, l := range strings.FieldsFunc(escapes.ReplaceAllString(out, ""), func(r rune) bool { return r == '\n' || r == '\r' }) {
		if l == want {
			return true
		}
	}
	return false
}

func exitEvent(app *flowharness.App, terminalID string) (terminal.Event, bool) {
	for _, ev := range app.Events.Since(0, appevent.ChannelTerminal) {
		if e, ok := ev.Data.(terminal.Event); ok && e.TerminalID == terminalID && e.Exited {
			return e, true
		}
	}
	return terminal.Event{}, false
}

func TestShellTabRunsGit(t *testing.T) {
	app := flowharness.New(t)
	repo := app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "a\n"})
	repo.Branch("topic-xyz", "main")
	repo.Checkout("topic-xyz")

	if got := app.W.Terminal.DefaultCwd(); got.Path != app.Home {
		t.Fatalf("DefaultCwd = %q, want the isolated home %q", got.Path, app.Home)
	}
	const id = "tab-1"
	opened, err := app.W.Terminal.Open(terminal.OpenArgs{TerminalID: id, WindowKey: window, Cwd: repo.Dir, Cols: 100, Rows: 30})
	if err != nil || opened.Shell == "" {
		t.Fatalf("Open = %+v, %v", opened, err)
	}
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{TerminalID: id, WindowKey: window, Cwd: repo.Dir, Cols: 100, Rows: 30}); err == nil {
		t.Fatal("a second Open with the same terminal id succeeded")
	}
	if n := len(app.W.TermRegistry.AgentSessions()); n != 0 {
		t.Fatalf("a shell tab counts as %d agent sessions", n)
	}

	cmd := base64.StdEncoding.EncodeToString([]byte("git rev-parse --abbrev-ref HEAD\n"))
	if err := app.W.Terminal.Write(terminal.WriteArgs{TerminalID: id, Data: cmd}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return hasLine(output(app, id), "topic-xyz") })

	if err := app.W.Terminal.Resize(terminal.ResizeArgs{TerminalID: id, Cols: 120, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	if err := app.W.Terminal.Resize(terminal.ResizeArgs{TerminalID: id, Cols: 0, Rows: 40}); err == nil {
		t.Fatal("Resize accepted zero columns")
	}
	size := base64.StdEncoding.EncodeToString([]byte("stty size\n"))
	if err := app.W.Terminal.Write(terminal.WriteArgs{TerminalID: id, Data: size}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return hasLine(output(app, id), "40 120") })

	if err := app.W.Terminal.Close(terminal.CloseArgs{TerminalID: id}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { _, ok := exitEvent(app, id); return ok })
	if err := app.W.Terminal.Close(terminal.CloseArgs{TerminalID: id}); err != nil {
		t.Fatalf("second Close = %v, want idempotent", err)
	}
	testx.WaitUntil(t, waitFor, func() bool { _, ok := app.W.TermRegistry.WindowOf(id); return !ok })
}

func TestCollectionsMoveAndDelete(t *testing.T) {
	app := flowharness.New(t)
	svc := app.W.CustomScripts
	mark := app.Events.Mark()
	mk := func(name string) scripts.CustomScript {
		s, err := svc.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
			Name: name, Command: "echo " + name, Color: "blue", Collection: "Build",
		}})
		if err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
		return s
	}
	keep := mk("keep")
	drop := mk("drop")
	// Contract commands: tests/ui/automations-scripts.spec.ts "contract: a collection with its scripts
	// shows after boot" reads the same fixture.
	grouped, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, "commands", "CustomScriptsService.List#grouped", grouped)

	scriptstest.Update(t, svc, keep.ID, scripts.CustomScriptFields{Name: keep.Name, Kind: keep.Kind, Command: keep.Command, Color: keep.Color})
	snap, err := svc.List()
	if err != nil || len(snap.Collections) != 1 || len(snap.Scripts) != 2 {
		t.Fatalf("List = %+v, %v", snap, err)
	}
	for _, s := range snap.Scripts {
		if (s.ID == keep.ID) == (s.Collection == "Build") {
			t.Fatalf("script %s collection = %q, want keep ungrouped and drop in Build", s.Name, s.Collection)
		}
	}

	if err := svc.Delete(bridge.CustomScriptsDeleteArgs{ID: drop.ID}); err != nil {
		t.Fatal(err)
	}
	snap, err = svc.List()
	if err != nil || len(snap.Collections) != 0 || len(snap.Scripts) != 1 || snap.Scripts[0].ID != keep.ID {
		t.Fatalf("after Delete: %+v, %v, want only the moved-out script and the emptied collection pruned", snap, err)
	}

	last := app.Events.Since(mark, bridge.ChannelCustomScriptsChanged)
	if len(last) == 0 {
		t.Fatal("no changed events")
	}
	var final scripts.Snapshot
	last[len(last)-1].Decode(t, &final)
	if len(final.Collections) != 0 || len(final.Scripts) != 1 {
		t.Fatalf("last changed event = %+v, want the full snapshot", final)
	}
}

func ptr[T any](v T) *T { return &v }

func TestAgentTabCountsForKeepAwake(t *testing.T) {
	app := flowharness.New(t)
	hold := filepath.Join(t.TempDir(), "hold")
	app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{"*": {{WaitFile: hold}}}})
	on := true
	if _, err := app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{Agent: &model.AgentPatch{KeepAwakeWithAgents: &on}}}); err != nil {
		t.Fatal(err)
	}
	if app.KeepAwake.Held() {
		t.Fatal("assertion held before any agent runs")
	}

	const id = "agent-1"
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: id, WindowKey: window, Cwd: app.Home, Cols: 100, Rows: 30,
		Command: "claude", LaunchKind: terminal.LaunchKindAgent,
	}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, app.KeepAwake.Held)
	if st := app.W.KeepAwake.Status(); st.Manual {
		t.Fatalf("status = %+v, want the manual reason off while the agent reason holds", st)
	}

	if err := app.W.Terminal.Close(terminal.CloseArgs{TerminalID: id}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return !app.KeepAwake.Held() })

	// A shell tab does not count.
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{TerminalID: "shell-1", WindowKey: window, Cwd: app.Home, Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if app.KeepAwake.Held() {
		t.Fatal("a plain shell tab holds the assertion")
	}
}
