// authmatrix_test.go is the complete (Tier 2) suite's own table for mongo — P25 §2.6. The
// authSource dimension is what makes this matrix different from every other adapter's, and it is
// the one that carried §1.2's bug.
package mongo_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func TestMongo_AuthMatrix(t *testing.T) {
	testsupport.RequireMatrix(t)
	f := testsupport.StartMongo(t)
	root := rootClient(t, f)
	ctx := context.Background()

	const adminUser, adminPass = "p25_matrix_admin_user", "p25_pw"
	if err := root.Database("admin").RunCommand(ctx, bson.D{
		{Key: "createUser", Value: adminUser},
		{Key: "pwd", Value: adminPass},
		{Key: "roles", Value: bson.A{
			bson.D{{Key: "role", Value: "readWrite"}, {Key: "db", Value: testsupport.MongoDatabase}},
		}},
	}).Err(); err != nil {
		t.Fatalf("create admin-scoped user: %v", err)
	}
	t.Cleanup(func() {
		_ = root.Database("admin").RunCommand(ctx, bson.D{{Key: "dropUser", Value: adminUser}}).Err()
	})

	const roUser, roPass = "p25_matrix_ro_user", "p25_pw"
	if err := root.Database(testsupport.MongoDatabase).RunCommand(ctx, bson.D{
		{Key: "createUser", Value: roUser},
		{Key: "pwd", Value: roPass},
		{Key: "roles", Value: bson.A{
			bson.D{{Key: "role", Value: "read"}, {Key: "db", Value: testsupport.MongoDatabase}},
		}},
	}).Err(); err != nil {
		t.Fatalf("create read-only user: %v", err)
	}
	t.Cleanup(func() {
		_ = root.Database(testsupport.MongoDatabase).RunCommand(ctx, bson.D{{Key: "dropUser", Value: roUser}}).Err()
	})

	adminUriURI := "mongodb://" + adminUser + ":" + adminPass + "@" + f.Host + ":" + strconv.Itoa(f.Port) + "/?authSource=admin"

	testsupport.RunMatrix(t, "mongodb", f, f.Config, []testsupport.Case{
		{
			Name:   "kira in kira_test, database=kira_test",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig { return c },
			Expect: testsupport.Outcome{Succeed: true},
		},
		{
			Name: "kira in kira_test, database unset",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Database = nil
				return c
			},
			// authSource defaults to admin, where `kira` does not exist — pins that the coupling
			// between "database" and "authSource" is real and understood, in both directions.
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "admin-scoped user, database=kira_test, no authSource",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp(adminUser), testsupport.Strp(adminPass)
				c.Database = testsupport.Strp(testsupport.MongoDatabase)
				return c
			},
			// The pre-fix bug shape, as a permanent assertion: the URI path is defaultauthdb, so
			// without an explicit authSource this must still fail — the fix is additive (§1.2),
			// not a change to this default.
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			Name: "admin-scoped user, database=kira_test, options.authSource=admin",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp(adminUser), testsupport.Strp(adminPass)
				c.Database = testsupport.Strp(testsupport.MongoDatabase)
				c.Options = map[string]any{"authSource": "admin"}
				return c
			},
			Expect: testsupport.Outcome{Succeed: true, Details: map[string]string{"database": testsupport.MongoDatabase}},
			Then: []testsupport.Scenario{
				{
					// P25 §1.2's fix is asserted only for the handshake there. This carries it one
					// step further: the fixed fields-mode connection (username/database +
					// options.authSource=admin) can actually write as the principal it authenticated,
					// not merely connect as it.
					Name: "the authSource-fixed connection can actually write",
					Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
						path := testsupport.NodePath(cfg.ID, testsupport.Seg("database", testsupport.MongoDatabase), testsupport.Seg("collection", "mutate_probe"))
						plan := model.MutationPlan{Path: path, Ops: []model.MutationRowOp{{
							Kind:   "insert",
							Values: model.RowValues{{Name: "$document", Value: testsupport.Strp(`{ name: "matrix-authsource-write", n: 1 }`)}},
						}}}
						result, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("matrix-authsource-mutate"))
						if err != nil {
							t.Fatalf("Mutate: %v", err)
						}
						if result.AffectedRows != 1 {
							t.Fatalf("AffectedRows = %d, want 1", result.AffectedRows)
						}
						read, err := a.Read(context.Background(), adapters.ReadRequest{
							Path: path, PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
						}, adapters.NewOpCtx("matrix-authsource-read"))
						if err != nil {
							t.Fatalf("Read: %v", err)
						}
						docPage := read.(page.DocumentPage)
						var foundID *string
						for row := 0; row < docPage.RowCount; row++ {
							if body := testsupport.DocBodyAt(t, docPage, row); body != nil && strings.Contains(*body, "matrix-authsource-write") {
								foundID = testsupport.DocIDAt(t, docPage, row)
							}
						}
						if foundID == nil {
							t.Fatal("the written document was not found on read-back")
						}
						if _, err := a.Mutate(context.Background(), model.MutationPlan{
							Path: path,
							Ops:  []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "_id", Value: foundID}}}},
						}, adapters.NewOpCtx("matrix-authsource-cleanup")); err != nil {
							t.Errorf("cleanup delete: %v", err)
						}
					},
				},
			},
		},
		{
			Name: "admin-scoped user, database unset",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp(adminUser), testsupport.Strp(adminPass)
				c.Database = nil
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
		},
		{
			Name: "read-only user in kira_test, database=kira_test",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp(roUser), testsupport.Strp(roPass)
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
			Then: []testsupport.Scenario{
				{
					Name: "Children(root) is [kira_test]",
					Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
						children, err := a.Children(context.Background(), testsupport.NodePath(cfg.ID), adapters.NewOpCtx("matrix-children"))
						if err != nil {
							t.Fatalf("Children: %v", err)
						}
						names := testsupport.ChildNames(t, children)
						if len(names) != 1 || names[0] != testsupport.MongoDatabase {
							t.Errorf("names = %v, want exactly [%s]", names, testsupport.MongoDatabase)
						}
					},
				},
				// The "read" role must read and filter — nothing proved that before this phase.
				testsupport.ReadFirstPage(testsupport.NodePath(f.Config.ID, testsupport.Seg("database", testsupport.MongoDatabase), testsupport.Seg("collection", "widgets"))),
				testsupport.FilterNarrowsResult(
					testsupport.NodePath(f.Config.ID, testsupport.Seg("database", testsupport.MongoDatabase), testsupport.Seg("collection", "widgets")),
					`{ name: "widget-3" }`, 1,
				),
				// §1.4's pin for mongo. CommandError code 13 (Unauthorized) shares errors.go:29-31's
				// branch with 18 (AuthenticationFailed) — a missing role reads as a wrong password
				// today. Pinned with this comment naming it, exactly as P25's redis matrix pinned its
				// own NOPERM equivalent (redis/authmatrix_test.go:82-104).
				// CountMatchesRead is deliberately not attached: mongo/caps.go sets ExactCount: false,
				// so its own Requires would skip it anyway — noted so a reader does not read the
				// omission as an oversight.
				testsupport.MutateIsRefused(model.MutationPlan{
					Path: testsupport.NodePath(f.Config.ID, testsupport.Seg("database", testsupport.MongoDatabase), testsupport.Seg("collection", "widgets")),
					Ops: []model.MutationRowOp{{
						Kind:   "insert",
						Values: model.RowValues{{Name: "$document", Value: testsupport.Strp(`{ name: "matrix-nope" }`)}},
					}},
				}, adapters.CodeAuth),
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
			Name: "nonexistent user",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Username, c.Password = testsupport.Strp("p25-nonexistent-user"), testsupport.Strp("whatever")
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
			Name: "admin-scoped user, uri mode ?authSource=admin",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Mode = "uri"
				c.URI = testsupport.Strp(adminUriURI)
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
		},
	})
}
