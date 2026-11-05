package dbmcpflow

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/ids"
)

// Go mints the op id when the wire carries none; the history row key is a UUID.
func TestOpIDs(t *testing.T) {
	f := newFixture(t)
	if _, err := f.app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: f.connID}); err != nil {
		t.Fatal(err)
	}
	if err := f.exec("", "select 1"); err != nil {
		t.Fatal(err)
	}
	recent, err := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) == 0 {
		t.Fatal("no op row for an id-less Execute")
	}
	for _, r := range recent {
		if !ids.IsUUID(r.ID) {
			t.Fatalf("op row id %q is not a UUID", r.ID)
		}
	}
}
