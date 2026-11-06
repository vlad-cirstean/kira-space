// authmatrix_test.go is the complete (Tier 2) suite's own table for mysql and mariadb — P25 §2.5.
// Run through runFamilyAuthMatrix so both engines get every row, exactly as mysqlfamily_test.go's
// own runFamilySuite already does for the general tier.
package mysqlfamily_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func TestMariaDB_AuthMatrix(t *testing.T) {
	testsupport.RequireMatrix(t)
	f := testsupport.StartMariadb(t)
	runFamilyAuthMatrix(t, "mariadb", f, f.Config, testsupport.RootMariaDSN)
}

func TestMySQL_AuthMatrix(t *testing.T) {
	testsupport.RequireMatrix(t)
	f := testsupport.StartMysql(t)
	runFamilyAuthMatrix(t, "mysql", f, f.Config, testsupport.RootMysqlDSN)
}

func runFamilyAuthMatrix(t *testing.T, kind string, fixture any, cfg model.ResolvedConnectionConfig, rootDSN func(host string, port int, database string) string) {
	t.Helper()

	host := ""
	if cfg.Host != nil {
		host = *cfg.Host
	}
	port := 3306
	if cfg.Port != nil {
		port = *cfg.Port
	}

	root, err := sql.Open("mysql", rootDSN(host, port, ""))
	if err != nil {
		t.Fatalf("root connect: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	const roleName, rolePassword = "p25_leastpriv", "p25_pw"
	mustExecSQL(t, root, fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'", roleName, rolePassword))
	t.Cleanup(func() { mustExecSQL(t, root, "DROP USER IF EXISTS '"+roleName+"'@'%'") })
	mustExecSQL(t, root, "GRANT SELECT ON kira_test.* TO '"+roleName+"'@'%'")
	mustExecSQL(t, root, "FLUSH PRIVILEGES")

	uri := fmt.Sprintf("%s://kira:%s@%s:%d/kira_test", kind, testsupport.FixturePassword, host, port)

	testsupport.RunMatrix(t, kind, fixture, cfg, []testsupport.Case{
		{
			Name:   "kira (scoped), database=kira_test",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig { return c },
			Expect: testsupport.Outcome{Succeed: true},
		},
		{
			Name: "kira, database unset",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Database = nil
				return c
			},
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": ""}},
		},
		{
			Name: "kira, database empty string",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				empty := ""
				c.Database = &empty
				return c
			},
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": ""}},
		},
		{
			// catalog.go's systemSchemas filter excludes
			// information_schema/performance_schema/mysql/sys from Children() unconditionally, for
			// every user — not merely for a scoped one, so "root's tree includes a system schema
			// the scoped user cannot see" does not hold for this adapter. Verified directly (both
			// engines return only kira_test/kira_analytics as root). This case is left as a plain
			// connectivity check instead of asserting a tree shape the adapter deliberately never
			// produces.
			Name: "root, database unset",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp("root"), testsupport.Strp(testsupport.FixturePassword)
				c.Database = nil
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
			Then: []testsupport.Scenario{
				// The cross-database read the scoped role (SELECT on kira_test only) cannot reach —
				// Children(root) already asserts visibility (P25); nothing asserted a read.
				testsupport.ReadFirstPage(testsupport.NodePath(cfg.ID, testsupport.Seg("database", "kira_analytics"), testsupport.Seg("table", "events"))),
			},
		},
		{
			Name: "fresh SELECT-only-on-kira_test user, database unset",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp(roleName), testsupport.Strp(rolePassword)
				c.Database = nil
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
			Then: []testsupport.Scenario{
				{
					Name: "Children(root) is exactly [kira_test]",
					Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
						children, err := a.Children(context.Background(), testsupport.NodePath(cfg.ID), adapters.NewOpCtx("matrix-children"))
						if err != nil {
							t.Fatalf("Children: %v", err)
						}
						names := testsupport.ChildNames(t, children)
						if len(names) != 1 || names[0] != "kira_test" {
							t.Errorf("names = %v, want exactly [kira_test]", names)
						}
					},
				},
				// A read-only grant must still read and filter — nothing proved that before this
				// phase; the matrix's own least-privilege row only ever proved it could connect.
				testsupport.ReadFirstPage(testsupport.NodePath(cfg.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("table", "customers"))),
				testsupport.FilterNarrowsResult(
					testsupport.NodePath(cfg.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("table", "customers")),
					"region_id = 1", 1,
				),
				// errno 1142 ("command denied") is not in mapError's auth switch (errors.go:24-29),
				// so this asserts the *correct* behaviour — a missing GRANT reads as E_QUERY, not
				// E_AUTH — and pins it against a future "helpful" widening of that switch.
				testsupport.MutateIsRefused(model.MutationPlan{
					Path: testsupport.NodePath(cfg.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("table", "regions")),
					Ops: []model.MutationRowOp{{
						Kind: "insert", Values: model.RowValues{{Name: "id", Value: testsupport.Strp("71")}, {Name: "name", Value: testsupport.Strp("nope")}},
					}},
				}, adapters.CodeQuery),
				testsupport.ExecuteIsRefused(
					testsupport.NodePath(cfg.ID, testsupport.Seg("database", "kira_test")),
					[]string{"CREATE TABLE p26_matrix_scratch (id INT)"},
					adapters.CodeQuery,
				),
			},
		},
		{
			Name: "fresh SELECT-only-on-kira_test user, a schema with no grant",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp(roleName), testsupport.Strp(rolePassword)
				c.Database = testsupport.Strp("kira_analytics")
				return c
			},
			Expect: testsupport.Outcome{NotCode: adapters.CodeAuth},
		},
		{
			Name: "kira, uri mode",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Mode = "uri"
				c.URI = testsupport.Strp(uri)
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
		},
		{
			Name: "kira, wrong password",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Password = testsupport.Strp("definitely-wrong")
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "nonexistent user",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp("p25-nonexistent-user"), testsupport.Strp("whatever")
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "kira, no password",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Password = nil
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "kira, database=kira_test, options.sslmode=require",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Options = map[string]any{"sslmode": "require"}
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
		},
	})
}

func mustExecSQL(t *testing.T, db *sql.DB, stmt string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), stmt); err != nil {
		t.Fatalf("exec %q: %v", stmt, err)
	}
}
