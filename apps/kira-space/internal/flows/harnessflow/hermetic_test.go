package harnessflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// TestHermeticOpenCode: a hermetic OpenCode TUI launch sees a redirected config home that links
// every user config entry but opencode, carries the hermetic env, drops the dir when the terminal
// closes, and leaves a non-hermetic launch on the real config home.
func TestHermeticOpenCode(t *testing.T) {
	for _, on := range []bool{true, false} {
		name := "hermetic"
		if !on {
			name = "off"
		}
		t.Run(name, func(t *testing.T) { hermeticOpenCode(t, on) })
	}
}

func hermeticOpenCode(t *testing.T, on bool) {
	app := flowharness.New(t, flowharness.WithOpenCode(), flowharness.WithTrackerGrace(time.Second))
	if !on {
		app.SetHermetic(t, false)
	}
	for _, f := range []string{".config/opencode/opencode.json", ".config/git/config"} {
		p := filepath.Join(app.Home, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	repoRec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: repo.Dir})
	if err != nil {
		t.Fatal(err)
	}
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: window})

	start := func(name string) adewire.Launch {
		t.Helper()
		tk := task(t, app, repoRec.ID, name, ocWorkflow(name, ""))
		br, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: branchOf(t, app, tk.ID, repoRec.ID).ID, Name: "feat/" + name})
		if err != nil {
			t.Fatal(err)
		}
		l, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br.ID, Provider: "opencode"})
		if err != nil {
			t.Fatal(err)
		}
		openTUI(t, app, l)
		return l
	}

	l := start("herm")
	if !on {
		testx.WaitUntil(t, waitFor, func() bool { _, err := os.Stat(filepath.Join(app.FakeDir, "opencode-1.prompt")); return err == nil })
		if rec := fakeRead(t, app, "opencode-1.hermetic"); strings.Contains(rec, "-iso/") || strings.Contains(rec, "OPENCODE_DISABLE_CLAUDE_CODE_PROMPT") {
			t.Fatalf("record = %q, want the real config home and no hermetic env", rec)
		}
		if strings.HasPrefix(l.Command, "/usr/bin/env ") {
			t.Fatalf("command = %q, want plain opencode", l.Command)
		}
		return
	}
	rec := waitFake(t, app, "opencode-1.hermetic")
	var xdg, entries string
	for _, line := range strings.Split(rec, "\n") {
		switch {
		case strings.HasPrefix(line, "XDG_CONFIG_HOME="):
			xdg = strings.TrimPrefix(line, "XDG_CONFIG_HOME=")
		case strings.HasPrefix(line, "entries="):
			entries = strings.TrimPrefix(line, "entries=")
		}
	}
	if !strings.HasSuffix(xdg, "-iso/xdg") {
		t.Fatalf("XDG_CONFIG_HOME = %q, want the run's isolation dir", xdg)
	}
	has := map[string]bool{}
	for _, e := range strings.Split(entries, ",") {
		has[e] = true
	}
	if !has["git"] || has["opencode"] {
		t.Fatalf("config home entries = %q, want git linked and opencode absent", entries)
	}
	if !strings.Contains(rec, "OPENCODE_DISABLE_CLAUDE_CODE_PROMPT=1") {
		t.Fatalf("record = %q, want OPENCODE_DISABLE_CLAUDE_CODE_PROMPT=1", rec)
	}
	if strings.Contains(rec, "OPENCODE_DISABLE_PROJECT_CONFIG") {
		t.Fatalf("record = %q, project config must stay on", rec)
	}
	if strings.Contains(fakeRead(t, app, "opencode-1.args"), "--pure") {
		t.Fatal("hermetic launch passed --pure")
	}

	// Closing the terminal drops the isolation dir.
	iso := filepath.Dir(xdg)
	testx.WaitUntil(t, waitFor, func() bool { return sessionByID(t, app, l.SessionID).State == "running" })
	if err := app.W.Terminal.Close(terminal.CloseArgs{TerminalID: l.TerminalID}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return sessionByID(t, app, l.SessionID).State == "stopped" })
	testx.WaitUntil(t, waitFor, func() bool { _, err := os.Stat(iso); return os.IsNotExist(err) })
}
