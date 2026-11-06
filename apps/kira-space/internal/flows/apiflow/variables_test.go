package apiflow

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func TestEnvironmentFiles(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)
	vs := app.W.Variables
	col := app.SeedCollection(t, "scoped", studioflowharness.APIVar{Name: "who", Value: "collection", Secret: true})
	dev := app.SeedEnvironment(t, "dev", studioflowharness.APIVar{Name: "who", Value: "from-dev", Secret: true})
	prod := app.SeedEnvironment(t, "prod", studioflowharness.APIVar{Name: "who", Value: "from-prod", Secret: true})
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

	app.Restart(t)
	vs = app.W.Variables
	if got := names(); len(got) != 2 {
		t.Fatalf("environments after relaunch = %v, want 2", got)
	}
}

// Same scenario as tests/ui/http-variables.spec.ts "the active environment's base variable reaches
// the wire": both halves read tests/contract/api-boot.json. The renderer substitutes plain
// variables before sending; the backend only resolves secrets.
func TestActiveEnvironmentResolvesBase(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)
	const sc = "api-boot"
	env := app.SeedEnvironment(t, "Flow", studioflowharness.APIVar{Name: "base", Value: srv.URL})
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
	opts := studioflowharness.Replace(srv.URL, "<http>")
	app.Contract(t, sc, "VariablesService.List", vars, opts)

	args := get(srv.URL + "/echo?from=ui")
	args.EnvironmentID = env.ID
	app.Contract(t, sc, "args:HttpService.Send", args, studioflowharness.Mask("opId"), opts)
	res := send(t, app, args)
	app.Contract(t, sc, "HttpService.Send", res, opts, studioflowharness.Mask("elapsedMs", "bodyBytes"), studioflowharness.Omit("body", "headers", "timeline", "wire"))
	if got := lastRequest(srv); got.Path != "/echo" || got.Query != "from=ui" {
		t.Errorf("server saw %s?%s, want /echo?from=ui", got.Path, got.Query)
	}
}
