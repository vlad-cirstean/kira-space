package testsupport

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

// ReadLaneCase drives ReadLaneRefusesWrite. Setup and Teardown run on the main (writable)
// adapter; Write runs on a fresh lane adapter and must fail with WantErr in its message (the
// engine's own refusal, not Kira's classifier text). Unchanged then proves the write did not land.
type ReadLaneCase struct {
	Path      model.NodePath
	Setup     []string
	Write     string
	WantErr   string
	Unchanged func(t *testing.T, main adapters.Adapter)
	Teardown  []string
}

func laneAdapter(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) adapters.Adapter {
	t.Helper()
	lane, err := adapters.CreateAdapter(a.Kind(), adapters.Deps{Log: func(string, string) {}})
	if err != nil {
		t.Fatalf("CreateAdapter(%s): %v", a.Kind(), err)
	}
	if _, err := lane.Connect(context.Background(), adapters.LaneConfig(cfg), adapters.NewOpCtx("lane-connect")); err != nil {
		t.Fatalf("lane Connect: %v", err)
	}
	t.Cleanup(func() { _ = lane.Disconnect(context.Background()) })
	return lane
}

func execAll(t *testing.T, a adapters.Adapter, path model.NodePath, statements []string) {
	t.Helper()
	for _, sql := range statements {
		if _, err := a.Execute(context.Background(), model.ConsoleRequest{Path: path, Statements: []string{sql}},
			adapters.NewOpCtx("lane-setup")); err != nil {
			t.Fatalf("Execute(%s): %v", sql, err)
		}
	}
}

// FirstCell runs sql on a and returns the first cell of the first tabular page, for Unchanged checks.
func FirstCell(t *testing.T, a adapters.Adapter, path model.NodePath, sql string) string {
	t.Helper()
	pages, err := a.Execute(context.Background(), model.ConsoleRequest{Path: path, Statements: []string{sql}},
		adapters.NewOpCtx("lane-verify"))
	if err != nil {
		t.Fatalf("Execute(%s): %v", sql, err)
	}
	p, ok := pages[0].(page.TabularPage)
	if !ok || p.RowCount == 0 {
		t.Fatalf("Execute(%s): want a tabular page with a row, got %T", sql, pages[0])
	}
	return page.CellText(p.Chunks[0], 0)
}

// ReadLaneRefusesWrite asserts the DB MCP read lane (a second adapter on adapters.LaneConfig of the
// same credentials) makes the engine itself refuse Write, and that nothing changed. Gated on c.SQL
// plus the kind's own reading of the lane (document and key-value kinds enforce in Kira's code).
func ReadLaneRefusesWrite(c ReadLaneCase) Scenario {
	return Scenario{
		Name:     "read lane: the engine refuses a write",
		Requires: func(caps adapters.Caps) bool { return caps.SQL },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			execAll(t, a, c.Path, c.Setup)
			t.Cleanup(func() { execAll(t, a, c.Path, c.Teardown) })
			lane := laneAdapter(t, a, cfg)
			_, err := lane.Execute(context.Background(), model.ConsoleRequest{
				Path: c.Path, Statements: []string{c.Write}, ReadOnly: true,
			}, adapters.NewOpCtx("lane-write"))
			if err == nil {
				t.Fatalf("lane Execute(%s): want a refusal, got nil", c.Write)
			}
			if !strings.Contains(err.Error(), c.WantErr) {
				t.Errorf("lane refusal = %q, want it to contain %q", err.Error(), c.WantErr)
			}
			if c.Unchanged != nil {
				c.Unchanged(t, a)
			}
		},
	}
}

// LaneCancel asserts Cancel on a lane adapter ends a long query server-side: Execute's own context
// stays live, so only the server-side cancel can stop longSQL before its natural end. Gated on c.Cancel.
func LaneCancel(path model.NodePath, longSQL string) Scenario {
	return Scenario{
		Name:     "read lane: cancel ends the query on the lane's connection",
		Requires: func(c adapters.Caps) bool { return c.Cancel && c.SQL },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			lane := laneAdapter(t, a, cfg)
			errc := make(chan error, 1)
			go func() {
				_, err := lane.Execute(context.Background(), model.ConsoleRequest{
					Path: path, Statements: []string{longSQL}, ReadOnly: true,
				}, adapters.NewOpCtx("lane-long"))
				errc <- err
			}()
			time.Sleep(700 * time.Millisecond)
			if _, err := lane.Cancel(context.Background(), "lane-long"); err != nil {
				t.Fatalf("lane Cancel: %v", err)
			}
			select {
			case err := <-errc:
				if err == nil {
					t.Fatal("lane query: want an error after cancel, got nil")
				}
			case <-time.After(8 * time.Second):
				t.Fatal("lane query still running 8s after Cancel")
			}
		},
	}
}
