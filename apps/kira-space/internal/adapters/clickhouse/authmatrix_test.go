// authmatrix_test.go is the complete (Tier 2) suite's own table for clickhouse — P25 §2.6.
// ClickHouse already carries the privilege spectrum this phase wants (testsupport/clickhouse.go
// seeds kira_admin, kira and kira_ro), so this table has the least to add of any adapter.
package clickhouse_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
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
