package bridge_test

import (
	"errors"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// TestConnectionsServiceRejectsBareID is a regression test for the finding that
// bridge.ConnectionsService's id-taking methods — unlike bridge/tree.go, ops.go, queries.go and
// filters.go, which all guard their own connectionId/id args the same way — never checked for an
// empty id before handing it to internal/connections.Service. Deps.Connections is left nil here on
// purpose: every method below must reject a bare id before ever touching it, so if a guard were
// missing or removed, this test would panic on a nil pointer dereference rather than silently
// pass.
func TestConnectionsServiceRejectsBareID(t *testing.T) {
	svc := &bridge.ConnectionsService{Deps: appcore.Deps{}}

	assertContract := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("want an error for a bare id, got nil")
		}
		var ie *ipcerr.Error
		if !errors.As(err, &ie) {
			t.Fatalf("err = %v, want *ipcerr.Error", err)
		}
		if ie.Code != "E_CONTRACT" {
			t.Errorf("Code = %q, want E_CONTRACT", ie.Code)
		}
	}

	t.Run("UpdateCredentials", func(t *testing.T) {
		assertContract(t, svc.UpdateCredentials(bridge.ConnectionsCredentialsArgs{ID: ""}))
	})
	t.Run("Duplicate", func(t *testing.T) {
		_, err := svc.Duplicate(bridge.ConnectionsIDArgs{ID: ""})
		assertContract(t, err)
	})
	t.Run("Remove", func(t *testing.T) {
		assertContract(t, svc.Delete(bridge.ConnectionsIDArgs{ID: ""}))
	})
	t.Run("Connect", func(t *testing.T) {
		_, err := svc.Connect(bridge.ConnectionsIDArgs{ID: ""})
		assertContract(t, err)
	})
	t.Run("Disconnect", func(t *testing.T) {
		_, err := svc.Disconnect(bridge.ConnectionsIDArgs{ID: ""})
		assertContract(t, err)
	})
	t.Run("Reveal never errors, but still reports a bare id rather than reaching Deps.Connections", func(t *testing.T) {
		result := svc.Reveal(bridge.ConnectionsRevealArgs{ID: ""})
		if result.Error == nil {
			t.Fatal("Reveal(bare id).Error = nil, want a message")
		}
		if result.Password != nil {
			t.Errorf("Reveal(bare id).Password = %v, want nil", result.Password)
		}
	})
}
