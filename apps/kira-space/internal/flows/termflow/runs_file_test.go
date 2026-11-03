package termflow_test

import (
	"encoding/json"
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
	if err := os.WriteFile(filepath.Join(app.SpaceHome, "runs", file), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunsConfigFile(t *testing.T) {
	app := flowharness.New(t)
	cs := app.W.CustomScripts

	agents, err := os.ReadFile(filepath.Join(app.SpaceHome, "runs", "AGENTS.md"))
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
	if err := os.Remove(filepath.Join(app.SpaceHome, "runs", "deploy.yaml")); err != nil {
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

// TestRunsFileMigration: legacy rows become files on the first start after the schema change.
func TestRunsFileMigration(t *testing.T) {
	app := flowharness.New(t)
	db := app.DB()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	smart, _ := json.Marshal(scripts.Smart{Model: "sonnet"})
	for _, r := range []struct {
		id, name, kind, cmd, dirMode, dir, collection, smart string
		sort                                                 int
		task, branch                                         string
	}{
		{"11111111-aaaa-4aaa-8aaa-111111111111", "Legacy", "script", "echo legacy", "home", "", "Old", "", 1, "", ""},
		{"22222222-bbbb-4bbb-8bbb-222222222222", "Brain", "smart", "Think", "kira", "", "", string(smart), 2, "task-1", "branch-1"},
	} {
		if r.collection != "" {
			if _, err := db.Exec(`INSERT OR IGNORE INTO custom_script_collections (name, sort_order) VALUES (?, 0)`, r.collection); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.Exec(`INSERT INTO custom_scripts (id, name, command, kind, dir_mode, working_dir, color, collection, sort_order, schedule_task_id, schedule_branch_id, smart_json, config_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 'blue', ?, ?, ?, ?, ?, '', 1, 1)`,
			r.id, r.name, r.cmd, r.kind, r.dirMode, r.dir, r.collection, r.sort, r.task, r.branch, r.smart); err != nil {
			t.Fatalf("seed %s: %v", r.name, err)
		}
	}
	keyed, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "Keyed", Command: "true", Color: "blue", Params: []scripts.Param{{Name: "token", Type: scripts.ParamText, Secret: true}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.CustomScripts.SetSecrets(bridge.CustomScriptsSetSecretsArgs{ID: keyed.ID, Values: map[string]*string{"token": ptr("s3cret")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE custom_scripts SET config_json = '' WHERE id = ?`, keyed.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(app.SpaceHome, "runs", keyed.ID+".yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM settings WHERE key = 'config.migrated.runs'`); err != nil {
		t.Fatal(err)
	}
	app.Restart()

	snap, err := app.W.CustomScripts.List()
	if err != nil {
		t.Fatal(err)
	}
	legacy, ok := scriptOf(snap, "11111111-aaaa-4aaa-8aaa-111111111111")
	brain, ok2 := scriptOf(snap, "22222222-bbbb-4bbb-8bbb-222222222222")
	if !ok || !ok2 || len(snap.Problems) != 0 {
		t.Fatalf("snapshot = %+v", snap)
	}
	if legacy.DirMode != scripts.DirModeFixed || legacy.WorkingDir != home || legacy.Collection != "Old" || legacy.Command != "echo legacy" {
		t.Fatalf("legacy record = %+v, want fixed home and collection Old", legacy)
	}
	if brain.Kind != scripts.KindSmart || brain.Smart == nil || brain.Smart.Model != "sonnet" {
		t.Fatalf("smart record = %+v", brain)
	}
	for _, id := range []string{legacy.ID, brain.ID} {
		if _, err := os.Stat(filepath.Join(app.SpaceHome, "runs", id+".yaml")); err != nil {
			t.Fatalf("legacy run %s not exported: %v", id, err)
		}
	}
	var task, branch string
	if err := app.DB().QueryRow(`SELECT schedule_task_id, schedule_branch_id FROM custom_scripts WHERE id = ?`, brain.ID).Scan(&task, &branch); err != nil {
		t.Fatal(err)
	}
	if task != "task-1" || branch != "branch-1" {
		t.Fatalf("schedule target = %q/%q, want task-1/branch-1 kept", task, branch)
	}
	if keyed, ok := scriptOf(snap, keyed.ID); !ok || len(keyed.SavedSecrets) != 1 || keyed.SavedSecrets[0] != "token" {
		t.Fatalf("keyed record = %+v, want its saved secret kept", keyed)
	}

	if err := os.Remove(filepath.Join(app.SpaceHome, "runs", legacy.ID+".yaml")); err != nil {
		t.Fatal(err)
	}
	app.Restart()
	snap, err = app.W.CustomScripts.List()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := scriptOf(snap, legacy.ID); ok {
		t.Fatal("deleted file's run is back after a restart")
	}
	if _, err := os.Stat(filepath.Join(app.SpaceHome, "runs", legacy.ID+".yaml")); !os.IsNotExist(err) {
		t.Fatalf("deleted file was exported again: %v", err)
	}
	if _, ok := scriptOf(snap, brain.ID); !ok {
		t.Fatal("kept run is gone")
	}
}
