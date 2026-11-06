// authmatrix_test.go is the complete suite's own table for sqlite. SQLite has
// no credentials at all (Username/Password are never read), so its permutation axis is path
// handling, not auth. File-permission cases are declined: this sandbox runs as root,
// which bypasses file modes entirely, so no claim is made either way.
package sqlite_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func TestSqlite_AuthMatrix(t *testing.T) {
	testsupport.RequireMatrix(t)
	f := testsupport.StartSqlite(t)

	missing := filepath.Join(f.Dir, "p25-missing.sqlite")
	garbage := filepath.Join(f.Dir, "p25-garbage.sqlite")
	if err := os.WriteFile(garbage, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatalf("write garbage file: %v", err)
	}
	dir := filepath.Join(f.Dir, "p25-a-directory")
	// MkdirAll, not Mkdir: f.Dir is a memoized fixture directory, so a second `go test -count=2`
	// run against the same directory would otherwise fail with "file exists".
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	testsupport.RunMatrix(t, "sqlite", f, f.Config, []testsupport.Case{
		{
			Name:   "the seeded file, read-write",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig { return c },
			Expect: testsupport.Outcome{Succeed: true},
		},
		{
			Name: "the seeded file, read-only",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.ReadOnly = true
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
			Then: []testsupport.Scenario{
				// A read-only connection still reads.
				testsupport.ReadFirstPage(testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "main"), testsupport.Seg("table", "customers"))),
				// mutate.go:64's own adapters.AssertWritable(readOnly) guard refuses before any
				// statement reaches the driver — the app-layer check, not sqlite/errors.go:44-45's
				// driver-level SQLITE_READONLY (code 8) mapping; the sibling ExecuteIsRefused below
				// is the one that reaches the driver's own readOnly case.
				testsupport.MutateIsRefused(model.MutationPlan{
					Path: testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "main"), testsupport.Seg("table", "regions")),
					Ops: []model.MutationRowOp{{
						Kind: "insert", Values: model.RowValues{{Name: "id", Value: testsupport.Strp("81")}, {Name: "name", Value: testsupport.Strp("nope")}},
					}},
				}, adapters.CodeUnsupported),
				testsupport.ExecuteIsRefused(
					testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "main")),
					[]string{"CREATE TABLE p26_ro_matrix_scratch (id INTEGER)"},
					adapters.CodeUnsupported,
				),
			},
		},
		{
			Name: "a missing file",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Database = testsupport.Strp(missing)
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeNotFound},
		},
		{
			Name: "a garbage non-database file",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Database = testsupport.Strp(garbage)
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeConnect},
		},
		{
			Name: "a directory",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Database = testsupport.Strp(dir)
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeConnect},
		},
		{
			Name: "empty/unset database field",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				empty := ""
				c.Database = &empty
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeConnect},
		},
		{
			// client.go's own comment: net/url.Parse's Path comes back one leading slash short of
			// the real absolute path, so the URI needs four slashes after the scheme, not three.
			Name: "uri mode, sqlite:////abs/path",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Mode = "uri"
				c.URI = testsupport.Strp("sqlite:///" + f.Path)
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
		},
		{
			Name: "uri mode, no path",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Mode = "uri"
				c.URI = testsupport.Strp("sqlite://")
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeConnect},
		},
		{
			// SQLite is the only engine where "two connections, one file" is an ordinary user
			// situation (two windows on the same database) — and the only adapter for which the DDL
			// catalog round trip already proves is also a *file* round trip: a second, wholly
			// independent connection must see what the first one's own Execute created.
			Name:   "the seeded file, read-write (two connections)",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig { return c },
			Expect: testsupport.Outcome{Succeed: true},
			Then: []testsupport.Scenario{
				{
					Name: "a second connection sees what the first's Execute created",
					Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
						ctx := context.Background()
						databasePath := testsupport.NodePath(cfg.ID, testsupport.Seg("database", "main"))
						t.Cleanup(func() {
							_, _ = a.Execute(context.Background(), model.ConsoleRequest{
								Path: databasePath, Statements: []string{"DROP TABLE IF EXISTS p26_two_conn"},
							}, adapters.NewOpCtx("matrix-two-conn-cleanup"))
						})
						if _, err := a.Execute(ctx, model.ConsoleRequest{
							Path: databasePath, Statements: []string{"CREATE TABLE p26_two_conn (id INTEGER PRIMARY KEY)"},
						}, adapters.NewOpCtx("matrix-two-conn-create")); err != nil {
							t.Fatalf("Execute(CREATE TABLE): %v", err)
						}

						second, err := adapters.CreateAdapter("sqlite", adapters.Deps{Log: func(level, message string) {}})
						if err != nil {
							t.Fatalf("CreateAdapter: %v", err)
						}
						if _, err := second.Connect(ctx, cfg, adapters.NewOpCtx("matrix-two-conn-connect")); err != nil {
							t.Fatalf("second Connect: %v", err)
						}
						defer second.Disconnect(context.Background())

						children, err := second.Children(ctx, databasePath, adapters.NewOpCtx("matrix-two-conn-children"))
						if err != nil {
							t.Fatalf("second Children: %v", err)
						}
						if !testsupport.ContainsName(testsupport.ChildNames(t, children), "p26_two_conn") {
							t.Errorf("second connection's Children(main) = %v, want p26_two_conn present", testsupport.ChildNames(t, children))
						}
					},
				},
			},
		},
	})
}
