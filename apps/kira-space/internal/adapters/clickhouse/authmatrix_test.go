// authmatrix_test.go is the complete (Tier 2) suite's own table for clickhouse — P25 §2.6.
// ClickHouse already carries the privilege spectrum this phase wants (testsupport/clickhouse.go
// seeds kira_admin, kira and kira_ro), so this table has the least to add of any adapter.
package clickhouse_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func TestClickHouse_AuthMatrix(t *testing.T) {
	testsupport.RequireMatrix(t)
	f := testsupport.StartClickHouse(t)
	testsupport.ClickHouseRun(t, f)

	database := *f.Config.Database
	uri := fmt.Sprintf("clickhouse://kira:%s@%s:%d/%s", testsupport.FixturePassword, *f.Config.Host, *f.Config.Port, database)

	testsupport.RunMatrix(t, "clickhouse", f, f.Config, []testsupport.Case{
		{
			Name:   "kira, database=kira_test",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig { return c },
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": database}},
		},
		{
			Name: "kira, database unset",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Database = nil
				return c
			},
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": "default"}},
		},
		{
			Name: "kira_admin, database unset",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username = testsupport.Strp("kira_admin")
				c.Password = testsupport.Strp(testsupport.FixturePassword)
				c.Database = nil
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
			Then: []testsupport.Scenario{
				{
					Name: "Children includes system",
					Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
						children, err := a.Children(context.Background(), testsupport.NodePath(cfg.ID), opCtx(t, "matrix-children"))
						if err != nil {
							t.Fatalf("Children: %v", err)
						}
						names := testsupport.ChildNames(t, children)
						if !testsupport.ContainsName(names, "system") {
							t.Errorf("names = %v, want to contain %q", names, "system")
						}
					},
				},
				// The posture half of the cross product: kira_admin reaches what kira_ro's own
				// system-table SELECT grant cannot (see the pinned read below).
				testsupport.ReadFirstPage(testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "system"), testsupport.Seg("table", "databases"))),
			},
		},
		{
			Name: "kira_ro, database=kira_test",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username = testsupport.Strp("kira_ro")
				c.Password = testsupport.Strp(testsupport.FixturePassword)
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
			Then: []testsupport.Scenario{
				testsupport.ReadFirstPage(testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("table", "customers"))),
				// The pin for clickhouse, found running this exact scenario against a real
				// container: ClickHouse's readonlyCode/tableIsReadOnly (164/242 -> E_UNSUPPORTED,
				// errors.go:60) was expected here, on the theory that kira_ro's missing INSERT
				// grant would surface as one of those two. Measured instead: a missing INSERT
				// grant is a plain authorization refusal —
				// "Not enough privileges ... (ACCESS_DENIED)", numeric code 497 — which
				// errors.go:64 maps to E_AUTH, the same code a wrong password produces. So this
				// is not a "correct mapping" case; it is a second instance of the very conflation
				// already found in kira's own system-table read, now pinned rather than asserted
				// as correct. Not fixed here: a correctly-authenticated, correctly-passworded
				// connection with a missing GRANT must not be told its password is wrong, and
				// nothing exercised that past Connect() before this.
				testsupport.MutateIsRefused(model.MutationPlan{
					Path: testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("table", "regions")),
					Ops: []model.MutationRowOp{{
						Kind: "insert", Values: model.RowValues{{Name: "id", Value: testsupport.Strp("61")}, {Name: "name", Value: testsupport.Strp("nope")}},
					}},
				}, adapters.CodeAuth),
				testsupport.ExecuteIsRefused(
					testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "kira_test")),
					[]string{"CREATE TABLE kira_test.p26_matrix_scratch (id UInt32) ENGINE = MergeTree ORDER BY id"},
					adapters.CodeAuth,
				),
				// P602: the lane's per-request readonly=2 for a grants-only user. The probe table gets an
				// INSERT grant so the refusal is the read-only session, not a missing privilege.
				testsupport.ReadLaneRefusesWrite(testsupport.ReadLaneCase{
					Path: testsupport.NodePath(f.Config.ID),
					Admin: chAdmin(f,
						"CREATE TABLE kira_test.p602_lane_log (id UInt8) ENGINE = Memory; GRANT INSERT ON kira_test.p602_lane_log TO kira_ro",
						"DROP TABLE IF EXISTS kira_test.p602_lane_log"),
					Write: "INSERT INTO kira_test.p602_lane_log VALUES (1)", WantErr: "readonly mode",
					Unchanged: func(t *testing.T, main adapters.Adapter) {
						t.Helper()
						if got := testsupport.FirstCell(t, main, testsupport.NodePath(f.Config.ID), "SELECT count() FROM kira_test.p602_lane_log"); got != "0" {
							t.Errorf("p602_lane_log rows = %s, want 0", got)
						}
					},
				}),
				testsupport.ReadLaneReads(testsupport.NodePath(f.Config.ID), "SELECT count() FROM kira_test.regions WHERE id > 0", "2"),
				testsupport.StatementTimeout(testsupport.StatementTimeoutCase{
					Name: "read lane", Path: testsupport.NodePath(f.Config.ID), Lane: true,
					Slow: "SELECT sum(cityHash64(number)) FROM numbers(1000000000000)", WantCode: "TIMEOUT_EXCEEDED",
					Baseline: "SELECT getSetting('max_execution_time')", BaselineWant: "0",
					Lifted: "SELECT sleepEachRow(1) FROM numbers(2) SETTINGS max_block_size = 1",
				}),
				chExplainPlan(f),
			},
		},
		{
			// P604: a readonly=1 profile refuses every settings change. Connect reads the profile's
			// readonly and the adapter sends no settings for it (docs/ARCHITECTURE.md ClickHouse).
			Name: "kira_ro1 (profile readonly=1), database=kira_test",
			Principal: &testsupport.Principal{Name: "readonly=1 profile user", Setup: func(t *testing.T, fx any) {
				t.Helper()
				f := fx.(*testsupport.ClickHouseFixture)
				testsupport.AdminStatements(t, f, `
					CREATE USER IF NOT EXISTS kira_ro1 IDENTIFIED WITH plaintext_password BY 'kira_ro1_pw' SETTINGS readonly = 1;
					GRANT SELECT ON kira_test.* TO kira_ro1;
					GRANT SELECT ON system.* TO kira_ro1;`)
				t.Cleanup(func() { testsupport.AdminStatements(t, f, "DROP USER IF EXISTS kira_ro1") })
			}},
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp("kira_ro1"), testsupport.Strp("kira_ro1_pw")
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
			Then: []testsupport.Scenario{
				testsupport.ReadFirstPage(testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("table", "customers"))),
				testsupport.CountMatchesRead(testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("table", "customers"))),
				{
					Name: "Children contains kira_test",
					Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
						children, err := a.Children(context.Background(), testsupport.NodePath(cfg.ID), opCtx(t, "matrix-ro1-children"))
						if err != nil {
							t.Fatalf("Children: %v", err)
						}
						if names := testsupport.ChildNames(t, children); !testsupport.ContainsName(names, "kira_test") {
							t.Errorf("names = %v, want to contain %q", names, "kira_test")
						}
					},
				},
				testsupport.ReadLaneReads(testsupport.NodePath(f.Config.ID), "SELECT count() FROM kira_test.regions WHERE id > 0", "2"),
				testsupport.ReadLaneRefusesWrite(testsupport.ReadLaneCase{
					Path: testsupport.NodePath(f.Config.ID),
					Admin: chAdmin(f,
						"CREATE TABLE kira_test.p604_ro1_log (id UInt8) ENGINE = Memory; GRANT INSERT ON kira_test.p604_ro1_log TO kira_ro1",
						"DROP TABLE IF EXISTS kira_test.p604_ro1_log"),
					Write: "INSERT INTO kira_test.p604_ro1_log VALUES (1)", WantErr: "readonly mode",
					Unchanged: func(t *testing.T, main adapters.Adapter) {
						t.Helper()
						if got := testsupport.FirstCell(t, main, testsupport.NodePath(f.Config.ID), "SELECT count() FROM kira_test.p604_ro1_log"); got != "0" {
							t.Errorf("p604_ro1_log rows = %s, want 0", got)
						}
					},
				}),
				// KILL QUERY works for the locked profile.
				testsupport.LaneCancel(testsupport.NodePath(f.Config.ID), "SELECT sum(cityHash64(number)) FROM numbers(1000000000000)"),
				chExplainPlan(f),
				{
					Name: "statement timeout degrades to the app timer",
					Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
						path := testsupport.NodePath(cfg.ID)
						pages, err := a.Execute(context.Background(), model.ConsoleRequest{
							Path: path, Statements: []string{"SELECT count() FROM kira_test.regions"}, StatementTimeout: 5 * time.Second,
						}, opCtx(t, "matrix-ro1-timeout-exec"))
						if err != nil {
							t.Fatalf("Execute with a statement timeout: %v", err)
						}
						p, ok := pages[0].(page.TabularPage)
						if !ok || p.RowCount == 0 || page.CellText(p.Chunks[0], 0) != "2" {
							t.Errorf("Execute result = %#v, want one row with 2", pages[0])
						}
						if _, err := a.Read(context.Background(), adapters.ReadRequest{
							Path:     testsupport.NodePath(cfg.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("table", "customers")),
							PageSize: 10, Cursor: model.PageCursor{Mode: "offset"}, StatementTimeout: 5 * time.Second,
						}, opCtx(t, "matrix-ro1-timeout-read")); err != nil {
							t.Fatalf("Read with a statement timeout: %v", err)
						}
						// The readonly=1 profile cannot carry the engine timeout, so the app timer must stop a slow read.
						t.Cleanup(func() { testsupport.AdminStatements(t, f, "KILL QUERY WHERE user = 'kira_ro1' SYNC") })
						const limit = time.Second
						ctx, cancel := context.WithTimeout(context.Background(), limit+10*time.Second)
						defer cancel()
						op := opCtx(t, "matrix-ro1-timeout-slow")
						start := time.Now()
						_, err = a.Execute(ctx, model.ConsoleRequest{
							Path: path, Statements: []string{"SELECT sum(cityHash64(number)) FROM numbers(1000000000000)"}, StatementTimeout: limit,
						}, op)
						var detail *adapters.TimeoutDetail
						if !errors.As(err, &detail) {
							t.Fatalf("slow Execute = %v, want a statement timeout", err)
						}
						if cause := detail.Cause; cause == nil || !strings.Contains(cause.Error(), "app timer") {
							t.Errorf("timeout cause = %v, want it to contain %q", detail.Cause, "app timer")
						}
						if took := time.Since(start); took > limit+3*time.Second {
							t.Errorf("slow Execute took %s, want about %s", took, limit)
						}
						waitForProcessesClean(t, connectedAdapter(t, f.Config), "kira-"+op.OpID+"-0", 5*time.Second)
					},
				},
				testsupport.ReadStatementTimeout(testsupport.ReadStatementTimeoutCase{
					Name: "readonly=1 app timer", WantCode: "app timer", Lane: false,
					Path: testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("view", "p607_ro1_slow")),
					Fast: testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("table", "customers")),
					Admin: chAdmin(f,
						"CREATE VIEW kira_test.p607_ro1_slow AS SELECT sum(cityHash64(number)) AS s FROM numbers(1000000000000)",
						"KILL QUERY WHERE user = 'kira_ro1' SYNC; DROP VIEW IF EXISTS kira_test.p607_ro1_slow"),
				}),
				chCapKill(f),
				{
					Name: "UInt64 max decodes exactly",
					Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
						testsupport.AdminStatements(t, f, `
							CREATE TABLE kira_test.p604_u64 (id UInt64) ENGINE = MergeTree ORDER BY id;
							INSERT INTO kira_test.p604_u64 VALUES (18446744073709551615)`)
						t.Cleanup(func() { testsupport.AdminStatements(t, f, "DROP TABLE IF EXISTS kira_test.p604_u64") })
						path := testsupport.NodePath(cfg.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("table", "p604_u64"))
						if _, err := a.Describe(context.Background(), path, opCtx(t, "matrix-ro1-u64-describe")); err != nil {
							t.Fatalf("Describe: %v", err)
						}
						first, err := a.Read(context.Background(), adapters.ReadRequest{
							Path: path, PageSize: 10, Cursor: model.PageCursor{Mode: "offset"},
						}, opCtx(t, "matrix-ro1-u64-read"))
						if err != nil {
							t.Fatalf("Read: %v", err)
						}
						tp, ok := first.(page.TabularPage)
						if !ok || tp.RowCount != 1 {
							t.Fatalf("Read page = %#v, want one row", first)
						}
						if got := page.CellText(tp.Chunks[0], 0); got != "18446744073709551615" {
							t.Errorf("UInt64 max read back as %q, want 18446744073709551615", got)
						}
						result, err := a.Count(context.Background(), adapters.CountRequest{Path: path}, opCtx(t, "matrix-ro1-u64-count"))
						if err != nil {
							t.Fatalf("Count: %v", err)
						}
						if result.Value != 1 {
							t.Errorf("Count = %d, want 1", result.Value)
						}
					},
				},
			},
		},
		{
			// P604: readonly=2 profile: settings travel, the per-request readonly never does.
			Name: "kira_ro2 (profile readonly=2), database=kira_test",
			Principal: &testsupport.Principal{Name: "readonly=2 profile user", Setup: func(t *testing.T, fx any) {
				t.Helper()
				f := fx.(*testsupport.ClickHouseFixture)
				testsupport.AdminStatements(t, f, `
					CREATE USER IF NOT EXISTS kira_ro2 IDENTIFIED WITH plaintext_password BY 'kira_ro2_pw' SETTINGS readonly = 2;
					GRANT SELECT ON kira_test.* TO kira_ro2;
					GRANT SELECT ON system.* TO kira_ro2;`)
				t.Cleanup(func() { testsupport.AdminStatements(t, f, "DROP USER IF EXISTS kira_ro2") })
			}},
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp("kira_ro2"), testsupport.Strp("kira_ro2_pw")
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
			Then: []testsupport.Scenario{
				testsupport.ReadFirstPage(testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("table", "customers"))),
				testsupport.ReadLaneReads(testsupport.NodePath(f.Config.ID), "SELECT count() FROM kira_test.regions WHERE id > 0", "2"),
				testsupport.ReadLaneRefusesWrite(testsupport.ReadLaneCase{
					Path: testsupport.NodePath(f.Config.ID),
					Admin: chAdmin(f,
						"CREATE TABLE kira_test.p604_ro2_log (id UInt8) ENGINE = Memory; GRANT INSERT ON kira_test.p604_ro2_log TO kira_ro2",
						"DROP TABLE IF EXISTS kira_test.p604_ro2_log"),
					Write: "INSERT INTO kira_test.p604_ro2_log VALUES (1)", WantErr: "readonly mode",
					Unchanged: func(t *testing.T, main adapters.Adapter) {
						t.Helper()
						if got := testsupport.FirstCell(t, main, testsupport.NodePath(f.Config.ID), "SELECT count() FROM kira_test.p604_ro2_log"); got != "0" {
							t.Errorf("p604_ro2_log rows = %s, want 0", got)
						}
					},
				}),
				testsupport.StatementTimeout(testsupport.StatementTimeoutCase{
					Name: "read lane", Path: testsupport.NodePath(f.Config.ID), Lane: true,
					Slow: "SELECT sum(cityHash64(number)) FROM numbers(1000000000000)", WantCode: "TIMEOUT_EXCEEDED",
					Baseline: "SELECT getSetting('max_execution_time')", BaselineWant: "0",
					Lifted: "SELECT sleepEachRow(1) FROM numbers(2) SETTINGS max_block_size = 1",
				}),
			},
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
			Name: "username unset, password set",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username = nil
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "username and password both unset",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username = nil
				c.Password = nil
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "uri mode",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Mode = "uri"
				c.URI = testsupport.Strp(uri)
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
		},
		{
			Name: "options.sslmode garbage",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Options = map[string]any{"sslmode": "garbage"}
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeConnect},
		},
	})
}
