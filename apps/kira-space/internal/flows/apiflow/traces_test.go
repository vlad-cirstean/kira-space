package apiflow

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func TestIncognitoLeavesNoTrace(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)
	gsrv := studioflowharness.GRPC(t)

	// Control: the same sends without incognito leave rows and bytes, so the scan below is meaningful.
	control := get(srv.URL + "/status?path=control-9e8d7c")
	send(t, app, control)
	controlCall := bridge.GrpcCallArgs{
		OpID: newOp(), TabID: "tab-1", WindowKey: "w1", DescriptorMode: "reflection", Target: gsrv.Addr,
		Service: "kira.flow.v1.Flow", Method: "Unary", MessageJSON: `{"text":"control-msg-4b1f"}`,
	}
	if _, err := app.W.Grpc.Call(ctx, controlCall); err != nil {
		t.Fatal(err)
	}
	waitOp(t, app, control.OpID)
	waitOp(t, app, controlCall.OpID)

	hidden := get(srv.URL + "/status?path=incognito-3a5c71")
	hidden.TabID, hidden.Incognito = "tab-incognito", true
	send(t, app, hidden)
	hiddenCall := controlCall
	hiddenCall.OpID, hiddenCall.TabID, hiddenCall.Incognito = newOp(), "tab-incognito", true
	hiddenCall.MessageJSON = `{"text":"incognito-msg-8d20"}`
	if _, err := app.W.Grpc.Call(ctx, hiddenCall); err != nil {
		t.Fatal(err)
	}
	waitOpUpdate(t, app, hidden.OpID)
	waitOpUpdate(t, app, hiddenCall.OpID)

	if rows, _ := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{TabID: "tab-incognito"}); len(rows) != 0 {
		t.Errorf("incognito HTTP send left %d history rows", len(rows))
	}
	if rows, _ := app.W.GrpcHistory.List(bridge.GrpcHistoryScopeArgs{TabID: "tab-incognito"}); len(rows) != 0 {
		t.Errorf("incognito gRPC call left %d history rows", len(rows))
	}
	recs, err := app.W.StudioOps.Recent(bridge.StudioOpsRecentArgs{Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range recs {
		ids = append(ids, r.ID)
		if r.ID == hidden.OpID || r.ID == hiddenCall.OpID {
			t.Errorf("op log persisted incognito op %s", r.ID)
		}
	}
	if len(ids) != 2 {
		t.Errorf("op log rows = %v, want only the two control ops", ids)
	}

	db := dbBytes(t, app)
	for _, marker := range []string{"control-9e8d7c", "control-msg-4b1f"} {
		if !contains(db, marker) {
			t.Errorf("database lacks the control marker %q; the scan reads the wrong files", marker)
		}
	}
	for _, marker := range []string{"incognito-3a5c71", "incognito-msg-8d20", "tab-incognito"} {
		if contains(db, marker) {
			t.Errorf("database bytes contain %q from an incognito request", marker)
		}
	}
}

// Dynamic references ({{$uuid}}, {{$timestamp}}) are resolved by the renderer before a send; the
// IPC layer carries them through untouched and history keeps the template.
func TestDynamicValues(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)
	col := newCollection(t, app, "dyn")
	env := app.SeedEnvironment(t, "dev", studioflowharness.APIVar{Name: "sk", Value: "dyn-secret-77", Secret: true})

	args := get(srv.URL + "/echo?id={{$uuid}}&ts={{$timestamp}}&k={{sk}}")
	args.CollectionID, args.EnvironmentID = col.ID, env.ID
	send(t, app, args)
	send(t, app, func() bridge.HttpSendArgs { a := args; a.OpID = newOp(); return a }())

	reqs := srv.Requests()
	for i, r := range reqs {
		if r.Query != "id={{$uuid}}&ts={{$timestamp}}&k=dyn-secret-77" {
			t.Errorf("request %d query = %q, want dynamic references verbatim and the secret resolved", i, r.Query)
		}
	}
	rows, err := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{TabID: "tab-1"})
	if err != nil || len(rows) != 2 {
		t.Fatalf("history = %d rows (%v), want 2", len(rows), err)
	}
	for _, row := range rows {
		if !strings.Contains(row.URL, "{{$uuid}}") || !strings.Contains(row.URL, "{{sk}}") {
			t.Errorf("history url = %q, want the template", row.URL)
		}
	}
}

func TestRestartKeepsApiState(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)
	col := newCollection(t, app, "persist")
	folder, err := app.W.Collections.CreateItem(bridge.CollectionsCreateItemArgs{CollectionID: col.ID, Kind: "folder", Name: "folder"})
	if err != nil {
		t.Fatal(err)
	}
	req := savedGet()
	req.URL = srv.URL + "/echo"
	if _, err := app.W.Collections.CreateItem(bridge.CollectionsCreateItemArgs{
		CollectionID: col.ID, ParentID: &folder.ID, Kind: "request", Name: "req", Request: &req,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.Collections.CreateGrpcItem(bridge.CollectionsCreateGrpcItemArgs{
		CollectionID: col.ID, Name: "rpc",
		Request: &model.SavedGrpcRequest{TLSMode: "plaintext", DescriptorMode: "reflection", Target: "localhost:1"},
	}); err != nil {
		t.Fatal(err)
	}
	env := app.SeedEnvironment(t, "dev", studioflowharness.APIVar{Name: "tok", Value: "persist-secret-1", Secret: true})
	if err := app.W.Variables.SetActiveEnvironment(bridge.VariablesEnvironmentIDArgs{ID: env.ID}); err != nil {
		t.Fatal(err)
	}
	before, err := app.W.Collections.List()
	if err != nil {
		t.Fatal(err)
	}

	app.Restart(t)

	after, err := app.W.Collections.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Collections) != 1 || len(after.Items) != 3 {
		t.Fatalf("tree after restart = %d collections, %d items; want 1 and 3", len(after.Collections), len(after.Items))
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("tree changed across restart:\n before %+v\n after  %+v", before, after)
	}
	// Contract api-restart, read by tests/ui/collections.spec.ts "after a restart the tree and the
	// active environment come back".
	app.Contract(t, "api-restart", "CollectionsService.List", after, studioflowharness.Replace(srv.URL, "<http>"))
	envs, err := app.W.Variables.ListEnvironments()
	if err != nil || len(envs) != 1 || !envs[0].IsActive || envs[0].ID != env.ID {
		t.Fatalf("environments after restart = %+v (%v), want dev still active", envs, err)
	}
	app.Contract(t, "api-restart", "VariablesService.ListEnvironments", envs)
	vars, err := app.W.Variables.List(bridge.VariablesScopeArgs{Scope: model.VariableScopeEnvironment, OwnerID: env.ID})
	if err != nil || len(vars) != 1 {
		t.Fatalf("variables = %+v (%v)", vars, err)
	}
	rev := app.W.Variables.Reveal(bridge.VariablesRevealArgs{VariableID: vars[0].ID, Confirmed: true})
	if rev.Outcome != "revealed" || rev.Value == nil || *rev.Value != "persist-secret-1" {
		t.Errorf("reveal after restart = %+v", rev)
	}
	args := get(srv.URL + "/bearer")
	args.CollectionID, args.EnvironmentID = col.ID, env.ID
	args.Headers = []httpclient.Header{{Name: "Authorization", Value: "Bearer {{tok}}"}}
	send(t, app, args)
	if got := lastRequest(srv).Header.Get("Authorization"); got != "Bearer persist-secret-1" {
		t.Errorf("server Authorization after restart = %q", got)
	}
}
