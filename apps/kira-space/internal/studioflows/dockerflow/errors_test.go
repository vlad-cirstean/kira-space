package dockerflow

import (
	"errors"
	"testing"

	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Mirrors tests/ui/consistency-docker-automations.spec.ts: container detail shows this text.
func TestStartMissingContainer(t *testing.T) {
	app, _ := boot(t)
	err := app.W.Docker.Start(docker.IDArgs{ID: "kira-flow-no-such-container"})
	var ie *ipcerr.Error
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v, want *ipcerr.Error", err)
	}
	if ie.Code != "E_NOT_FOUND" || ie.Message != "That Docker item no longer exists." {
		t.Fatalf("err = %q %q, want E_NOT_FOUND and the catalog text", ie.Code, ie.Message)
	}
}
