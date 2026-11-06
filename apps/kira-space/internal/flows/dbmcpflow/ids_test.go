package dbmcpflow

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/ids"
)

// Go mints the op id when the wire carries none; the history row key is a UUID.
func TestOpIDs(t *testing.T) {
	f := newFixture(t)
	if _, err := f.app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: f.connID}); err != nil {
		t.Fatal(err)
	}
	if u, err := uuid.Parse(f.connID); err != nil || u.Version() != 7 {
		t.Fatalf("connection id %q is not a v7 UUID", f.connID)
	}
	legacy := uuid.NewString()
	if ok, err := f.app.W.Router.Cancel(ctx, legacy); ok || err != nil {
		t.Fatalf("Cancel(v4 id) = %v, %v; want false, nil", ok, err)
	}
	if err := f.exec("", "select 1"); err != nil {
		t.Fatal(err)
	}
	// Db events reach the op log through an async subscriber.
	var recent []oplog.Record
	deadline := time.Now().Add(5 * time.Second)
	for len(recent) == 0 {
		var err error
		if recent, err = f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 50}); err != nil {
			t.Fatal(err)
		}
		if len(recent) == 0 {
			if time.Now().After(deadline) {
				t.Fatal("no op row for an id-less Execute")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	for _, r := range recent {
		if !ids.IsUUID(r.ID) {
			t.Fatalf("op row id %q is not a UUID", r.ID)
		}
	}
}
