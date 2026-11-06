package dbmcpflow

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// After a home fold the Claude Code registration names the moved helper script. The first start
// rewrites it once, against the Space home, and clears the flag.
func TestDbMcpReregisterAfterFold(t *testing.T) {
	app := flowharness.New(t)
	if _, err := app.W.DbMcp.SetEnabled(bridge.DbMcpSetEnabledArgs{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	app.McpInstaller.Known = map[string]bool{"kira-db": true}
	if _, err := app.DB().Exec(`INSERT INTO settings (key, value) VALUES ('homefold.p539', '{"state":"done","reregister":true}')
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`); err != nil {
		t.Fatal(err)
	}
	if n := len(app.McpInstaller.InstallCalls()); n != 0 {
		t.Fatalf("%d installs before the restart, want 0", n)
	}

	app.Restart()
	flagged := func() bool {
		var raw string
		if err := app.DB().QueryRow(`SELECT value FROM settings WHERE key = 'homefold.p539'`).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		return raw == `{"state":"done","reregister":true}`
	}
	testx.WaitUntil(t, 10*time.Second, func() bool { return !flagged() })
	installs := app.McpInstaller.InstallCalls()
	if len(installs) != 1 || installs[0].Name != "kira-db" || installs[0].Token != filepath.Join(app.SpaceHome, "mcp-header-helper.sh") {
		t.Fatalf("installs = %+v, want one kira-db with the Space helper path", installs)
	}

	app.Restart()
	if n := len(app.McpInstaller.InstallCalls()); n != 1 {
		t.Fatalf("%d installs after the second boot, want still 1", n)
	}
}
