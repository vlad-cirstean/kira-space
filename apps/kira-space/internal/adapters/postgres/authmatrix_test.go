// authmatrix_test.go is the complete suite's own table for postgres. Row 6
// generalizes across every adapter's own matrix: a permission failure and an authentication
// failure must get different codes.
package postgres_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func TestPostgres_AuthMatrix(t *testing.T) {
	testsupport.RequireMatrix(t)
	f := testsupport.StartPostgres(t)

	// f.URI is already scoped to kira_test (postgres.ts's own superuser connection string), so one
	// side connection covers both the role/database DDL below and the app-schema grants.
	side, err := pgx.Connect(context.Background(), f.URI)
	if err != nil {
		t.Fatalf("side connect: %v", err)
	}
	// Registered first, so its LIFO position is last: every DROP cleanup below must run against a
	// still-open connection.
	t.Cleanup(func() { _ = side.Close(context.Background()) })

	const roleName, rolePassword = "p25_leastpriv", "p25_pw"
	mustExec(t, side, fmt.Sprintf(`CREATE ROLE %s LOGIN PASSWORD '%s'`, roleName, rolePassword))
	t.Cleanup(func() {
		// DROP ROLE fails while any privilege is still granted to it, in this database or any
		// other — both the schema/table grants below and the database-level CONNECT grant have to
		// go first.
		mustExec(t, side, `DROP OWNED BY `+roleName)
		mustExec(t, side, `REVOKE CONNECT ON DATABASE kira_test FROM `+roleName)
		mustExec(t, side, `DROP ROLE IF EXISTS `+roleName)
	})
	mustExec(t, side, `GRANT CONNECT ON DATABASE kira_test TO `+roleName)
	mustExec(t, side, `GRANT USAGE ON SCHEMA app TO `+roleName)
	mustExec(t, side, `GRANT SELECT ON ALL TABLES IN SCHEMA app TO `+roleName)

	// A throwaway database the least-privilege role has no CONNECT on at all — row 6's whole
	// point, isolated from the shared "postgres" maintenance database so this never touches a
	// privilege every other test in this package relies on.
	const noAccessDB = "p25_no_access"
	mustExec(t, side, `DROP DATABASE IF EXISTS `+noAccessDB)
	mustExec(t, side, `CREATE DATABASE `+noAccessDB)
	t.Cleanup(func() { mustExec(t, side, `DROP DATABASE IF EXISTS `+noAccessDB) })
	mustExec(t, side, `REVOKE CONNECT ON DATABASE `+noAccessDB+` FROM PUBLIC`)

	uri := fmt.Sprintf("postgres://%s:%s@%s/kira_test", roleName, rolePassword, hostPort(f.Config))

	testsupport.RunMatrix(t, "postgres", f, f.Config, []testsupport.Case{
		{
			Name:   "superuser, database=kira_test",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig { return c },
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": "kira_test"}},
		},
		{
			Name: "superuser, database unset",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Database = nil
				return c
			},
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": "postgres"}},
		},
		{
			Name: "least-privilege role, database=kira_test",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp(roleName), testsupport.Strp(rolePassword)
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
			Then: []testsupport.Scenario{
				{
					Name: "Children(root) is non-empty",
					Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
						children, err := a.Children(context.Background(), testsupport.NodePath(cfg.ID), adapters.NewOpCtx("matrix-children"))
						if err != nil {
							t.Fatalf("Children: %v", err)
						}
						if len(children.Nodes) == 0 {
							t.Error("Children(root) is empty, want at least kira_test")
						}
					},
				},
				// The role has SELECT via GRANT SELECT ON ALL TABLES IN SCHEMA app; reads must work
				// under it, and nothing proved that before this phase.
				testsupport.ReadFirstPage(testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("schema", "app"), testsupport.Seg("table", "customers"))),
				testsupport.FilterNarrowsResult(
					testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("schema", "app"), testsupport.Seg("table", "customers")),
					"region_id = 1", 1,
				),
				testsupport.CountMatchesRead(testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("schema", "app"), testsupport.Seg("table", "customers"))),
				// SQLSTATE 42501 (insufficient_privilege) is not in mapError's auth switch
				// (errors.go:32-37) — the single most valuable new assertion: it pins
				// a missing GRANT does *not* read as a wrong password, asserted for the first time
				// past Connect().
				testsupport.MutateIsRefused(model.MutationPlan{
					Path: testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("schema", "app"), testsupport.Seg("table", "regions")),
					Ops: []model.MutationRowOp{{
						Kind: "insert", Values: model.RowValues{{Name: "id", Value: testsupport.Strp("91")}, {Name: "name", Value: testsupport.Strp("nope")}},
					}},
				}, adapters.CodeQuery),
				testsupport.ExecuteIsRefused(
					testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "kira_test")),
					[]string{"CREATE TABLE app.p26_matrix_scratch (id int)"},
					adapters.CodeQuery,
				),
			},
		},
		{
			Name: "least-privilege role, database unset",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp(roleName), testsupport.Strp(rolePassword)
				c.Database = nil
				return c
			},
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": "postgres"}},
		},
		{
			Name: "least-privilege role, database empty string",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp(roleName), testsupport.Strp(rolePassword)
				empty := ""
				c.Database = &empty
				return c
			},
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": "postgres"}},
		},
		{
			Name: "least-privilege role, a database it has no CONNECT on",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp(roleName), testsupport.Strp(rolePassword)
				c.Database = testsupport.Strp(noAccessDB)
				return c
			},
			Expect: testsupport.Outcome{NotCode: adapters.CodeAuth},
		},
		{
			Name: "least-privilege role, uri mode",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Mode = "uri"
				c.URI = testsupport.Strp(uri)
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
		},
		{
			Name: "superuser, wrong password",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Password = testsupport.Strp("definitely-wrong")
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "nonexistent role",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp("p25-nonexistent-role"), testsupport.Strp("whatever")
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "least-privilege role, no password given",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username = testsupport.Strp(roleName)
				c.Password = nil
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
	})
}

func mustExec(t *testing.T, conn *pgx.Conn, sql string) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func hostPort(cfg model.ResolvedConnectionConfig) string {
	host := "localhost"
	if cfg.Host != nil {
		host = *cfg.Host
	}
	port := 5432
	if cfg.Port != nil {
		port = *cfg.Port
	}
	return fmt.Sprintf("%s:%d", host, port)
}
