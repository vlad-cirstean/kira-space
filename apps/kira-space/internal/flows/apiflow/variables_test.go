package apiflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

func TestEnvironmentFiles(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
	vs := app.W.Variables
	col := app.SeedCollection(t, "scoped", flowharness.APIVar{Name: "who", Value: "collection", Secret: true})
	dev := app.SeedEnvironment(t, "dev", flowharness.APIVar{Name: "who", Value: "from-dev", Secret: true})
	prod := app.SeedEnvironment(t, "prod", flowharness.APIVar{Name: "who", Value: "from-prod", Secret: true})
	sendWho := func(envID string) string {
		t.Helper()
		args := get(srv.URL + "/echo?who={{who}}")
		args.CollectionID, args.EnvironmentID = col.ID, envID
		send(t, app, args)
		return lastRequest(srv).Query
	}
	names := func() []string {
		t.Helper()
		envs, err := vs.ListEnvironments()
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(envs))
		for i, e := range envs {
			out[i] = e.Name
		}
		return out
	}

	// CreateEnvironment writes a file named after the environment.
	created, err := vs.CreateEnvironment(bridge.VariablesCreateEnvironmentArgs{Name: "Stage Env", Description: "shared", Color: "green"})
	if err != nil {
		t.Fatal(err)
	}
	if created.File != "stage-env.environment.yaml" || created.Color != "green" || created.Description != "shared" {
		t.Fatalf("CreateEnvironment = %+v, want stage-env.environment.yaml, green, shared", created)
	}
	if _, err := vs.CreateEnvironment(bridge.VariablesCreateEnvironmentArgs{Name: "", Color: "none"}); ipcErr(t, err).Code != "E_BAD_REQUEST" {
		t.Fatalf("CreateEnvironment with no name = %v, want E_BAD_REQUEST", err)
	}
	if len(names()) != 3 {
		t.Fatalf("environments = %v, want dev, prod, Stage Env", names())
	}

	// Removing the active environment's file: the list loses it, nothing stays active, and a send
	// that names it resolves the collection's value.
	if err := vs.SetActiveEnvironment(bridge.VariablesEnvironmentIDArgs{ID: prod.ID}); err != nil {
		t.Fatal(err)
	}
	app.RemoveAPIFile(t, prod.File)
	envs, _ := vs.ListEnvironments()
	for _, e := range envs {
		if e.IsActive || e.ID == prod.ID {
			t.Fatalf("environment %q survives the removal of the active environment's file", e.Name)
		}
	}
	if got := sendWho(dev.ID); got != "who=from-dev" {
		t.Fatalf("send with a live environment = %q", got)
	}
	if got := sendWho(prod.ID); got != "who=collection" {
		t.Fatalf("send naming the removed environment = %q, want the collection value", got)
	}
	if left, err := vs.List(bridge.VariablesScopeArgs{Scope: model.VariableScopeEnvironment, OwnerID: prod.ID}); err != nil || len(left) != 0 {
		t.Fatalf("removed environment kept variables: %+v (%v)", left, err)
	}

	app.Restart()
	vs = app.W.Variables
	if got := names(); len(got) != 2 {
		t.Fatalf("environments after relaunch = %v, want 2", got)
	}
}

// Same scenario as tests/ui/http-variables.spec.ts "the active environment's base variable reaches
// the wire": both halves read tests/contract/api-boot.json. The renderer substitutes plain
// variables before sending; the backend only resolves secrets.
func TestActiveEnvironmentResolvesBase(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
	const sc = "api-boot"
	env := app.SeedEnvironment(t, "Flow", flowharness.APIVar{Name: "base", Value: srv.URL})
	if err := app.W.Variables.SetActiveEnvironment(bridge.VariablesEnvironmentIDArgs{ID: env.ID}); err != nil {
		t.Fatal(err)
	}
	envs, err := app.W.Variables.ListEnvironments()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "VariablesService.ListEnvironments", envs)
	vars, err := app.W.Variables.List(bridge.VariablesScopeArgs{Scope: model.VariableScopeEnvironment, OwnerID: env.ID})
	if err != nil {
		t.Fatal(err)
	}
	opts := flowharness.Replace(srv.URL, "<http>")
	app.Contract(t, sc, "VariablesService.List", vars, opts)

	args := get(srv.URL + "/echo?from=ui")
	args.EnvironmentID = env.ID
	app.Contract(t, sc, "args:HttpService.Send", args, flowharness.Mask("opId"), opts)
	res := send(t, app, args)
	app.Contract(t, sc, "HttpService.Send", res, opts, flowharness.Mask("elapsedMs", "bodyBytes"), flowharness.Omit("body", "headers", "timeline", "wire"))
	if got := lastRequest(srv); got.Path != "/echo" || got.Query != "from=ui" {
		t.Errorf("server saw %s?%s, want /echo?from=ui", got.Path, got.Query)
	}
}

// J-ENV-010. A plain variable's earlier values are kept as history; reverting is writing the old
// value back, and the file then holds exactly the bytes it held before the first edit.
func TestVariableHistoryAndRevert(t *testing.T) {
	app := flowharness.New(t)
	env := app.SeedEnvironment(t, "Hist", flowharness.APIVar{Name: "base", Value: "https://one.test"})
	path := filepath.Join(app.W.ApiFiles.Dir(), env.File)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	setBase := func(value string) {
		t.Helper()
		cur, err := app.W.ApiConfig.File(bridge.ApiFileArgs{File: env.File})
		if err != nil {
			t.Fatal(err)
		}
		res, err := app.W.ApiConfig.Form(bridge.ApiFormArgs{File: env.File, Text: cur.Text, Edits: []configfs.Edit{
			{Path: []string{"variables"}, Value: []any{map[string]any{"name": "base", "value": value}}},
		}})
		if err != nil || res.Problem != nil {
			t.Fatalf("Form = %+v (%v)", res, err)
		}
		if _, err := app.W.ApiConfig.SaveFile(bridge.ApiSaveFileArgs{File: env.File, Text: res.Text, BaseHash: cur.Hash}); err != nil {
			t.Fatal(err)
		}
	}
	list := func() model.Variable {
		t.Helper()
		vars, err := app.W.Variables.List(bridge.VariablesScopeArgs{Scope: model.VariableScopeEnvironment, OwnerID: env.ID})
		if err != nil || len(vars) != 1 {
			t.Fatalf("variables = %+v (%v)", vars, err)
		}
		return vars[0]
	}
	history := func() []string {
		t.Helper()
		rows, err := app.W.Variables.History(bridge.VariablesHistoryArgs{VariableID: list().ID})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range rows {
			out = append(out, r.Value)
		}
		return out
	}

	setBase("https://two.test")
	setBase("https://three.test")
	if got := list().Value; got != "https://three.test" {
		t.Fatalf("value = %q, want three", got)
	}
	if got := history(); len(got) != 2 || got[0] != "https://two.test" || got[1] != "https://one.test" {
		t.Fatalf("history = %v, want two then one, newest first", got)
	}

	setBase("https://one.test")
	if got := list().Value; got != "https://one.test" {
		t.Fatalf("value after the revert = %q, want one", got)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("file after the revert differs from before the edits\n got: %q\nwant: %q", after, before)
	}
	if got := history(); len(got) != 3 || got[0] != "https://three.test" {
		t.Errorf("history after the revert = %v, want three on top of two and one", got)
	}
}
