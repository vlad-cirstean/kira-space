package termflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const deployYAML = `name: Deploy
collection: Ops
color: blue
command: echo deploy {env}
params:
  - name: env
    type: select
    options: [staging, prod]
    default: staging
  - name: token
    type: text
    secret: true
`

var runsMasks = []flowharness.ContractOption{flowharness.Mask("createdAt", "updatedAt", "hash")}

// waitSnapshot waits for a changed event after mark whose snapshot satisfies ok.
func waitSnapshot(t *testing.T, app *flowharness.App, mark int, ok func(scripts.Snapshot) bool) scripts.Snapshot {
	t.Helper()
	var got scripts.Snapshot
	testx.WaitUntil(t, waitFor, func() bool {
		evs := app.Events.Since(mark, bridge.ChannelCustomScriptsChanged)
		if len(evs) == 0 {
			return false
		}
		var snap scripts.Snapshot
		evs[len(evs)-1].Decode(t, &snap)
		if !ok(snap) {
			return false
		}
		got = snap
		return true
	})
	return got
}

func scriptOf(snap scripts.Snapshot, id string) (scripts.CustomScript, bool) {
	for _, s := range snap.Scripts {
		if s.ID == id {
			return s, true
		}
	}
	return scripts.CustomScript{}, false
}

func writeRun(t *testing.T, app *flowharness.App, file, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(app.ConfigDir("runs"), file), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunsConfigFile(t *testing.T) {
	app := flowharness.New(t)
	cs := app.W.CustomScripts

	agents, err := os.ReadFile(filepath.Join(app.ConfigDir("runs"), "AGENTS.md"))
	if err != nil || !strings.Contains(string(agents), "smart.model") {
		t.Fatalf("AGENTS.md = %q, %v, want the field docs", agents, err)
	}

	mark := app.Events.Mark()
	writeRun(t, app, "deploy.yaml", deployYAML)
	snap := waitSnapshot(t, app, mark, func(s scripts.Snapshot) bool { _, ok := scriptOf(s, "deploy"); return ok })
	rec, _ := scriptOf(snap, "deploy")
	if rec.Name != "Deploy" || rec.Collection != "Ops" || len(rec.Params) != 2 || len(snap.Problems) != 0 {
		t.Fatalf("record = %+v, problems %+v", rec, snap.Problems)
	}
	app.Contract(t, "runs-file", "snapshot#valid", snap, runsMasks...)

	withToken, err := cs.SetSecrets(bridge.CustomScriptsSetSecretsArgs{ID: "deploy", Values: map[string]*string{"token": ptr("s3cret")}})
	if err != nil || len(withToken.SavedSecrets) != 1 || withToken.SavedSecrets[0] != "token" {
		t.Fatalf("SetSecrets = %+v, %v", withToken, err)
	}

	edited := strings.NewReplacer("name: Deploy", "name: Ship", "echo deploy {env}", "echo ship {env}").Replace(deployYAML)
	mark = app.Events.Mark()
	writeRun(t, app, "deploy.yaml", edited)
	snap = waitSnapshot(t, app, mark, func(s scripts.Snapshot) bool { r, _ := scriptOf(s, "deploy"); return r.Name == "Ship" })
	app.Contract(t, "runs-file", "snapshot#edited", snap, runsMasks...)
	view, err := cs.File(bridge.CustomScriptsFileArgs{File: "deploy.yaml"})
	if err != nil || view.Text != edited || view.Problem != nil || !view.Editable {
		t.Fatalf("File = %+v, %v", view, err)
	}
	app.Contract(t, "runs-file", "file#edited", view, runsMasks...)

	bad := strings.Replace(edited, "color: blue", "kind: bogus", 1)
	badAt := strings.Index(bad, "kind: bogus")
	if badAt < 0 {
		t.Fatalf("fixture lacks %q", "kind: bogus")
	}
	if line := 1 + strings.Count(bad[:badAt], "\n"); line != 3 {
		t.Fatalf("fixture line = %d", line)
	}
	mark = app.Events.Mark()
	writeRun(t, app, "deploy.yaml", bad)
	snap = waitSnapshot(t, app, mark, func(s scripts.Snapshot) bool { return len(s.Problems) == 1 })
	kept, _ := scriptOf(snap, "deploy")
	p := snap.Problems[0]
	if kept.Name != "Ship" || kept.Command != "echo ship {env}" || len(kept.SavedSecrets) != 1 ||
		p.File != "deploy.yaml" || p.ID != "deploy" || p.Line != 3 || !p.LastGood {
		t.Fatalf("after a bad edit: record %+v, problem %+v, want the last good record and a line-3 problem", kept, p)
	}
	app.Contract(t, "runs-file", "snapshot#bad", snap, runsMasks...)
	badView, err := cs.File(bridge.CustomScriptsFileArgs{File: "deploy.yaml"})
	if err != nil || badView.Problem == nil || badView.Problem.Line != 3 || !badView.LastGood {
		t.Fatalf("File of a bad file = %+v, %v", badView, err)
	}
	app.Contract(t, "runs-file", "file#bad", badView, runsMasks...)

	startTerminalRun(t, app, "t-bad", scriptruns.RunArgs{ScriptID: "deploy"})
	if r := runOf(t, app, "t-bad"); r.Command != kept.Command {
		t.Fatalf("run command = %q, want the last good command", r.Command)
	}

	stale := bridge.CustomScriptsSaveFileArgs{File: "deploy.yaml", Text: edited, BaseHash: "stale"}
	if _, err := cs.SaveFile(stale); errCode(err) != "E_CONFLICT" {
		t.Fatalf("SaveFile with a stale hash = %v, want E_CONFLICT", err)
	}
	save := bridge.CustomScriptsSaveFileArgs{File: "deploy.yaml", Text: edited, BaseHash: badView.Hash}
	app.Contract(t, "runs-file", "args:CustomScriptsService.SaveFile", save, runsMasks...)
	saved, err := cs.SaveFile(save)
	if err != nil || saved.Problem != nil {
		t.Fatalf("SaveFile = %+v, %v", saved, err)
	}
	app.Contract(t, "runs-file", "file#saved", saved, runsMasks...)
	snap = waitSnapshot(t, app, 0, func(s scripts.Snapshot) bool { return len(s.Problems) == 0 })
	if r, _ := scriptOf(snap, "deploy"); r.Name != "Ship" {
		t.Fatalf("after the fix: %+v", r)
	}

	mark = app.Events.Mark()
	if err := os.Remove(filepath.Join(app.ConfigDir("runs"), "deploy.yaml")); err != nil {
		t.Fatal(err)
	}
	waitSnapshot(t, app, mark, func(s scripts.Snapshot) bool { _, ok := scriptOf(s, "deploy"); return !ok })
	mark = app.Events.Mark()
	writeRun(t, app, "deploy.yaml", deployYAML)
	snap = waitSnapshot(t, app, mark, func(s scripts.Snapshot) bool { _, ok := scriptOf(s, "deploy"); return ok })
	if r, _ := scriptOf(snap, "deploy"); len(r.SavedSecrets) != 0 {
		t.Fatalf("recreated run keeps saved secrets %v, want none", r.SavedSecrets)
	}

	mark = app.Events.Mark()
	writeRun(t, app, "bad stem.yaml", "name: X\ncommand: true\n")
	snap = waitSnapshot(t, app, mark, func(s scripts.Snapshot) bool { return len(s.Problems) == 1 })
	if p := snap.Problems[0]; p.File != "bad stem.yaml" || p.ID != "" || len(snap.Scripts) != 1 {
		t.Fatalf("bad stem: problem %+v, scripts %d, want an id-less problem and no record", p, len(snap.Scripts))
	}
}
