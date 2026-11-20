package journeyflow_test

import (
	"reflect"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/windowsvc"
)

// J-BC-070: state set through bound services in every module survives a relaunch unchanged.
func TestRestartChecklist(t *testing.T) {
	app := flowharness.New(t)

	// Windows and tabs: two windows, own modes and tab sets, one active tab each.
	ws := "ws"
	tabs := map[string][]model.TabRecord{
		"w-a": {
			{ID: "a1", Path: "/a1", Kind: "terminal", State: []byte(`{}`), Order: 0, WorkspaceID: &ws},
			{ID: "a2", Path: "/a2", Kind: "terminal", State: []byte(`{}`), Order: 1, Active: true, WorkspaceID: &ws},
		},
		"w-b": {{ID: "b1", Path: "/b1", Kind: "repo-graph", State: []byte(`{}`), Active: true, WorkspaceID: &ws}},
	}
	for key, mode := range map[string]string{"w-a": "ade", "w-b": "automations"} {
		if _, err := app.W.WindowsSvc.Ensure(windowsvc.EnsureArgs{WindowKey: key}); err != nil {
			t.Fatal(err)
		}
		if err := app.W.WindowsSvc.SetMode(windowsvc.SetModeArgs{WindowKey: key, Mode: mode}); err != nil {
			t.Fatal(err)
		}
		if err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: key, Tabs: tabs[key]}); err != nil {
			t.Fatal(err)
		}
	}

	// Layout: ADE panel width and the cell editor dock height.
	width, height := 520.0, 300.0
	layout, err := app.W.Layout.Set(bridge.LayoutSetArgs{Patch: model.LayoutPatch{Panel: &model.PanelsPatch{
		Ade: &model.PanelAdePatch{Width: &width}, CellEditor: &model.PanelCellEditorPatch{Height: &height},
	}}})
	if err != nil {
		t.Fatal(err)
	}

	// Settings: API client limits and the data grid page size.
	timeout, maxMb, page := 45000, 12, 1000
	redirects := false
	if _, err := app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{
		Api:  &model.ApiPatch{RequestTimeoutMs: &timeout, MaxResponseMb: &maxMb, FollowRedirects: &redirects},
		Data: &model.DataPatch{DefaultPageSize: &page},
	}}); err != nil {
		t.Fatal(err)
	}
	settings, err := app.W.Settings.GetAll()
	if err != nil {
		t.Fatal(err)
	}

	// Database console: a saved query and its connection.
	conn, err := app.W.Connections.Create(connInput("pw"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.Queries.SaveConsole(bridge.QueriesSaveConsoleArgs{
		ConnectionID: conn.ID, Path: "main", Name: "recent orders", Body: model.ConsoleBody{Text: "select * from orders limit 5"}, Pinned: true,
	}); err != nil {
		t.Fatal(err)
	}
	queries, err := app.W.Queries.ListConsole(bridge.QueriesListArgs{ConnectionID: conn.ID, Path: "main"})
	if err != nil || len(queries) != 1 {
		t.Fatalf("ListConsole = %+v, %v", queries, err)
	}

	// API: the active environment.
	env := app.SeedEnvironment(t, "staging", flowharness.APIVar{Name: "host", Value: "stg.example"})
	if err := app.W.Variables.SetActiveEnvironment(bridge.VariablesEnvironmentIDArgs{ID: env.ID}); err != nil {
		t.Fatal(err)
	}
	envs, err := app.W.Variables.ListEnvironments()
	if err != nil {
		t.Fatal(err)
	}

	// ADE: the plan (board) order the user set.
	var taskIDs []string
	for _, title := range []string{"first", "second", "third"} {
		task, err := app.W.AdeTask.CaptureTask(ctx, adewire.CaptureTaskArgs{Title: title})
		if err != nil {
			t.Fatal(err)
		}
		taskIDs = append(taskIDs, task.ID)
	}
	wantOrder := []string{taskIDs[2], taskIDs[0], taskIDs[1]}
	if err := app.W.AdeTask.SetPlan(ctx, adewire.SetPlanArgs{Order: wantOrder}); err != nil {
		t.Fatal(err)
	}

	app.Restart()

	board, err := app.W.AdeTask.Board(ctx)
	if err != nil || !reflect.DeepEqual(board.Plan.Order, wantOrder) {
		t.Errorf("plan order after restart = %v, %v, want %v", board.Plan.Order, err, wantOrder)
	}

	for key, want := range tabs {
		got, err := app.W.Tabs.List(bridge.TabsListArgs{WindowKey: key})
		if err != nil || len(got) != len(want) {
			t.Fatalf("tabs %s after restart = %+v, %v", key, got, err)
		}
		for i, tab := range got {
			if tab.ID != want[i].ID || tab.Active != want[i].Active || tab.Order != want[i].Order || tab.Kind != want[i].Kind {
				t.Errorf("tab %s/%d = %+v, want %+v", key, i, tab, want[i])
			}
		}
	}
	for key, mode := range map[string]string{"w-a": "ade", "w-b": "automations"} {
		res, err := app.W.WindowsSvc.Ensure(windowsvc.EnsureArgs{WindowKey: key})
		if err != nil || res.Mode != mode {
			t.Errorf("window %s after restart = %+v, %v, want mode %s", key, res, err, mode)
		}
	}
	if got, err := app.W.Layout.GetAll(); err != nil || !reflect.DeepEqual(got, layout) {
		t.Errorf("layout after restart = %+v, %v, want %+v", got, err, layout)
	}
	if got, err := app.W.Settings.GetAll(); err != nil || !reflect.DeepEqual(got, settings) {
		t.Errorf("settings after restart = %+v, %v, want %+v", got, err, settings)
	}
	if got, err := app.W.Queries.ListConsole(bridge.QueriesListArgs{ConnectionID: conn.ID, Path: "main"}); err != nil || !reflect.DeepEqual(got, queries) {
		t.Errorf("saved console queries after restart = %+v, %v, want %+v", got, err, queries)
	}
	if got, err := app.W.Variables.ListEnvironments(); err != nil || !reflect.DeepEqual(got, envs) {
		t.Errorf("environments after restart = %+v, %v, want %+v", got, err, envs)
	}
	// Connections reopen disconnected: a relaunch never auto-connects or reruns a console.
	list, err := app.W.Connections.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("connections after restart = %+v, %v", list, err)
	}
	states, err := app.W.Connections.States()
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range states {
		if st.Status != "disconnected" {
			t.Errorf("connection state after restart = %+v, want disconnected", st)
		}
	}
}
