// authmatrix_test.go is the complete suite's own table for sqs. Client-config permutations only:
// LocalStack does not enforce credentials at any configuration this fixture can drive, verified
// directly (a bogus access key connects successfully even with ENFORCE_IAM=1), so there is no way
// to exercise a genuine SigV4 rejection or IAM permission boundary here. sqs and s3 share awscfg,
// so this table's shape mirrors s3's own rather than a shared driver — per-adapter tables are the
// point.
package sqs_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// clearAwsEnv/seedAmbientAwsEnv are testsupport.ClearAwsEnv/testsupport.SeedAmbientAwsEnv (P107
// I2-28 — byte-identical to s3/authmatrix_test.go's own, moved to testsupport).
var (
	clearAwsEnv       = testsupport.ClearAwsEnv
	seedAmbientAwsEnv = testsupport.SeedAmbientAwsEnv
)

func TestSqs_AuthMatrix(t *testing.T) {
	testsupport.RequireMatrix(t)
	f := testsupport.StartSqs(t)

	testsupport.RunMatrix(t, "sqs", f, f.Config, []testsupport.Case{
		{
			Name:   "uri mode, key+secret in the URI host",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig { return c },
			Expect: testsupport.Outcome{Succeed: true},
		},
		{
			Name: "uri mode, key only, no secret",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.URI = testsupport.Strp("sqs://" + testsupport.LocalStackStaticAccessKey + "@" + testsupport.LocalStackRegion)
				return c
			},
			// config.go's both-or-nothing: a lone access key with no secret is refused outright,
			// before any connection attempt (P168 Part 4 F1) — it no longer falls back to the
			// ambient credential chain.
			Expect: testsupport.Outcome{FailWith: adapters.CodeConnect},
		},
		{
			Name: "uri mode, malformed URI",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.URI = testsupport.Strp("not a valid uri at all")
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeConnect},
		},
		{
			Name:      "fields mode, region only, no profile, no ambient credentials",
			Principal: clearAwsEnv,
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Mode = "fields"
				c.Database = testsupport.Strp(testsupport.LocalStackRegion)
				c.Username, c.URI, c.Options = nil, nil, map[string]any{}
				return c
			},
			Expect: testsupport.Outcome{NotCode: adapters.CodeAuth},
		},
		{
			Name: "fields mode, profile that does not exist",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Mode = "fields"
				c.Database = testsupport.Strp(testsupport.LocalStackRegion)
				c.Username = testsupport.Strp("p25-nonexistent-profile")
				c.URI, c.Options = nil, map[string]any{}
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeAuth},
		},
		{
			// Every functional SQS test today runs URI mode (testsupport/sqs.go's own config is
			// URI-only), so awscfg's fields branch had never served a data-plane request for this
			// adapter either. Kept to one scenario deliberately: LocalStack does not enforce
			// IAM at any configuration this fixture can drive (measured twice), so there is
			// no least-privilege principal to cross this with — the same reason this table has no
			// auth-posture rows at all.
			Name:      "fields mode, region set, ambient credentials",
			Principal: seedAmbientAwsEnv,
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Mode = "fields"
				c.Database = testsupport.Strp(testsupport.LocalStackRegion)
				c.Username, c.URI = nil, nil
				return c
			},
			Expect: testsupport.Outcome{Succeed: true},
			Then: []testsupport.Scenario{
				{
					Name: "send then receive against a scratch queue",
					Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
						const queueName = "p26-matrix-fields-send-receive"
						if _, err := f.Client.CreateQueue(context.Background(), &awssqs.CreateQueueInput{QueueName: aws.String(queueName)}); err != nil {
							t.Fatalf("CreateQueue: %v", err)
						}
						path := testsupport.NodePath(cfg.ID, testsupport.Seg("queue", queueName))

						sendPlan := model.MutationPlan{
							Path: path,
							Ops:  []model.MutationRowOp{{Kind: "insert", Values: model.RowValues{{Name: "$body", Value: testsupport.Strp("matrix-hello")}}}},
						}
						sendResult, err := a.Mutate(context.Background(), sendPlan, adapters.NewOpCtx("matrix-fields-send"))
						if err != nil {
							t.Fatalf("Mutate insert: %v", err)
						}
						if sendResult.AffectedRows != 1 {
							t.Fatalf("AffectedRows = %d, want 1", sendResult.AffectedRows)
						}

						p, err := a.Read(context.Background(), adapters.ReadRequest{
							Path: path, PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
						}, adapters.NewOpCtx("matrix-fields-receive"))
						if err != nil {
							t.Fatalf("Read: %v", err)
						}
						sp := p.(page.StreamPage)
						if sp.RowCount != 1 {
							t.Fatalf("RowCount = %d, want 1", sp.RowCount)
						}
						body := testsupport.StreamBodyAt(t, sp, 0)
						if body == nil || *body != "matrix-hello" {
							t.Errorf("body = %v, want matrix-hello", body)
						}
					},
				},
			},
		},
		{
			Name: "fields mode, region unset",
			Config: func(c model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
				c.Mode = "fields"
				c.Database, c.Username, c.URI = nil, nil, nil
				c.Options = map[string]any{}
				return c
			},
			Expect: testsupport.Outcome{FailWith: adapters.CodeConnect},
		},
	})

	// "options.endpoint absent" sends the request to real AWS, which this
	// sandbox's outbound network cannot reach unassisted (a bare TCP route to AWS's own endpoints,
	// not proxied).
	t.Run("uri mode, valid credentials, options.endpoint absent (declined: needs real AWS network access)", func(t *testing.T) {
		testsupport.RequireMatrix(t)
		t.Skip("declined: reaching real AWS with no endpoint override depends on outbound network this sandbox does not have")
	})
}
