// authmatrix_test.go is the complete (Tier 2) suite's own table for redis — P25 §2.6. Its axes
// are ACL shape and db index; it has no "database name" at all.
package redis_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func TestRedis_AuthMatrix(t *testing.T) {
	testsupport.RequireMatrix(t)
	f := testsupport.StartRedis(t)

	uri := fmt.Sprintf("redis://:%s@%s:%d/1", testsupport.RedisPassword, f.Host, f.Port)

	testsupport.RunMatrix(t, "redis", f, f.Config, []testsupport.Case{
		{
			Name:   "no username, requirepass password, database=0",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig { return c },
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": "db0"}},
		},
		{
			Name: "no username, requirepass password, database unset",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Database = nil
				return c
			},
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": "db0"}},
		},
		{
			Name: "no username, requirepass password, database=1",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Database = testsupport.Strp("1")
				return c
			},
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": "db1"}},
		},
		{
			Name:      "ACL user ~* +@all -@dangerous",
			Principal: testsupport.RedisAclUser("p25_matrix_all_no_dangerous", "p25_pw", "~*", "+@all", "-@dangerous"),
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp("p25_matrix_all_no_dangerous"), testsupport.Strp("p25_pw")
				return c
			},
			// The degraded "Redis unknown" ServerVersion string is asserted by the Tier-1
			// regression test (TestRedis_Connect_LeastPrivilegeAclUser) rather than duplicated
			// here — this row's own job is the matrix table's completeness, not a second copy of
			// that assertion.
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": "db0"}},
			Then: []testsupport.Scenario{
				// P25 §1.3's headline "most commonly recommended application ACL" — P25 proved only
				// that it can connect. A read and a write must both work under it too.
				testsupport.ReadFirstPage(testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "db0"), testsupport.Seg("key", "counter"))),
				testsupport.MutateSucceeds(model.MutationPlan{
					Path: testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "db0")),
					Ops: []model.MutationRowOp{{
						Kind: "insert",
						Values: model.RowValues{
							{Name: "_key", Value: testsupport.Strp("p26:matrix:acl-all-no-dangerous")},
							{Name: "$value", Value: testsupport.Strp("hello")},
						},
					}},
				}, 1),
				// The write above left its key behind in the shared container —
				// mongo's own equivalent write scenario cleans up after itself
				// (mongo/authmatrix_test.go's "the authSource-fixed connection can actually
				// write"); mirrored here as a plain delete rather than folded into the write
				// scenario's own Run, since MutateSucceeds is the shared constructor (also backing
				// Tier 1 via RunScenarios) and this key's cleanup is redis-specific.
				{
					Name: "cleanup: delete the key the write above left behind",
					Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
						plan := model.MutationPlan{
							Path: testsupport.NodePath(cfg.ID, testsupport.Seg("database", "db0")),
							Ops: []model.MutationRowOp{{
								Kind: "delete",
								Key:  model.RowValues{{Name: "_key", Value: testsupport.Strp("p26:matrix:acl-all-no-dangerous")}},
							}},
						}
						if _, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("matrix-acl-all-no-dangerous-cleanup")); err != nil {
							t.Errorf("cleanup delete: %v", err)
						}
					},
				},
			},
		},
		{
			// Deviation from the plan's own row: it asserts "~* +@read" connects post-fix. Verified
			// against a real container that it still does not — PING (the "left fatal,
			// deliberately") is in Redis's @connection/@fast categories, not @read, so a bare
			// +@read grant was never enough even before this phase, matching the transcript
			// for "~* +@read +client|setname" (still NOPERM on ping there too, pre-fix). Dropping
			// ClientName only removed the *first* blocker for a read-only ACL; PING remains one.
			// This row now asserts the real, current behaviour rather than the plan's claim.
			Name:      "ACL user ~* +@read (still fails: PING is not in @read)",
			Principal: testsupport.RedisAclUser("p25_matrix_read_only", "p25_pw", "~*", "+@read"),
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp("p25_matrix_read_only"), testsupport.Strp("p25_pw")
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name:      "ACL user with no ~* keyspace grant",
			Principal: testsupport.RedisAclUser("p25_matrix_no_keys", "p25_pw", "+@all", "-@dangerous"),
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp("p25_matrix_no_keys"), testsupport.Strp("p25_pw")
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
			Then: []testsupport.Scenario{
				{
					// redis/errors.go's authPrefixRE maps *every* NOPERM (command-category or
					// key-pattern) to E_AUTH, not merely a credential failure — verified directly.
					// So a permission refusal from a missing keyspace pattern still surfaces as
					// E_AUTH today; the fix only concerned Connect's own probe sequence, not this
					// broader "every NOPERM is E_AUTH" mapping, which is a real permission/auth
					// conflation of its own. This case is left asserting the real, current
					// behaviour rather than the not-yet-true "not E_AUTH".
					Name: "Read of a key fails, and today that is coded E_AUTH (not yet distinguished from a credential failure)",
					Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
						_, err := a.Read(context.Background(), adapters.ReadRequest{
							Path:     testsupport.NodePath(cfg.ID, testsupport.Seg("database", "db0"), testsupport.Seg("key", "counter")),
							PageSize: 10,
						}, adapters.NewOpCtx("matrix-read"))
						if err == nil {
							t.Fatal("Read: want an error for a user with no ~* keyspace grant")
						}
						code, _ := adapters.CodeOf(err)
						if code != adapters.CodeAuth {
							t.Errorf("code = %v, want E_AUTH (current behaviour — see comment above)", code)
						}
					},
				},
				// The write half of the same pin: a user with no ~* keyspace grant is refused a
				// write too, and that refusal is also coded E_AUTH today — the same conflation the
				// read case above already pins, so a future errors.go fix breaks both halves
				// together.
				testsupport.MutateIsRefused(model.MutationPlan{
					Path: testsupport.NodePath(f.Config.ID, testsupport.Seg("database", "db0")),
					Ops: []model.MutationRowOp{{
						Kind: "insert",
						Values: model.RowValues{
							{Name: "_key", Value: testsupport.Strp("p26:matrix:no-keys")},
							{Name: "$value", Value: testsupport.Strp("nope")},
						},
					}},
				}, adapters.CodeAuth),
			},
		},
		{
			Name: "wrong password",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Password = testsupport.Strp("definitely-wrong")
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "username set (default), no password",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username = testsupport.Strp("default")
				c.Password = nil
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "no credentials at all",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = nil, nil
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "correct password, non-numeric database",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Database = testsupport.Strp("mydb")
				return c
			},
			// §1.3's own adjacent, not-fixed observation: a non-numeric database is silently
			// treated as index 0 (strconv.Atoi's error discarded). Pinned here as known, so a
			// later phase changing it breaks a test on purpose rather than silently.
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": "db0"}},
		},
		{
			Name: "correct password, out-of-range database",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Database = testsupport.Strp("99")
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeQuery},
		},
		{
			Name: "correct password, uri mode redis://:pw@host:port/1",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Mode = "uri"
				c.URI = testsupport.Strp(uri)
				return c
			},
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": "db1"}},
		},
	})
}
