package apiflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Contract api-variables-matrix, read by tests/ui/studio/api-variables-matrix.spec.ts: the Variables
// tab edits one cell, one row or one column per call (EditVariables), writing the YAML in place
// (comments stay), sealing secret values in the same call, and a send resolves what was written.
func TestApiVariablesMatrix(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
	const sc = "api-variables-matrix"
	cfg, vs := app.W.ApiConfig, app.W.Variables
	mask := flowharness.Mask("hash", "baseHash")
	paths := flowharness.Replace(app.SpaceHome, "<home>")

	col := app.SeedCollection(t, "Orders API",
		flowharness.APIVar{Name: "base_url", Value: "https://orders.example.com"},
		flowharness.APIVar{Name: "page_size", Value: "50"})
	app.WriteAPIFile(t, "staging.environment.yaml", []byte("# Staging servers.\n"+
		"name: Staging # shown in the picker\n"+
		"color: green\n"+
		"variables:\n"+
		"  - name: base_url\n"+
		"    value: https://staging.example.com\n"+
		"  - name: token\n"+
		"    value: plain-token-1\n"))
	app.WriteAPIFile(t, "local.environment.yaml", []byte("name: Local\ncolor: blue\nvariables:\n  - name: base_url\n    value: http://localhost:8080\n"))
	envByName := func(name string) model.Environment {
		t.Helper()
		envs, err := vs.ListEnvironments()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range envs {
			if e.Name == name {
				return e
			}
		}
		t.Fatalf("environment %q missing", name)
		return model.Environment{}
	}
	staging, local := envByName("Staging"), envByName("Local")
	fileText := func(file string) string {
		t.Helper()
		d, err := cfg.File(bridge.ApiFileArgs{File: file})
		if err != nil {
			t.Fatal(err)
		}
		return d.Text
	}
	rows := func(scope model.VariableScope, id string) map[string]model.Variable {
		t.Helper()
		list, err := vs.List(bridge.VariablesScopeArgs{Scope: scope, OwnerID: id})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]model.Variable{}
		for _, v := range list {
			out[v.Name] = v
		}
		return out
	}
	sent := func(envID, name string) string {
		t.Helper()
		args := get(srv.URL + "/echo?v={{" + name + "}}")
		args.CollectionID, args.EnvironmentID = col.ID, envID
		send(t, app, args)
		return lastRequest(srv).Query
	}
	edit := func(name string, edits ...apifiles.VariableEdit) {
		t.Helper()
		args := bridge.ApiEditVariablesArgs{Edits: edits}
		app.Contract(t, sc, "args:ApiConfigService.EditVariables#"+name, args, mask, paths)
		mark := app.Events.Mark()
		view, err := cfg.EditVariables(args)
		if err != nil {
			t.Fatalf("EditVariables %s: %v", name, err)
		}
		app.Contract(t, sc, "EditVariables#"+name, view, mask, paths)
		if len(apiChanges(t, app, mark)) == 0 {
			t.Errorf("EditVariables %s emitted no kira:api:dataChanged", name)
		}
	}
	env := func(e model.Environment, op, name string) apifiles.VariableEdit {
		return apifiles.VariableEdit{Scope: "environment", OwnerID: e.ID, Op: op, Name: name}
	}

	// Set one cell: the file keeps its comments and the row follows.
	set := env(staging, apifiles.OpSet, "page_size")
	set.Value = ptr("100")
	edit("set", set)
	text := fileText(staging.File)
	for _, want := range []string{"# Staging servers.", "# shown in the picker", "name: page_size", "value: \"100\""} {
		if !strings.Contains(text, want) {
			t.Errorf("staging file lacks %q after set:\n%s", want, text)
		}
	}
	if got := rows(model.VariableScopeEnvironment, staging.ID)["page_size"].Value; got != "100" {
		t.Errorf("page_size in staging = %q, want 100", got)
	}

	// Unset the cell: the send falls back to the collection default.
	edit("unset", env(staging, apifiles.OpUnset, "page_size"))
	if _, ok := rows(model.VariableScopeEnvironment, staging.ID)["page_size"]; ok {
		t.Error("page_size is still a row of staging after unset")
	}
	// Plain values substitute in the renderer, which falls back to the collection default.
	if got := rows(model.VariableScopeCollection, col.ID)["page_size"].Value; got != "50" {
		t.Errorf("collection default page_size = %q, want 50", got)
	}

	// Rename one row across the collection and two environments, in one call.
	var renames []apifiles.VariableEdit
	for _, o := range []apifiles.VariableEdit{
		{Scope: "collection", OwnerID: col.ID}, {Scope: "environment", OwnerID: staging.ID}, {Scope: "environment", OwnerID: local.ID},
	} {
		o.Op, o.Name, o.NewName = apifiles.OpRename, "base_url", "host"
		renames = append(renames, o)
	}
	edit("rename", renames...)
	for owner, want := range map[string]string{staging.ID: "https://staging.example.com", local.ID: "http://localhost:8080"} {
		if got := rows(model.VariableScopeEnvironment, owner)["host"].Value; got != want {
			t.Errorf("renamed row host = %q, want %q", got, want)
		}
	}
	if _, old := rows(model.VariableScopeEnvironment, staging.ID)["base_url"]; old {
		t.Error("the old name is still a row")
	}
	if !strings.Contains(fileText(col.File), "name: host") {
		t.Error("the collection file keeps the old name")
	}
	if !strings.Contains(fileText(staging.File), "# shown in the picker") {
		t.Error("rename lost a comment")
	}

	// Lock a plain row: the value is sealed, the file holds the reference, the send still resolves it.
	edit("lock", env(staging, apifiles.OpLock, "token"))
	text = fileText(staging.File)
	if !strings.Contains(text, "secret: ${secret:staging.token}") || strings.Contains(text, "plain-token-1") {
		t.Errorf("staging file after lock:\n%s", text)
	}
	tok := rows(model.VariableScopeEnvironment, staging.ID)["token"]
	if !tok.IsSecret || !tok.SecretSet || tok.Value != "" {
		t.Errorf("token row after lock = %+v, want a set secret with no plain value", tok)
	}
	if got := sent(staging.ID, "token"); got != "v=plain-token-1" {
		t.Errorf("send of the locked row = %q, want the sealed value", got)
	}

	// Set a secret value in one cell: sealed, never in the file.
	setSecret := env(staging, apifiles.OpSet, "token")
	setSecret.SecretValue = ptr("sealed-token-2")
	edit("setSecret", setSecret)
	if strings.Contains(fileText(staging.File), "sealed-token-2") {
		t.Error("a secret value reached the file")
	}
	if got := sent(staging.ID, "token"); got != "v=sealed-token-2" {
		t.Errorf("send after setting the secret = %q", got)
	}

	// A plain value on a secret row is refused; so is a second lock of nothing.
	bad := env(staging, apifiles.OpSet, "token")
	bad.Value = ptr("x")
	if _, err := cfg.EditVariables(bridge.ApiEditVariablesArgs{Edits: []apifiles.VariableEdit{bad}}); testx.AsIpcErr(t, err).Code != "E_BAD_REQUEST" {
		t.Errorf("a plain value on a secret = %v, want E_BAD_REQUEST", err)
	}

	// Duplicate: the copy holds its own secret, sealed with the same value; editing it leaves the
	// original alone.
	dup, err := vs.DuplicateEnvironment(bridge.VariablesEnvironmentIDArgs{ID: staging.ID})
	if err != nil {
		t.Fatal(err)
	}
	copyText := fileText(dup.File)
	if strings.Contains(copyText, "${secret:staging.token}") || !strings.Contains(copyText, "${secret:staging-copy.token}") {
		t.Errorf("the copy shares the secret name:\n%s", copyText)
	}
	if got := sent(dup.ID, "token"); got != "v=sealed-token-2" {
		t.Errorf("send in the copy = %q, want the copied value", got)
	}
	copySet := env(dup, apifiles.OpSet, "token")
	copySet.SecretValue = ptr("copy-only-3")
	edit("copySecret", copySet)
	if got := sent(staging.ID, "token"); got != "v=sealed-token-2" {
		t.Errorf("the original's secret changed with the copy: %q", got)
	}
	if got := sent(dup.ID, "token"); got != "v=copy-only-3" {
		t.Errorf("send in the copy = %q", got)
	}

	// Unlock needs the confirmation; confirmed, the value goes back to the file as plain text and the
	// sealed value is released.
	unlock := env(staging, apifiles.OpUnlock, "token")
	if _, err := cfg.EditVariables(bridge.ApiEditVariablesArgs{Edits: []apifiles.VariableEdit{unlock}}); testx.AsIpcErr(t, err).Code != "E_BAD_REQUEST" {
		t.Errorf("unlock without confirmation = %v, want E_BAD_REQUEST", err)
	}
	if !strings.Contains(fileText(staging.File), "${secret:staging.token}") {
		t.Error("a refused unlock changed the file")
	}
	unlock.Confirmed = true
	edit("unlock", unlock)
	text = fileText(staging.File)
	if strings.Contains(text, "secret:") || !strings.Contains(text, "value: sealed-token-2") {
		t.Errorf("staging file after unlock:\n%s", text)
	}
	if view, err := cfg.View(); err != nil {
		t.Fatal(err)
	} else {
		for _, s := range view.Secrets {
			if s.Name == "staging.token" {
				t.Errorf("unlock left the sealed secret %+v", s)
			}
		}
	}

	// Description and sharing are file edits too.
	desc := env(staging, apifiles.OpDescribe, "host")
	desc.Description = ptr("API root")
	share := env(staging, apifiles.OpShare, "host")
	share.Agent = ptr(true)
	edit("describeShare", desc, share)
	text = fileText(staging.File)
	if !strings.Contains(text, "description: API root") || !strings.Contains(text, "agent: true") {
		t.Errorf("staging file after describe and share:\n%s", text)
	}

	// A broken file is refused whole and left as it is.
	broken := "name: Local\nvariables:\n  - name: a\n    value: 1\n    secret: ${secret:x}\n"
	app.WriteAPIFile(t, local.File, []byte(broken))
	probe := env(local, apifiles.OpSet, "page_size")
	probe.Value = ptr("1")
	_, err = cfg.EditVariables(bridge.ApiEditVariablesArgs{Edits: []apifiles.VariableEdit{probe}})
	if testx.AsIpcErr(t, err).Code != "E_BAD_REQUEST" || fileText(local.File) != broken {
		t.Errorf("edit on a broken file = %v, file %q", err, fileText(local.File))
	}

	// A Postman environment import names the secrets that arrived without a value.
	dir := t.TempDir()
	imp := filepath.Join(dir, "prod.json")
	if err := os.WriteFile(imp, []byte(`{"name":"Production","values":[{"key":"host","value":"https://prod.example.com"},{"key":"token","value":"","type":"secret"},{"key":"client_secret","value":"","type":"secret"},{"key":"key","value":"k","type":"secret"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	importArgs := bridge.CollectionsImportArgs{Paths: []string{imp}}
	app.Contract(t, sc, "args:CollectionsService.Import", importArgs, flowharness.Replace(dir, "<tmp>"))
	res, err := app.W.Collections.Import(importArgs)
	if err != nil || len(res.Files) != 1 || res.Files[0].Environment == nil {
		t.Fatalf("Import = %+v, %v", res, err)
	}
	app.Contract(t, sc, "CollectionsService.Import", res, flowharness.Replace(dir, "<tmp>"))
	if got := strings.Join(res.Files[0].Environment.SecretsUnset, ","); got != "token,client_secret" {
		t.Errorf("secretsUnset = %q, want token,client_secret", got)
	}
}
