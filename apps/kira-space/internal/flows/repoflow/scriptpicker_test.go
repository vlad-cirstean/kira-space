package repoflow_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

func setEdit(value any, path ...string) configfs.Edit {
	return configfs.Edit{Path: path, Value: value}
}

func unsetEdit(path ...string) configfs.Edit {
	return configfs.Edit{Path: path, Unset: true}
}

// TestRepoScriptPicker: the repo form picks a Run script for the prepare hook and a deployed-SHA
// row. The edits are what the form emits (tests/ui/repo-script-picker.spec.ts is the producer, so
// a mismatch fixes this list). A param keeps its one-or-list shape, and a reference to a smart,
// missing or unknown-param script still decodes: those are run-time checks.
func TestRepoScriptPicker(t *testing.T) {
	app := flowharness.New(t)
	r := app.NewRepo("proj")
	r.Commit("init", map[string]string{"a.txt": "a\n"})
	rec := importVia(t, app, r.Dir)
	key := rec.RepoID
	svc := app.W.RepoConfigSvc
	mask := flowharness.Mask("hash", "baseHash", "filePath", "id", "createdAt", "updatedAt")

	tok := "p515-saved-token"
	create := func(f scripts.CustomScriptFields, secret map[string]*string) {
		t.Helper()
		f.Color = "blue"
		if _, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: f, SecretValues: secret}); err != nil {
			t.Fatalf("create script %q: %v", f.Name, err)
		}
	}
	create(scripts.CustomScriptFields{
		Name: "Install deps", Kind: scripts.KindScript, Command: "echo install", UseAdeDir: true,
		Params: []scripts.Param{
			{Name: "mode", Type: scripts.ParamSelect, Options: []string{"ci", "dev"}, Required: true},
			{Name: "flags", Type: scripts.ParamMultiselect, Options: []string{"a", "b", "c"}},
			{Name: "token", Type: scripts.ParamText, Secret: true},
		},
	}, map[string]*string{"token": &tok})
	create(scripts.CustomScriptFields{Name: "Staging sha", Kind: scripts.KindScript, Command: "echo abc", Collection: "Deploy"}, nil)
	create(scripts.CustomScriptFields{Name: "Review", Kind: scripts.KindSmart, Command: "Review it", UseAdeDir: true,
		Smart: &scripts.Smart{Model: "opus", MaxBudgetUSD: 0.5, Timeout: "5m", Tools: []string{"Read"}}}, nil)
	list, err := app.W.CustomScripts.List()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, "repo-script-picker", "scripts#list", list, mask)

	seed := "# Settings of the web repo.\n" +
		"path: " + key + "\n" +
		"nickname: Web # shown in the board\n" +
		"worktree:\n" +
		"  prepare_script: pnpm install\n" +
		"environments:\n" +
		"  - name: staging\n" +
		"    deployed_sha_script: curl -fsS https://staging.example.com/version\n" +
		"  - name: prod\n" +
		"    deployed_sha_run:\n" +
		"      script: Prod sha\n"
	saved, err := svc.SaveFile(repocfg.SaveArgs{Path: key, Text: seed})
	if err != nil || saved.Problem != nil {
		t.Fatalf("SaveFile = %+v, %v, want a valid file", saved, err)
	}
	view, err := svc.File(repocfg.FileArgs{Path: key})
	if err != nil || view.Text != seed {
		t.Fatalf("File = %+v, %v", view, err)
	}
	app.Contract(t, "repo-script-picker", "file#read", view, mask)

	call := func(name, text string, edits ...configfs.Edit) (configfs.FormResult, repocfg.File) {
		t.Helper()
		if edits == nil {
			edits = []configfs.Edit{}
		}
		args := repocfg.FormArgs{File: view.File, Text: text, Edits: edits}
		app.Contract(t, "repo-script-picker", "args:RepoConfigService.Form#"+name, args, mask)
		res, err := svc.Form(args)
		if err != nil || res.Problem != nil {
			t.Fatalf("Form %s = %+v, %v", name, res, err)
		}
		app.Contract(t, "repo-script-picker", "form#"+name, res, mask)
		f, ok := res.Value.(repocfg.File)
		if !ok {
			t.Fatalf("Form %s value = %#v", name, res.Value)
		}
		return res, f
	}

	call("read", seed)

	install := map[string]any{"script": "Install deps"}
	pick, f := call("pick-prepare", seed,
		unsetEdit("worktree", "prepare_script"),
		setEdit(install, "worktree", "prepare_run"),
		unsetEdit("worktree", "prepare_script"))
	if !strings.Contains(pick.Text, "prepare_run:") || strings.Contains(pick.Text, "prepare_script") ||
		!strings.Contains(pick.Text, "# Settings of the web repo.") || !strings.Contains(pick.Text, "# shown in the board") ||
		f.Worktree == nil || !f.Worktree.PrepareRun.Equal(&model.ScriptRef{Script: "Install deps"}) {
		t.Fatalf("picking a script = %+v", pick)
	}

	// Back on the form tab the UI re-decodes the draft; the next edit diffs against that value.
	call("pick-prepare-view", pick.Text)

	withParams := map[string]any{"script": "Install deps", "params": map[string]any{"mode": "ci", "flags": []string{"a", "b"}}}
	params, f := call("params", pick.Text,
		setEdit(withParams, "worktree", "prepare_run"),
		unsetEdit("worktree", "prepare_script"))
	wantRef := &model.ScriptRef{Script: "Install deps", Params: map[string]model.ParamValues{"mode": {"ci"}, "flags": {"a", "b"}}}
	if !strings.Contains(params.Text, "mode: ci") || !strings.Contains(params.Text, "flags:") ||
		strings.Contains(params.Text, "- ci") || f.Worktree == nil || !f.Worktree.PrepareRun.Equal(wantRef) {
		t.Fatalf("params = %+v", params)
	}
	// The value crosses the bridge in the file's own shape, so the form sees no change on reload.
	blob, err := json.Marshal(params.Value)
	if err != nil || !strings.Contains(string(blob), `"mode":"ci"`) || !strings.Contains(string(blob), `"flags":["a","b"]`) {
		t.Fatalf("value JSON = %s, %v, want mode as a string and flags as a list", blob, err)
	}
	if _, again := call("params-view", params.Text); !again.Worktree.PrepareRun.Equal(wantRef) {
		t.Fatalf("re-decode drifted: %+v", again.Worktree.PrepareRun)
	}

	back, f := call("back-to-command", params.Text,
		setEdit("pnpm i", "worktree", "prepare_script"),
		unsetEdit("worktree", "prepare_run"),
		unsetEdit("worktree", "prepare_run"))
	if strings.Contains(back.Text, "prepare_run") || !strings.Contains(back.Text, "prepare_script: pnpm i") ||
		f.Worktree == nil || f.Worktree.PrepareRun != nil {
		t.Fatalf("back to a command = %+v", back)
	}

	env, f := call("pick-env", seed, setEdit([]any{
		map[string]any{"name": "staging", "deployed_sha_run": map[string]any{"script": "Staging sha"}},
		map[string]any{"name": "prod", "deployed_sha_run": map[string]any{"script": "Prod sha"}},
	}, "environments"))
	if len(f.Environments) != 2 || !f.Environments[0].DeployedSHARun.Equal(&model.ScriptRef{Script: "Staging sha"}) ||
		f.Environments[0].DeployedSHAScript != "" || !f.Environments[1].DeployedSHARun.Equal(&model.ScriptRef{Script: "Prod sha"}) ||
		strings.Contains(env.Text, "curl") {
		t.Fatalf("picking an environment script = %+v", env)
	}

	variant := func(lines ...string) string {
		return repoText(key, append([]string{"worktree:", "  prepare_run:"}, lines...)...)
	}
	for name, text := range map[string]string{
		"smart-read":   variant("    script: Review"),
		"missing-read": variant("    script: Nope"),
		"unknown-read": variant("    script: Install deps", "    params:", "      mode: ci", "      zzz: x"),
	} {
		if _, f := call(name, text); f.Worktree == nil || f.Worktree.PrepareRun == nil {
			t.Fatalf("%s did not decode a reference", name)
		}
	}
	_, f = call("drop-unknown", variant("    script: Install deps", "    params:", "      mode: ci", "      zzz: x"),
		setEdit(map[string]any{"script": "Install deps", "params": map[string]any{"mode": "ci"}}, "worktree", "prepare_run"),
		unsetEdit("worktree", "prepare_script"))
	if !f.Worktree.PrepareRun.Equal(&model.ScriptRef{Script: "Install deps", Params: map[string]model.ParamValues{"mode": {"ci"}}}) {
		t.Fatalf("drop-unknown = %+v", f.Worktree.PrepareRun)
	}

	saveArgs := repocfg.SaveArgs{File: view.File, Text: env.Text, BaseHash: view.Hash}
	app.Contract(t, "repo-script-picker", "args:RepoConfigService.SaveFile#saved", saveArgs, mask)
	out, err := svc.SaveFile(saveArgs)
	if err != nil || out.Problem != nil {
		t.Fatalf("SaveFile = %+v, %v", out, err)
	}
	back2, err := svc.File(repocfg.FileArgs{File: view.File})
	if err != nil || back2.Text != env.Text {
		t.Fatalf("File after the save = %+v, %v", back2, err)
	}
	app.Contract(t, "repo-script-picker", "file#saved", back2, mask)
	waitFor(t, 10*time.Second, func() bool { return app.W.RepoConfig.Repo(key).Environments[0].DeployedShaRun != nil })

	again, err := svc.SaveFile(repocfg.SaveArgs{File: view.File, Text: params.Text, BaseHash: back2.Hash})
	if err != nil || again.Problem != nil {
		t.Fatalf("SaveFile = %+v, %v", again, err)
	}
	waitFor(t, 10*time.Second, func() bool { return app.W.RepoConfig.Repo(key).Settings.WorktreePrepareRun != nil })
	if got := app.W.RepoConfig.Repo(key).Settings.WorktreePrepareRun; !got.Equal(wantRef) {
		t.Fatalf("stored prepare_run = %+v, want %+v", got, wantRef)
	}
}
