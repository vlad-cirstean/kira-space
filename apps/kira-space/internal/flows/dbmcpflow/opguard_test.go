package dbmcpflow

import (
	"errors"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
)

func (f *fixture) exec(opID, statement string) error {
	_, err := f.app.W.Router.Execute(ctx, adapterhost.ExecuteRequestWire{OpID: opID, ConnectionID: f.connID, Statements: []string{statement}})
	return err
}

// Backend half of tests/ui/op-id-guard.spec.ts.
func TestOpIDGuard(t *testing.T) {
	f := newFixture(t)
	if _, err := f.app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: f.connID}); err != nil {
		t.Fatal(err)
	}

	for _, bad := range []string{"not-a-uuid", "0123\nforged log line", strings.Repeat("a", 37)} {
		err := f.exec(bad, "select 1")
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeQuery || ae.Message != "The operation ID is not valid." {
			t.Fatalf("Execute(%q) = %v, want E_QUERY, operation id is not valid", bad, err)
		}
		if strings.Contains(err.Error(), strings.TrimSpace(bad)) {
			t.Fatalf("error echoes the id: %v", err)
		}
		if _, ok := f.opRow(t, bad); ok {
			t.Fatalf("op_log has a row for refused id %q", bad)
		}
		if _, _, err := f.app.W.Router.Host().Cancellable(ctx, bad); err == nil {
			t.Fatalf("Cancellable(%q) accepted a malformed id", bad)
		}
		if ok, err := f.app.W.Router.Cancel(ctx, bad); ok || err != nil {
			t.Fatalf("Cancel(%q) = %v, %v; want false, nil", bad, ok, err)
		}
	}

	// Reuse: the second op's history write fails; the first op's row stays intact.
	reused := ids.NewID()
	if err := f.exec(reused, "select 1 as first"); err != nil {
		t.Fatal(err)
	}
	f.waitStatus(t, reused, "ok")
	if err := f.exec(reused, "select 2 as second"); err != nil {
		t.Fatal(err)
	}
	sentinel := ids.NewID()
	if err := f.exec(sentinel, "select 3"); err != nil {
		t.Fatal(err)
	}
	f.waitStatus(t, sentinel, "ok")
	row, ok := f.opRow(t, reused)
	if !ok || row.Status != "ok" || row.Command == nil || !strings.Contains(*row.Command, "first") {
		t.Fatalf("reused op row = %+v, want the first op's row unchanged", row)
	}
	n := 0
	recent, err := f.app.W.StudioOps.Recent(bridge.StudioOpsRecentArgs{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recent {
		if r.ID == reused {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("op_log has %d rows for the reused id, want 1", n)
	}
	f.app.Contract(t, "op-id-guard", "StudioOpsService.Recent", []model.DbOpRecord{row}, flowharness.Mask("startedAt", "durationMs", "connectionId"))

	// Early cancel still works for a UUID.
	early := ids.NewID()
	if ok, err := f.app.W.Router.Cancel(ctx, early); ok || err != nil {
		t.Fatalf("early Cancel = %v, %v", ok, err)
	}
	err = f.exec(early, "select 1")
	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.CodeCancelled {
		t.Fatalf("Run after early cancel = %v, want E_CANCELLED", err)
	}
}
