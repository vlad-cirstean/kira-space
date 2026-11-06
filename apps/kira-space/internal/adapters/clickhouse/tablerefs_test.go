package clickhouse_test

import (
	"context"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
)

// P532 conformance: the live function probe behind the table allow list.
func TestClickHouse_TableProbe(t *testing.T) {
	fixture := testsupport.StartClickHouse(t)
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), fixture.Config, adapters.NewOpCtx("connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })

	ref, ok := a.(adapters.TableReferencer)
	if !ok {
		t.Fatal("clickhouse adapter is not a TableReferencer")
	}
	path := testsupport.NodePath(fixture.Config.ID)
	probe := func(sql string) string {
		t.Helper()
		parsed := ref.ParseTables(sql, path)
		if parsed.Refusal != "" {
			return parsed.Refusal
		}
		if !parsed.NeedsProbe() {
			return ""
		}
		_, refusal, err := ref.ResolveTables(context.Background(), parsed, path, adapters.NewOpCtx("tables"))
		if err != nil {
			t.Fatalf("ResolveTables(%q): %v", sql, err)
		}
		return refusal
	}

	testsupport.AdminStatements(t, fixture, `CREATE FUNCTION IF NOT EXISTS p532_fn AS (x) -> x + 1`)
	t.Cleanup(func() { testsupport.AdminStatements(t, fixture, `DROP FUNCTION IF EXISTS p532_fn`) })
	if probe(`SELECT p532_fn(1)`) == "" {
		t.Error("a user-defined function was not refused")
	}
	if r := probe(`SELECT lower('A'), count() FROM t`); r != "" {
		t.Errorf("a built-in function was refused: %s", r)
	}
	testsupport.AdminStatements(t, fixture, `CREATE FUNCTION IF NOT EXISTS p532_late AS (x) -> x * 2`)
	t.Cleanup(func() { testsupport.AdminStatements(t, fixture, `DROP FUNCTION IF EXISTS p532_late`) })
	if probe(`SELECT p532_late(1)`) == "" {
		t.Error("a function created after startup was not refused")
	}
}
