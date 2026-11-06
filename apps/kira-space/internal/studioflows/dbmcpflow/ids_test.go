package dbmcpflow

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
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
	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := (&repos.OpsRepo{DB: f.app.DB()}).Append(model.OpAppend{ID: legacy, Kind: "execute", StartedAt: startedAt, Source: "user"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.opRow(t, legacy); !ok {
		t.Fatal("a v4 op row does not list")
	}
	if ok, err := f.app.W.Router.Cancel(ctx, legacy); ok || err != nil {
		t.Fatalf("Cancel(v4 id) = %v, %v; want false, nil", ok, err)
	}
	if err := f.exec("", "select 1"); err != nil {
		t.Fatal(err)
	}
	recent, err := f.app.W.StudioOps.Recent(bridge.StudioOpsRecentArgs{Limit: 50})
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
