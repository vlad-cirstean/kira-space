package dbflow

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// discoverRequest is the statement text of a Kibana run.
func discoverRequest(t *testing.T, req model.KibanaDiscoverRequest) string {
	t.Helper()
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func executeWire(opID, connID, statement string) adapterhost.ExecuteRequestWire {
	return adapterhost.ExecuteRequestWire{OpID: opID, ConnectionID: connID, Statements: []string{statement}}
}

// TestKibanaDiscoverJourney is the backend half of the Discover scenario: connect, list data
// views, read fields, run a query with a filter and a sort, histogram it and ask for value hints.
// Contract kibana-discover: tests/ui/kibana-discover.spec.ts answers its calls with these results.
func TestKibanaDiscoverJourney(t *testing.T) {
	flowharness.Complete(t)
	flowharness.RequireDocker(t)
	fx := testsupport.StartKibana(t)
	app := flowharness.New(t)

	in := connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "flow kibana", Kind: "kibana", Color: "indigo", Mode: "fields",
		Host: fx.Config.Host, Port: fx.Config.Port, Username: fx.Config.Username,
		Options: map[string]any{}, McpReadMode: "deny", McpWriteMode: "deny", McpDdlMode: "deny",
	}, Password: fx.Config.Password}
	conn := createConn(t, app, in)

	if res := app.W.Connections.Test(bridge.ConnectionsIDArgs{ID: conn.ID}); !res.OK || res.ServerVersion == nil {
		t.Fatalf("Test = %+v, want ok with a server version", res)
	}
	st := connect(t, app, conn.ID)
	if st.Status != "connected" {
		t.Fatalf("Connect = %+v, want connected", st)
	}
	app.StudioContract(t, "kibana-discover", "ConnectionsService.Connect#connected", st,
		flowharness.Mask("since", "serverVersion"))

	kids, err := app.W.Tree.Children(bridge.TreeChildrenArgs{ConnectionID: conn.ID, Path: ""})
	if err != nil {
		t.Fatal(err)
	}
	var logsPath string
	for _, n := range kids.Nodes {
		if n.Kind != "collection" {
			t.Fatalf("root node %+v, want only data views (collection)", n)
		}
		if n.Name == testsupport.KibanaLogsView {
			logsPath = n.Path
		}
	}
	if logsPath == "" {
		t.Fatalf("data view %q not in the tree: %+v", testsupport.KibanaLogsView, kids.Nodes)
	}
	app.StudioContract(t, "kibana-discover", "TreeService.Children#dataViews", kids)

	views, err := app.W.Kibana.DataViews(ctx, model.KibanaDataViewsRequest{ConnectionID: conn.ID, OpID: "56f267ab-fc96-7c8d-878f-3998a2fe8a06"})
	if err != nil || len(views) < 3 {
		t.Fatalf("DataViews = %d (%v), want the seeded views", len(views), err)
	}
	app.StudioContract(t, "kibana-discover", "KibanaService.DataViews#list", views)

	view, err := app.W.Kibana.DataView(ctx, model.KibanaDataViewRequest{ConnectionID: conn.ID, OpID: "3cdb70c3-388f-7d1b-8418-661d3b8dec8a", ID: testsupport.KibanaLogsView})
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(view.Fields))
	for i, f := range view.Fields {
		names[i] = f.Name
	}
	for _, want := range []string{"@timestamp", "level", "status", "status_class"} {
		if !slices.Contains(names, want) {
			t.Fatalf("fields %v lack %q", names, want)
		}
	}
	app.StudioContract(t, "kibana-discover", "KibanaService.DataView#logs", view)

	window := &model.KibanaTimeRange{From: testsupport.KibanaWindowFrom, To: testsupport.KibanaWindowTo}
	base := model.KibanaDiscoverRequest{
		DataView: testsupport.KibanaLogsView, Query: "level:error", Language: "kuery", TimeRange: window,
		Filters: []model.KibanaFilter{}, Sort: []model.KibanaSort{}, Size: 500, TimeZone: "UTC",
	}
	pages := execute(t, app, conn.ID, discoverRequest(t, base))
	if len(pages) != 1 {
		t.Fatalf("Execute returned %d pages, want 1", len(pages))
	}
	doc, ok := pages[0].(page.DocumentPage)
	if !ok || doc.RowCount != testsupport.KibanaDocCount/4 {
		t.Fatalf("Execute page = %#v, want %d error documents", pages[0], testsupport.KibanaDocCount/4)
	}

	filtered := base
	filtered.Filters = []model.KibanaFilter{{Field: "status", Operator: "between", Values: []string{"500", "503"}}}
	filtered.Sort = []model.KibanaSort{{Field: "status", Order: "desc"}}
	narrowed := execute(t, app, conn.ID, discoverRequest(t, filtered))
	if d, ok := narrowed[0].(page.DocumentPage); !ok || d.RowCount == 0 || d.RowCount >= doc.RowCount {
		t.Fatalf("filtered page = %#v, want fewer than %d documents", narrowed[0], doc.RowCount)
	}

	withColumns := base
	withColumns.Columns = []string{"level", "status"}
	tabular := execute(t, app, conn.ID, discoverRequest(t, withColumns))
	tp, ok := tabular[0].(page.TabularPage)
	if !ok || tp.RowCount != doc.RowCount || len(tp.Columns) != 3 {
		t.Fatalf("columns page = %#v, want %d rows of @timestamp, level, status", tabular[0], doc.RowCount)
	}
	colNames := make([]string, len(tp.Columns))
	for i, c := range tp.Columns {
		colNames[i] = c.Name
	}
	if !slices.Equal(colNames, []string{"@timestamp", "level", "status"}) {
		t.Fatalf("columns = %v, want @timestamp, level, status", colNames)
	}
	app.StudioContract(t, "kibana-discover", "Execute#columns", colNames)

	hist, err := app.W.Kibana.Histogram(ctx, model.KibanaHistogramRequest{
		ConnectionID: conn.ID, OpID: "30268575-f547-762e-87e9-66fee18246e5", TabID: "tab", Request: base, Interval: "auto",
	})
	if err != nil {
		t.Fatal(err)
	}
	var sum int64
	for _, b := range hist.Buckets {
		sum += b.Count
	}
	if hist.Total != int64(doc.RowCount) || sum != hist.Total || hist.Relation != "eq" {
		t.Fatalf("Histogram = total %d (%s), buckets sum %d, want %d", hist.Total, hist.Relation, sum, doc.RowCount)
	}
	app.StudioContract(t, "kibana-discover", "KibanaService.Histogram#auto", hist)

	suggestReq := base
	suggestReq.Query, suggestReq.Size = "", 0
	sugg, err := app.W.Kibana.Suggest(ctx, model.KibanaSuggestRequest{
		ConnectionID: conn.ID, OpID: "c9041db7-0f6b-726e-8f6f-8cc1fe63ffdf", TabID: "tab", Request: suggestReq, Field: "level",
	})
	if err != nil || !sugg.Complete || !slices.Equal(sugg.Values, []string{"debug", "error", "info", "warn"}) {
		t.Fatalf("Suggest = %+v (%v), want the four levels, complete", sugg, err)
	}
	app.StudioContract(t, "kibana-discover", "KibanaService.Suggest#level", sugg)

	bad := base
	bad.Query = "level:"
	if _, err := app.W.Router.Execute(ctx, executeWire("kb-bad", conn.ID, discoverRequest(t, bad))); err == nil {
		t.Fatal("a syntax error in the query must fail the run")
	} else {
		app.StudioContract(t, "kibana-discover", "Execute#syntaxError", err.Error())
	}

	if st, err := app.W.Connections.Disconnect(bridge.ConnectionsIDArgs{ID: conn.ID}); err != nil || st.Status == "connected" {
		t.Fatalf("Disconnect = %+v (%v)", st, err)
	}
}
