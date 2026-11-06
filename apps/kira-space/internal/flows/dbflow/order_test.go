package dbflow

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func connOrder(t *testing.T, list []model.ConnectionSummary, want ...string) {
	t.Helper()
	if len(list) != len(want) {
		t.Fatalf("order has %d connections, want %d", len(list), len(want))
	}
	for i, c := range list {
		if c.ID != want[i] {
			t.Fatalf("order[%d] = %s, want %s", i, c.ID, want[i])
		}
	}
}

func TestConnectionOrder(t *testing.T) {
	app := flowharness.New(t)
	path := newSQLiteFile(t)
	a := createConn(t, app, sqliteInput("a", path))
	b := createConn(t, app, sqliteInput("b", path))
	c := createConn(t, app, sqliteInput("c", path))

	mark := app.Events.Mark()
	args := bridge.ConnectionsReorderArgs{IDs: []string{c.ID, a.ID, b.ID}}
	got, err := app.W.Connections.Reorder(args)
	if err != nil {
		t.Fatal(err)
	}
	connOrder(t, got, c.ID, a.ID, b.ID)
	listed, err := app.W.Connections.List()
	if err != nil {
		t.Fatal(err)
	}
	connOrder(t, listed, c.ID, a.ID, b.ID)

	ev := app.Events.WaitAfter(t, mark, bridge.ChannelConnectionsChanged, nil, 5*time.Second)
	var changed []model.ConnectionSummary
	ev.Decode(t, &changed)
	connOrder(t, changed, c.ID, a.ID, b.ID)

	// Contract connection-order: tests/ui/connection-order.spec.ts drags and sends these args.
	app.Contract(t, "connection-order", "args:ConnectionsService.Reorder", args)
	app.Contract(t, "connection-order", "ConnectionsService.Reorder", got, flowharness.Mask("createdAt", "updatedAt"), flowharness.Replace(filepath.Dir(path), "<fixture>"))

	if _, err := app.W.Connections.Reorder(bridge.ConnectionsReorderArgs{IDs: []string{a.ID, a.ID}}); err == nil {
		t.Fatal("Reorder with a duplicate id succeeded")
	}
	if _, err := app.W.Connections.Reorder(bridge.ConnectionsReorderArgs{}); err == nil {
		t.Fatal("Reorder with no ids succeeded")
	}

	d := createConn(t, app, sqliteInput("d", path))
	app.Restart()
	again, err := app.W.Connections.List()
	if err != nil {
		t.Fatal(err)
	}
	connOrder(t, again, c.ID, a.ID, b.ID, d.ID)
}
