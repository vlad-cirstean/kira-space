package dbflow

import (
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// startPostgres runs a throwaway Postgres on a random port; the journey seeds it through the app.
func startPostgres(t *testing.T) connections.Input {
	t.Helper()
	flowharness.RequireDocker(t)
	const pass, dbName = "flowpass", "flowdb"
	c, err := tcpostgres.Run(ctx, testsupport.ImageFor("postgres", "postgres:17.11-alpine"),
		tcpostgres.WithDatabase(dbName), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword(pass),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(120*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := c.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatal(err)
	}
	return connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "flow pg", Kind: "postgres", Color: "indigo", Mode: "fields",
		Host: ptr(host), Port: ptr(int(mapped.Num())), Database: ptr(dbName), Username: ptr("postgres"),
		Options: map[string]any{"sslmode": "disable"}, McpReadMode: "deny", McpWriteMode: "deny", McpDdlMode: "deny",
	}, Password: ptr(pass)}
}

func TestPostgresJourney(t *testing.T) {
	flowharness.Complete(t)
	in := startPostgres(t)
	app := flowharness.New(t)
	seed := createConn(t, app, in)
	if st := connect(t, app, seed.ID); st.Status != "connected" {
		t.Fatalf("seed connect = %+v", st)
	}
	execute(t, app, seed.ID,
		"create table users (id serial primary key, name text not null, email text)",
		"insert into users (name, email) values ('ada', 'ada@example.com'), ('bob', 'bob@example.com'), ('cy', null)",
		"create view named_users as select id, name from users")
	if err := app.W.Connections.Delete(bridge.ConnectionsIDArgs{ID: seed.ID}); err != nil {
		t.Fatal(err)
	}
	bad := in
	bad.Port = ptr(1)
	j := &journey{t: t, app: app, input: in, table: "users", readSQL: "select id, name, email from users order by id", rows: 3, bad: &bad}
	j.run()
}

// Contract paste-credentials: tests/ui/connection-paste-credentials.spec.ts "Update credentials with a
// password only reconnects a live connection" answers its connect calls with these states.
func TestPostgresUpdateCredentials(t *testing.T) {
	flowharness.Complete(t)
	in := startPostgres(t)
	app := flowharness.New(t)
	stale := in
	stale.Password = ptr("stale-password")
	conn := createConn(t, app, stale)
	st, err := app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: conn.ID})
	if err == nil && st.Status != "error" {
		t.Fatalf("connect with a stale password = %+v, want an error", st)
	}

	if err := app.W.Connections.UpdateCredentials(bridge.ConnectionsCredentialsArgs{ID: conn.ID, Password: in.Password}); err != nil {
		t.Fatal(err)
	}
	fixed := connect(t, app, conn.ID)
	if fixed.Status != "connected" {
		t.Fatalf("connect after UpdateCredentials = %+v, want connected", fixed)
	}
	app.Contract(t, "paste-credentials", "ConnectionsService.Connect#connected", fixed,
		flowharness.Mask("since", "serverVersion"), flowharness.Omit("caps"))
}
