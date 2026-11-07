package appflow_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Startup removes what retired builds left behind.
func TestStartupSweepsLeftovers(t *testing.T) {
	var sock, lock string
	flowharness.New(t, flowharness.WithSeed(func(app *flowharness.App) {
		sock = filepath.Join(app.SpaceHome, "git.sock")
		lock = filepath.Join(app.SpaceHome, "git.sock.lock")
		if err := os.MkdirAll(app.SpaceHome, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{sock, lock} {
			if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}))

	testx.WaitUntil(t, 5*time.Second, func() bool {
		_, errSock := os.Stat(sock)
		_, errLock := os.Stat(lock)
		return os.IsNotExist(errSock) && os.IsNotExist(errLock)
	})
}
