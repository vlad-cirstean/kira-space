package dbflow

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// stalePassword stands in for a rotated-away secret.
const stalePassword = "stale-password-0e4"

// connectState connects id and returns the state; a refused connect and an error state both come
// back as "error".
func connectState(t *testing.T, app *flowharness.App, id string) string {
	t.Helper()
	st, err := app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: id})
	if err != nil {
		return "error"
	}
	return st.Status
}

// leaks lists the files under root that hold secret as plain bytes.
func leaks(t *testing.T, root, secret string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr == nil && bytes.Contains(b, []byte(secret)) {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// J-CONN-001 (contract paste-credentials): a pasted `.env` block becomes one UpdateCredentials call;
// the connection file never holds the password and the connection reconnects after a relaunch.
// tests/ui/studio/connection-paste-credentials.spec.ts pastes the same block and asserts the call.
func TestPasteCredentialsSurviveRestart(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartPostgres(t)
	in := startPgInput(fx)
	app := flowharness.New(t)

	stale := in
	stale.Password = ptr(stalePassword)
	conn := createConn(t, app, stale)
	if got := connectState(t, app, conn.ID); got != "error" {
		t.Fatalf("connect with a stale password = %s, want error", got)
	}

	env := "PGHOST=elsewhere\nPGUSER=" + *in.Username + "\nPGPASSWORD=" + *in.Password
	app.Contract(t, "paste-credentials", "paste#env", map[string]string{
		"env": env, "username": *in.Username, "password": *in.Password,
	})
	args := bridge.ConnectionsCredentialsArgs{ID: conn.ID, Username: in.Username, Password: in.Password}
	app.Contract(t, "paste-credentials", "args:ConnectionsService.UpdateCredentials", args, flowharness.Mask("id"))
	if err := app.W.Connections.UpdateCredentials(args); err != nil {
		t.Fatal(err)
	}
	if got := connectState(t, app, conn.ID); got != "connected" {
		t.Fatalf("connect after the paste = %s, want connected", got)
	}

	file := filepath.Join(app.W.ConnectionFiles.Dir(), conn.File)
	if b, err := os.ReadFile(file); err != nil || bytes.Contains(b, []byte(*in.Password)) || bytes.Contains(b, []byte(stalePassword)) {
		t.Fatalf("connection file (%v) holds a password:\n%s", err, b)
	}
	if bad := leaks(t, app.ConfigDir("connections"), *in.Password); len(bad) > 0 {
		t.Fatalf("password bytes in %v", bad)
	}

	app.Restart()
	if got := connectState(t, app, conn.ID); got != "connected" {
		t.Fatalf("connect after a relaunch = %s, want connected", got)
	}
	if bad := leaks(t, app.SpaceHome, *in.Password); len(bad) > 0 {
		t.Fatalf("password bytes at rest in %v", bad)
	}
}

// J-CONN-002: a duplicate owns its password. Rotating the copy's secret leaves the original
// connecting with the old one; the copy fails until its own secret is right.
func TestDuplicateRotatesIndependently(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartPostgres(t)
	in := startPgInput(fx)
	app := flowharness.New(t)

	conn := createConn(t, app, in)
	dup, err := app.W.Connections.Duplicate(bridge.ConnectionsIDArgs{ID: conn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := connectState(t, app, dup.ID); got != "connected" {
		t.Fatalf("copy with the inherited password = %s, want connected", got)
	}
	if err := app.W.Connections.UpdateCredentials(bridge.ConnectionsCredentialsArgs{ID: dup.ID, Password: ptr(stalePassword)}); err != nil {
		t.Fatal(err)
	}
	if got := connectState(t, app, conn.ID); got != "connected" {
		t.Fatalf("original after the copy rotated = %s, want connected with its old secret", got)
	}
	if got := connectState(t, app, dup.ID); got != "error" {
		t.Fatalf("copy with the rotated secret = %s, want error", got)
	}
	if err := app.W.Connections.UpdateCredentials(bridge.ConnectionsCredentialsArgs{ID: dup.ID, Password: in.Password}); err != nil {
		t.Fatal(err)
	}
	if got := connectState(t, app, dup.ID); got != "connected" {
		t.Fatalf("copy after the right secret = %s, want connected", got)
	}
}
