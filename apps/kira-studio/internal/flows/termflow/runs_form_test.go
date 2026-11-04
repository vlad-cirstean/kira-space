package termflow

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

const formRunYAML = `# Ship the build.
name: Ship # shown in the panel
schedule:
  enabled: false
  confirm: false
  cron: "0 9 * * *"
command: echo ship
color: blue
collection: Ops
`

const formSecret = `f0rm"s3cret-71`

// TestRunsFormStudio: the form edits a run file as node edits (comments and key order stay), a
// YAML-side change re-decodes, a bad draft is a problem with a line, and a secret param's value
// never enters the file or the contract.
func TestRunsFormStudio(t *testing.T) {
	app := flowharness.New(t)
	cs := app.W.CustomScripts
	writeRun(t, app, "ship.yaml", formRunYAML)
	waitSnapshot(t, app, 0, func(s scripts.Snapshot) bool { _, ok := scriptOf(s, "ship"); return ok })
	view, err := cs.File(bridge.CustomScriptsFileArgs{File: "ship.yaml"})
	if err != nil {
		t.Fatal(err)
	}

	read, err := cs.Form(bridge.CustomScriptsFormArgs{File: "ship.yaml", Text: view.Text})
	if err != nil || read.Problem != nil || read.Text != formRunYAML {
		t.Fatalf("Form without edits = %+v, %v, want the text unchanged and no problem", read, err)
	}
	app.Contract(t, "runs-form", "form#read", read, runsMasks...)

	args := bridge.CustomScriptsFormArgs{File: "ship.yaml", Text: view.Text, Edits: []configfs.Edit{
		{Path: []string{"name"}, Value: "Release"},
		{Path: []string{"schedule", "cron"}, Value: "30 8 * * 1"},
		{Path: []string{"params"}, Value: []any{map[string]any{"name": "token", "type": "text", "secret": true}}},
		{Path: []string{"color"}, Unset: true},
	}}
	app.Contract(t, "runs-form", "args:CustomScriptsService.Form", args, runsMasks...)
	res, err := cs.Form(args)
	if err != nil || res.Problem != nil {
		t.Fatalf("Form = %+v, %v", res, err)
	}
	for _, want := range []string{"# Ship the build.", "# shown in the panel", "name: Release", `cron: 30 8 * * 1`, "secret: true", "collection: Ops"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("edited text lacks %q:\n%s", want, res.Text)
		}
	}
	if strings.Contains(res.Text, "color:") {
		t.Errorf("unset left color:\n%s", res.Text)
	}
	if strings.Index(res.Text, "schedule:") > strings.Index(res.Text, "command:") || strings.Index(res.Text, "command:") > strings.Index(res.Text, "collection:") {
		t.Errorf("key order changed:\n%s", res.Text)
	}
	app.Contract(t, "runs-form", "form#edited", res, runsMasks...)

	saved, err := cs.SaveFile(bridge.CustomScriptsSaveFileArgs{File: "ship.yaml", Text: res.Text, BaseHash: view.Hash})
	if err != nil || saved.Problem != nil || saved.Text != res.Text {
		t.Fatalf("SaveFile = %+v, %v", saved, err)
	}
	snap := waitSnapshot(t, app, 0, func(s scripts.Snapshot) bool { r, _ := scriptOf(s, "ship"); return r.Name == "Release" })
	rec, _ := scriptOf(snap, "ship")
	if len(rec.Params) != 1 || rec.Params[0].Name != "token" || rec.Schedule == nil || rec.Schedule.Cron != "30 8 * * 1" {
		t.Fatalf("record after the save = %+v", rec)
	}

	withSecret, err := cs.SetSecrets(bridge.CustomScriptsSetSecretsArgs{ID: "ship", Values: map[string]*string{"token": ptr(formSecret)}})
	if err != nil || len(withSecret.SavedSecrets) != 1 {
		t.Fatalf("SetSecrets = %+v, %v", withSecret, err)
	}
	back, err := cs.File(bridge.CustomScriptsFileArgs{File: "ship.yaml"})
	if err != nil || back.Text != res.Text || strings.Contains(back.Text, "s3cret") || strings.Contains(back.Text, formSecret) {
		t.Fatalf("File after the save = %+v, %v", back, err)
	}
	app.Contract(t, "runs-form", "file#saved", back, runsMasks...)

	yamlSide := strings.Replace(back.Text, "name: Release", "name: Rollout", 1)
	changed, err := cs.Form(bridge.CustomScriptsFormArgs{File: "ship.yaml", Text: yamlSide})
	if err != nil || changed.Problem != nil || changed.Text != yamlSide {
		t.Fatalf("Form of a YAML-side change = %+v, %v", changed, err)
	}
	if v, ok := changed.Value.(scripts.File); !ok || v.Name != "Rollout" {
		t.Fatalf("decoded value = %#v, want name Rollout", changed.Value)
	}

	bad, err := cs.Form(bridge.CustomScriptsFormArgs{File: "ship.yaml", Text: back.Text + "  broken: [\n"})
	if err != nil || bad.Problem == nil || bad.Problem.Line == 0 || bad.Value != nil {
		t.Fatalf("Form of bad text = %+v, %v, want a problem with a line and no value", bad, err)
	}
	app.Contract(t, "runs-form", "form#bad", bad, runsMasks...)

}

func ptr[T any](v T) *T { return &v }
