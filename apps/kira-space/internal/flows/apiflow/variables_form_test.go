package apiflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Contract api-variables-form, read by tests/ui/api-variables-form.spec.ts: the variables dialog
// edits an environment file as node edits (comments and key order stay), a YAML-side change
// re-decodes, a bad draft is a problem with a line, a collection file edits `variables` only, and a
// typed secret saves a reference into the file and the value through the write-only call.
func TestApiVariablesForm(t *testing.T) {
	app := flowharness.New(t)
	const sc = "api-variables-form"
	const typed = "typed-token-5d2e"
	const envFile = "staging.environment.yaml"
	cfg := app.W.ApiConfig
	mask := flowharness.Mask("hash", "baseHash")
	paths := flowharness.Replace(app.SpaceHome, "<home>")

	// A head comment, a line comment and color before name.
	seed := "# Staging servers for the web team.\n" +
		"color: green\n" +
		"name: Staging # shown in the picker\n" +
		"variables:\n" +
		"  - name: base_url\n" +
		"    value: https://staging.example.com\n" +
		"  - name: token\n" +
		"    secret: ${secret:staging.token}\n"
	app.WriteAPIFile(t, envFile, []byte(seed))
	view, err := cfg.File(bridge.ApiFileArgs{File: envFile})
	if err != nil || view.Text != seed || view.Problem != nil || view.Kind != "environment" {
		t.Fatalf("File = %+v, %v, want the seed", view, err)
	}
	app.Contract(t, sc, "file#read", view, mask, paths)

	var recorded []any
	call := func(name, file string, edits []configfs.Edit, text string) configfs.FormResult {
		t.Helper()
		args := bridge.ApiFormArgs{File: file, Text: text, Edits: edits}
		app.Contract(t, sc, "args:ApiConfigService.Form#"+name, args, mask, paths)
		res, err := cfg.Form(args)
		if err != nil {
			t.Fatalf("Form %s: %v", name, err)
		}
		app.Contract(t, sc, "form#"+name, res, mask, paths)
		recorded = append(recorded, args, res)
		return res
	}

	read := call("read", envFile, []configfs.Edit{}, view.Text)
	f, ok := read.Value.(apifiles.EnvironmentFile)
	if read.Problem != nil || read.Text != seed || !ok || f.Name != "Staging" || len(f.Variables) != 2 {
		t.Fatalf("Form without edits = %+v, want the text unchanged and the file decoded", read)
	}

	renamed := call("rename", envFile, []configfs.Edit{{Path: []string{"name"}, Value: "Staging EU"}}, view.Text)
	if renamed.Problem != nil || !strings.Contains(renamed.Text, "name: Staging EU # shown in the picker") {
		t.Fatalf("Form of one scalar = %+v", renamed)
	}

	// A scalar, a list replace and an unset in one call; the typed secret is a reference.
	edited := call("edited", envFile, []configfs.Edit{
		{Path: []string{"description"}, Value: "EU staging"},
		{Path: []string{"variables"}, Value: []any{
			map[string]any{"name": "base_url", "value": "https://eu.example.com", "description": "API root"},
			map[string]any{"name": "token", "secret": "${secret:staging-eu.token}"},
		}},
		{Path: []string{"color"}, Unset: true},
	}, view.Text)
	if edited.Problem != nil {
		t.Fatalf("Form = %+v", edited)
	}
	for _, want := range []string{"# Staging servers for the web team.", "# shown in the picker", "description: EU staging", "value: https://eu.example.com", "secret: ${secret:staging-eu.token}"} {
		if !strings.Contains(edited.Text, want) {
			t.Errorf("edited text lacks %q:\n%s", want, edited.Text)
		}
	}
	if strings.Contains(edited.Text, "color:") || strings.Contains(edited.Text, typed) {
		t.Errorf("edited text keeps the unset color or holds the secret:\n%s", edited.Text)
	}
	if a, b := strings.Index(edited.Text, "name:"), strings.Index(edited.Text, "variables:"); a > b {
		t.Errorf("key order changed:\n%s", edited.Text)
	}
	got, ok := edited.Value.(apifiles.EnvironmentFile)
	if !ok || got.Description != "EU staging" || len(got.Variables) != 2 || got.Variables[1].Secret != "${secret:staging-eu.token}" {
		t.Fatalf("decoded value = %#v", edited.Value)
	}
	if _, err := cfg.SetSecret(bridge.ApiSecretArgs{Name: "staging-eu.token", Value: typed}); err != nil {
		t.Fatal(err)
	}

	yamlSide := strings.Replace(view.Text, "name: Staging", "name: Staging yaml", 1)
	changed := call("yaml", envFile, []configfs.Edit{}, yamlSide)
	if got, ok := changed.Value.(apifiles.EnvironmentFile); !ok || got.Name != "Staging yaml" || changed.Text != yamlSide {
		t.Fatalf("Form of a YAML-side change = %+v", changed)
	}

	bad := call("bad", envFile, []configfs.Edit{}, strings.Replace(view.Text, "    secret: ${secret:staging.token}", "    secret: ${secret:staging.token}\n    value: x", 1))
	if bad.Problem == nil || bad.Problem.Line == 0 || bad.Value != nil {
		t.Fatalf("Form of bad text = %+v, want a problem with a line and no value", bad)
	}

	// A collection file edits `variables` only; the rest, items included, stays.
	col := newCollection(t, app, "Orders API")
	colView, err := cfg.File(bridge.ApiFileArgs{File: col.File})
	if err != nil || colView.Kind != "collection" {
		t.Fatalf("File of a collection = %+v, %v", colView, err)
	}
	app.Contract(t, sc, "file#collection", colView, mask, paths)
	vars := call("collection", col.File, []configfs.Edit{{Path: []string{"variables"}, Value: []any{
		map[string]any{"name": "base_url", "value": "https://orders.example.com"},
	}}}, colView.Text)
	if cf, ok := vars.Value.(apifiles.CollectionFile); vars.Problem != nil || !ok || cf.Name != "Orders API" || len(cf.Variables) != 1 {
		t.Fatalf("Form of a collection = %+v", vars)
	}

	// Save: a stale hash conflicts; the right one writes, rows follow and the change event fires.
	saveArgs := bridge.ApiSaveFileArgs{File: envFile, Text: edited.Text, BaseHash: view.Hash}
	app.Contract(t, sc, "args:ApiConfigService.SaveFile", saveArgs, mask, paths)
	if _, err := cfg.SaveFile(bridge.ApiSaveFileArgs{File: envFile, Text: edited.Text, BaseHash: "stale"}); testx.AsIpcErr(t, err).Code != "E_CONFLICT" {
		t.Errorf("SaveFile with a stale hash = %v, want E_CONFLICT", err)
	}
	mark := app.Events.Mark()
	out, err := cfg.SaveFile(saveArgs)
	if err != nil || out.Problem != nil || out.Text != edited.Text {
		t.Fatalf("SaveFile = %+v, %v", out, err)
	}
	app.Contract(t, sc, "file#saved", out, mask, paths)
	if len(apiChanges(t, app, mark)) == 0 {
		t.Error("SaveFile emitted no kira:api:dataChanged")
	}
	back, err := cfg.File(bridge.ApiFileArgs{File: envFile})
	if err != nil || back.Text != edited.Text {
		t.Fatalf("File after SaveFile = %+v, %v", back, err)
	}
	envs, err := app.W.Variables.ListEnvironments()
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].Name != "Staging" {
		t.Errorf("environments after SaveFile = %+v, want Staging", envs)
	}

	// Invalid text is saved too, flagged with its line; the last good rows keep serving.
	broken, err := cfg.SaveFile(bridge.ApiSaveFileArgs{File: envFile, Text: bad.Text, BaseHash: out.Hash})
	if err != nil || broken.Problem == nil || broken.Problem.Line == 0 || !broken.LastGood {
		t.Fatalf("SaveFile of bad text = %+v, %v, want a flagged file with last good rows", broken, err)
	}
	if _, err := cfg.File(bridge.ApiFileArgs{File: "nope.environment.yaml"}); testx.AsIpcErr(t, err).Code != "E_NOT_FOUND" {
		t.Errorf("File of a missing file = %v, want E_NOT_FOUND", err)
	}

	all, err := json.Marshal(append(recorded, view, out, back))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(all), typed) {
		t.Error("a form call or a file view holds the typed secret")
	}
}
