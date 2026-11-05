package repoflow_test

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// TestRepoConfigForm: the form edits a repo file as node edits (comments and key order stay), a
// YAML-side change re-decodes, a bad draft is a problem with a line, and writing one script form
// drops its exclusive partner (the P513 keys) while the other rows keep theirs.
func TestRepoConfigForm(t *testing.T) {
	app := flowharness.New(t)
	r := app.NewRepo("proj")
	r.Commit("init", map[string]string{"a.txt": "a\n"})
	rec := importVia(t, app, r.Dir)
	key := rec.RepoID
	svc := app.W.RepoConfigSvc
	mask := flowharness.Mask("hash", "baseHash", "filePath")

	seed := "# Settings of the web repo.\n" +
		"path: " + key + "\n" +
		"nickname: Web # shown in the board\n" +
		"worktree:\n" +
		"  prepare_timeout: 15m\n" +
		"  prepare_run:\n" +
		"    script: Install deps\n" +
		"environments:\n" +
		"  - name: staging\n" +
		"    deployed_sha_run:\n" +
		"      script: Staging sha\n" +
		"  - name: prod\n" +
		"    deployed_sha_run:\n" +
		"      script: Prod sha\n" +
		"integration_branches: [develop]\n"
	saved, err := svc.SaveFile(repocfg.SaveArgs{Path: key, Text: seed})
	if err != nil || saved.Problem != nil {
		t.Fatalf("SaveFile = %+v, %v, want a valid file", saved, err)
	}
	view, err := svc.File(repocfg.FileArgs{Path: key})
	if err != nil || view.Text != seed {
		t.Fatalf("File = %+v, %v", view, err)
	}
	app.Contract(t, "repo-config-form", "file#read", view, mask)

	call := func(name string, edits []configfs.Edit, text string) configfs.FormResult {
		t.Helper()
		args := repocfg.FormArgs{File: view.File, Text: text, Edits: edits}
		app.Contract(t, "repo-config-form", "args:RepoConfigService.Form#"+name, args, mask)
		res, err := svc.Form(args)
		if err != nil {
			t.Fatalf("Form %s: %v", name, err)
		}
		app.Contract(t, "repo-config-form", "form#"+name, res, mask)
		return res
	}

	read := call("read", []configfs.Edit{}, view.Text)
	if read.Problem != nil || read.Text != seed {
		t.Fatalf("Form without edits = %+v, want the text unchanged and no problem", read)
	}
	if f, ok := read.Value.(repocfg.File); !ok || f.Nickname != "Web" || f.Worktree == nil || f.Worktree.PrepareRun == nil || len(f.Environments) != 2 {
		t.Fatalf("decoded value = %#v", read.Value)
	}

	renamed := call("rename", []configfs.Edit{{Path: []string{"nickname"}, Value: "Web app"}}, view.Text)
	if renamed.Problem != nil || !strings.Contains(renamed.Text, "nickname: Web app # shown in the board") {
		t.Fatalf("Form of one scalar = %+v", renamed)
	}

	// Typing a prepare script unsets prepare_run in the same call: once as the field's partner, once
	// as the form's own change of the ref leaf.
	seam := call("seam", []configfs.Edit{
		{Path: []string{"worktree", "prepare_script"}, Value: "bun install"},
		{Path: []string{"worktree", "prepare_run"}, Unset: true},
		{Path: []string{"worktree", "prepare_run"}, Unset: true},
	}, view.Text)
	if seam.Problem != nil || strings.Contains(seam.Text, "prepare_run") || !strings.Contains(seam.Text, "prepare_script: bun install") ||
		!strings.Contains(seam.Text, "prepare_timeout: 15m") {
		t.Fatalf("prepare_script over prepare_run = %+v", seam)
	}

	// Typing a row's script drops that row's deployed_sha_run; the other row keeps its own.
	env := call("env", []configfs.Edit{{Path: []string{"environments"}, Value: []any{
		map[string]any{"name": "staging", "deployed_sha_script": "curl -fsS https://staging.example.com/version"},
		map[string]any{"name": "prod", "deployed_sha_run": map[string]any{"script": "Prod sha"}},
	}}}, view.Text)
	if env.Problem != nil || strings.Count(env.Text, "deployed_sha_run") != 1 || !strings.Contains(env.Text, "Prod sha") ||
		strings.Contains(env.Text, "Staging sha") || !strings.Contains(env.Text, "deployed_sha_script:") {
		t.Fatalf("environments row edit = %+v", env)
	}

	all := call("edited", []configfs.Edit{
		{Path: []string{"nickname"}, Value: "Web app"},
		{Path: []string{"worktree", "prepare_script"}, Value: "bun install"},
		{Path: []string{"worktree", "prepare_run"}, Unset: true},
		{Path: []string{"environments"}, Value: []any{
			map[string]any{"name": "staging", "deployed_sha_script": "curl -fsS https://staging.example.com/version"},
			map[string]any{"name": "prod", "deployed_sha_run": map[string]any{"script": "Prod sha"}},
		}},
		{Path: []string{"integration_branches"}, Unset: true},
	}, view.Text)
	if all.Problem != nil {
		t.Fatalf("Form = %+v", all)
	}
	for _, want := range []string{"# Settings of the web repo.", "# shown in the board", "nickname: Web app", "prepare_script: bun install", "prepare_timeout: 15m"} {
		if !strings.Contains(all.Text, want) {
			t.Errorf("edited text lacks %q:\n%s", want, all.Text)
		}
	}
	if strings.Contains(all.Text, "prepare_run") || strings.Contains(all.Text, "integration_branches") || strings.Contains(all.Text, "Staging sha") {
		t.Errorf("edited text keeps a removed key:\n%s", all.Text)
	}
	if a, b, c := strings.Index(all.Text, "path:"), strings.Index(all.Text, "nickname:"), strings.Index(all.Text, "worktree:"); !(a < b && b < c && c < strings.Index(all.Text, "environments:")) {
		t.Errorf("key order changed:\n%s", all.Text)
	}

	yamlSide := strings.Replace(view.Text, "nickname: Web", "nickname: Shop", 1)
	changed := call("yaml", []configfs.Edit{}, yamlSide)
	if f, ok := changed.Value.(repocfg.File); !ok || f.Nickname != "Shop" || changed.Text != yamlSide {
		t.Fatalf("Form of a YAML-side change = %+v", changed)
	}

	bad := call("bad", []configfs.Edit{}, view.Text+"  broken: [\n")
	if bad.Problem == nil || bad.Problem.Line == 0 || bad.Value != nil {
		t.Fatalf("Form of bad text = %+v, want a problem with a line and no value", bad)
	}

	saveArgs := repocfg.SaveArgs{File: view.File, Text: all.Text, BaseHash: view.Hash}
	app.Contract(t, "repo-config-form", "args:RepoConfigService.SaveFile", saveArgs, mask)
	if _, err := svc.SaveFile(repocfg.SaveArgs{File: view.File, Text: all.Text, BaseHash: "stale"}); err == nil {
		t.Fatalf("SaveFile with a stale hash succeeded")
	}
	out, err := svc.SaveFile(saveArgs)
	if err != nil || out.Problem != nil || out.Text != all.Text {
		t.Fatalf("SaveFile = %+v, %v", out, err)
	}
	back, err := svc.File(repocfg.FileArgs{File: view.File})
	if err != nil || back.Text != all.Text {
		t.Fatalf("File after the save = %+v, %v", back, err)
	}
	app.Contract(t, "repo-config-form", "file#saved", back, mask)
}
