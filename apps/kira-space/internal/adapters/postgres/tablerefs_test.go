package postgres_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
)

// P532 conformance: the live catalog probe behind the table allow list.
func TestPostgres_TableProbe(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	side, err := pgx.Connect(context.Background(), fixture.URI)
	if err != nil {
		t.Fatalf("side connect: %v", err)
	}
	defer side.Close(context.Background())
	// The seed drops public, so "$user" is the one schema on the default search_path to resolve through.
	var user string
	if err := side.QueryRow(context.Background(), `SELECT current_user`).Scan(&user); err != nil {
		t.Fatalf("current_user: %v", err)
	}
	qu := pgx.Identifier{user}.Sanitize()
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ` + qu,
		`CREATE TABLE IF NOT EXISTS ` + qu + `.p532_pub (id int)`,
		`CREATE TABLE IF NOT EXISTS app.p532_hidden (id int)`,
		`CREATE OR REPLACE FUNCTION ` + qu + `.p532_fn() RETURNS int LANGUAGE sql AS 'SELECT 1'`,
		`CREATE OR REPLACE FUNCTION ` + qu + `.format(int) RETURNS text LANGUAGE sql AS $$SELECT 'x'$$`,
		`CREATE OR REPLACE FUNCTION ` + qu + `.p532_eq(int, int) RETURNS bool LANGUAGE sql AS 'SELECT true'`,
		`CREATE OPERATOR ` + qu + `.=== (LEFTARG = int, RIGHTARG = int, FUNCTION = ` + qu + `.p532_eq)`,
	} {
		if _, err := side.Exec(context.Background(), stmt); err != nil {
			t.Fatalf("setup %q: %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		_, _ = side.Exec(context.Background(), `DROP SCHEMA `+qu+` CASCADE`)
		_, _ = side.Exec(context.Background(), `DROP TABLE IF EXISTS app.p532_hidden`)
	})
	ref, ok := a.(adapters.TableReferencer)
	if !ok {
		t.Fatal("postgres adapter is not a TableReferencer")
	}
	path := nodePath(fixture, seg("database", "kira_test"))
	probe := func(sql string) ([]tableref.Ref, string) {
		t.Helper()
		parsed := ref.ParseTables(sql, path)
		if parsed.Refusal != "" {
			return nil, parsed.Refusal
		}
		if !parsed.NeedsProbe() {
			return parsed.Refs, ""
		}
		refs, refusal, err := ref.ResolveTables(context.Background(), parsed, path, adapters.NewOpCtx("tables"))
		if err != nil {
			t.Fatalf("ResolveTables(%q): %v", sql, err)
		}
		return append(parsed.Refs, refs...), refusal
	}

	refs, refusal := probe(`SELECT * FROM p532_pub`)
	if refusal != "" || len(refs) != 1 || refs[0].Schema != user || refs[0].Name != "p532_pub" {
		t.Errorf("unqualified on search_path = %v, %q; want %s.p532_pub", refs, refusal, user)
	}
	// A name in a schema off the search_path matches no listed table, so the gate refuses it.
	refs, _ = probe(`SELECT * FROM p532_hidden`)
	if len(refs) != 1 || refs[0].Schema == "app" {
		t.Errorf("off-path name = %v; want it not attributed to app", refs)
	}
	if _, refusal := probe(`SELECT p532_fn()`); refusal == "" {
		t.Error("a user-defined function was not refused")
	}
	if _, refusal := probe(`SELECT lower('A') FROM p532_pub`); refusal != "" {
		t.Errorf("a built-in function was refused: %s", refusal)
	}
	// A user overload of a built-in name is refused, and the refusal names the way to the built-in.
	if _, refusal := probe(`SELECT format('%s', 1) FROM p532_pub`); !strings.Contains(refusal, "function format") || !strings.Contains(refusal, "pg_catalog.format") {
		t.Errorf("a user overload of format = %q; want a refusal pointing at pg_catalog.format", refusal)
	}
	if _, refusal := probe(`SELECT pg_catalog.format('%s', 1) FROM p532_pub`); refusal != "" {
		t.Errorf("pg_catalog.format was refused: %s", refusal)
	}
	// A user operator runs its backing function, so it is refused; a built-in operator is not.
	if _, refusal := probe(`SELECT 1 === 2`); refusal != "operator ===" {
		t.Errorf("a user-defined operator = %q; want operator ===", refusal)
	}
	if _, refusal := probe(`SELECT 1 = 2 FROM p532_pub`); refusal != "" {
		t.Errorf("a built-in operator was refused: %s", refusal)
	}
	// A function created after the first probe is still seen: no cached name list.
	if _, err := side.Exec(context.Background(), `CREATE OR REPLACE FUNCTION `+qu+`.p532_late() RETURNS int LANGUAGE sql AS 'SELECT 2'`); err != nil {
		t.Fatalf("create late function: %v", err)
	}
	if _, refusal := probe(`SELECT p532_late()`); refusal == "" {
		t.Error("a function created after startup was not refused")
	}
}
