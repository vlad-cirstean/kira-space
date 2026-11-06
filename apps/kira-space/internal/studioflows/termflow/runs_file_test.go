package termflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/configfs/configsvc"
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
`

var runsMasks = []studioflowharness.ContractOption{studioflowharness.Mask("createdAt", "updatedAt", "hash")}

func waitSnapshot(t *testing.T, app *studioflowharness.App, mark int, ok func(scripts.Snapshot) bool) scripts.Snapshot {
	t.Helper()
	var got scripts.Snapshot
	testx.WaitUntil(t, wait, func() bool {
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

func writeRun(t *testing.T, app *studioflowharness.App, file, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(app.KiraHome, "runs", file), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRunsConfigFileStudio: Studio's watcher and config service serve the runs folder; the Space
// task fields are refused.
func TestRunsConfigFileStudio(t *testing.T) {
	app := studioflowharness.New(t)
	cs := app.W.CustomScripts

	mark := app.Events.Mark()
	writeRun(t, app, "deploy.yaml", deployYAML)
	snap := waitSnapshot(t, app, mark, func(s scripts.Snapshot) bool { _, ok := scriptOf(s, "deploy"); return ok })
	if rec, _ := scriptOf(snap, "deploy"); rec.Name != "Deploy" || rec.Collection != "Ops" || len(snap.Problems) != 0 {
		t.Fatalf("record = %+v, problems %+v", rec, snap.Problems)
	}
	app.Contract(t, "runs-file", "snapshot#valid", snap, runsMasks...)

	edited := strings.NewReplacer("name: Deploy", "name: Ship", "echo deploy {env}", "echo ship {env}").Replace(deployYAML)
	mark = app.Events.Mark()
	writeRun(t, app, "deploy.yaml", edited)
	snap = waitSnapshot(t, app, mark, func(s scripts.Snapshot) bool { r, _ := scriptOf(s, "deploy"); return r.Name == "Ship" })
	app.Contract(t, "runs-file", "snapshot#edited", snap, runsMasks...)
	view, err := cs.File(bridge.CustomScriptsFileArgs{File: "deploy.yaml"})
	if err != nil || view.Text != edited || view.Problem != nil {
		t.Fatalf("File = %+v, %v", view, err)
	}
	app.Contract(t, "runs-file", "file#edited", view, runsMasks...)

	bad := strings.Replace(edited, "color: blue", "kind: bogus", 1)
	mark = app.Events.Mark()
	writeRun(t, app, "deploy.yaml", bad)
	snap = waitSnapshot(t, app, mark, func(s scripts.Snapshot) bool { return len(s.Problems) == 1 })
	kept, _ := scriptOf(snap, "deploy")
	if p := snap.Problems[0]; kept.Name != "Ship" || kept.Command != "echo ship {env}" || p.Line != 3 || !p.LastGood {
		t.Fatalf("after a bad edit: record %+v, problem %+v, want the last good record and a line-3 problem", kept, p)
	}
	app.Contract(t, "runs-file", "snapshot#bad", snap, runsMasks...)
	badView, err := cs.File(bridge.CustomScriptsFileArgs{File: "deploy.yaml"})
	if err != nil || badView.Problem == nil || badView.Problem.Line != 3 {
		t.Fatalf("File of a bad file = %+v, %v", badView, err)
	}
	app.Contract(t, "runs-file", "file#bad", badView, runsMasks...)
	startTerminalRun(t, app, "t-bad", scriptruns.RunArgs{ScriptID: "deploy"})
	if r := runOf(t, app, "t-bad"); r.Command != kept.Command {
		t.Fatalf("run command = %q, want the last good command", r.Command)
	}

	mark = app.Events.Mark()
	writeRun(t, app, "deploy.yaml", edited+"use_ade_dir: true\n")
	line := strings.Count(edited, "\n") + 1
	snap = waitSnapshot(t, app, mark, func(s scripts.Snapshot) bool { return len(s.Problems) == 1 && s.Problems[0].Line == line })
	if r, _ := scriptOf(snap, "deploy"); r.Name != "Ship" {
		t.Fatalf("use_ade_dir replaced the last good record: %+v", r)
	}

	if err := app.W.ConfigFiles.Open(configsvc.FileArgs{Folder: "runs", File: "deploy.yaml"}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(app.KiraHome, "runs", "deploy.yaml")
	if opened := app.FileOpener.OpenedFiles(); len(opened) != 1 || opened[0] != want {
		t.Fatalf("opened = %v, want [%s]", opened, want)
	}
}
