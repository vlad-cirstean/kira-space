package testsupport

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/testcontainers/testcontainers-go"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// 0007_s3_seed.ts's own constants, re-expressed in Go (P58d D24).
const (
	S3MainBucket  = "main-bucket"
	S3EmptyBucket = "empty-bucket"

	S3RootObjectKey          = "readme.txt"
	S3RootObjectBody         = "hello from the bucket root"
	S3NestedObjectKey        = "reports/2024/summary.json"
	S3SiblingPrefixObjectKey = "reports/notes.txt"
	S3SmallForCountKey       = "sizes/small-for-count.txt"
	S3SmallForCountBody      = "a small object, used only as the count() comparison baseline"
	S3OversizedObjectKey     = "sizes/oversized.bin"
	S3BinaryObjectKey        = "sizes/logo.png"
	s3BinaryObjectBase64     = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

	S3MutableBucket      = "mutable-bucket"
	S3EditableObjectKey  = "editable.json"
	S3EditableObjectBody = `{"status":"draft"}`
	S3ReadonlyTargetKey  = "readonly-target.txt"
	S3ReadonlyTargetBody = "must not change if a read-only connection tries to edit it"
	S3DeleteTargetKey    = "delete-target.txt"
	S3DeleteTargetBody   = "this object exists only to be deleted"
	// P58d M8.0's AWS-4 found this missing from the plan's own §4.6 checklist: the TypeScript's
	// delete scenario removes one object from a tree row and a different one from an open tab in
	// the same test, so each needs its own key.
	S3SecondDeleteTargetKey  = "second-delete-target.txt"
	S3SecondDeleteTargetBody = "a second object, deleted from an open tab instead of the tree"
	S3UploadTargetKey        = "uploaded-from-disk.txt" // never pre-seeded
)

// S3OversizedObjectBytes mirrors 0007_s3_seed.ts's own OVERSIZED_OBJECT_BYTES — sized relative to
// the real shared threshold so this fixture tracks any future change to it automatically.
var S3OversizedObjectBytes = page.SingleRowMaxBytes + 1024

// S3Fixture is support/s3.ts's S3Fixture.
type S3Fixture struct {
	Config    model.ResolvedConnectionConfig
	Client    *s3.Client // a side client, for per-test setup only
	Endpoint  string
	container testcontainers.Container
}

// ConnectionID satisfies hasConnectionID (P107 I2-28, FixtureNodePath's own constraint).
func (f S3Fixture) ConnectionID() string { return f.Config.ID }

var s3Memo = fixture[S3Fixture]{reset: resetS3}

// StartS3 is support/s3.ts's startS3. Skips the test when Docker is unreachable.
func StartS3(t *testing.T) *S3Fixture {
	return s3Memo.start(t, "s3 container", startS3)
}

// StopS3 terminates the memoized container, if ever started. Call once, from the test binary's
// own TestMain, after m.Run() returns.
func StopS3() {
	s3Memo.stop(func(f *S3Fixture) { _ = f.container.Terminate(context.Background()) })
}

func startS3() (*S3Fixture, error) {
	ctx := context.Background()
	c, endpoint, err := startLocalStack(ctx, "s3")
	if err != nil {
		return nil, err
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(LocalStackRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(LocalStackStaticAccessKey, LocalStackStaticSecret, "")),
	)
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	if err := seedS3(ctx, client); err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	cfg := model.ResolvedConnectionConfig{
		ID: "test-s3", SortOrder: 0, CreatedAt: now, UpdatedAt: now,
		Name: "Test S3", Kind: "s3", Color: "amber", Mode: "uri", ReadOnly: false,
		URI:     Strp(fmt.Sprintf("s3://%s:%s@%s", LocalStackStaticAccessKey, LocalStackStaticSecret, LocalStackRegion)),
		Options: map[string]any{"endpoint": endpoint},
	}
	return &S3Fixture{Config: cfg, Client: client, Endpoint: endpoint, container: c}, nil
}

// resetS3 empties and deletes every bucket, then reseeds.
func resetS3(f *S3Fixture) error {
	ctx := context.Background()
	buckets, err := f.Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return err
	}
	for _, b := range buckets.Buckets {
		pager := s3.NewListObjectsV2Paginator(f.Client, &s3.ListObjectsV2Input{Bucket: b.Name})
		for pager.HasMorePages() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				return err
			}
			for _, o := range page.Contents {
				if _, err := f.Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: b.Name, Key: o.Key}); err != nil {
					return err
				}
			}
		}
		if _, err := f.Client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: b.Name}); err != nil {
			return err
		}
	}
	return seedS3(ctx, f.Client)
}

func seedS3(ctx context.Context, client *s3.Client) error {
	for _, bucket := range []string{S3MainBucket, S3EmptyBucket, S3MutableBucket} {
		if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
			return err
		}
	}

	binaryObject, err := base64.StdEncoding.DecodeString(s3BinaryObjectBase64)
	if err != nil {
		return err
	}

	puts := []*s3.PutObjectInput{
		{
			Bucket: aws.String(S3MainBucket), Key: aws.String(S3RootObjectKey),
			Body: strings.NewReader(S3RootObjectBody), ContentType: aws.String("text/plain"),
			Metadata: map[string]string{"seeded": "true"},
		},
		{
			Bucket: aws.String(S3MainBucket), Key: aws.String(S3NestedObjectKey),
			Body: strings.NewReader(`{"year":2024,"total":42}`), ContentType: aws.String("application/json"),
		},
		{
			Bucket: aws.String(S3MainBucket), Key: aws.String(S3SiblingPrefixObjectKey),
			Body: strings.NewReader("a sibling of the 2024/ prefix, under reports/ itself"), ContentType: aws.String("text/plain"),
		},
		{
			Bucket: aws.String(S3MainBucket), Key: aws.String(S3SmallForCountKey),
			Body: strings.NewReader(S3SmallForCountBody), ContentType: aws.String("text/plain"),
		},
		{
			Bucket: aws.String(S3MainBucket), Key: aws.String(S3OversizedObjectKey),
			Body: bytes.NewReader(bytes.Repeat([]byte("x"), S3OversizedObjectBytes)), ContentType: aws.String("text/plain"),
		},
		{
			Bucket: aws.String(S3MainBucket), Key: aws.String(S3BinaryObjectKey),
			Body: bytes.NewReader(binaryObject), ContentType: aws.String("image/png"),
		},
		{
			Bucket: aws.String(S3MutableBucket), Key: aws.String(S3EditableObjectKey),
			Body: strings.NewReader(S3EditableObjectBody), ContentType: aws.String("application/json"),
			Metadata: map[string]string{"seeded": "true"},
		},
		{
			Bucket: aws.String(S3MutableBucket), Key: aws.String(S3ReadonlyTargetKey),
			Body: strings.NewReader(S3ReadonlyTargetBody), ContentType: aws.String("text/plain"),
		},
		{
			Bucket: aws.String(S3MutableBucket), Key: aws.String(S3DeleteTargetKey),
			Body: strings.NewReader(S3DeleteTargetBody), ContentType: aws.String("text/plain"),
		},
		{
			Bucket: aws.String(S3MutableBucket), Key: aws.String(S3SecondDeleteTargetKey),
			Body: strings.NewReader(S3SecondDeleteTargetBody), ContentType: aws.String("text/plain"),
		},
	}
	for _, in := range puts {
		if _, err := client.PutObject(ctx, in); err != nil {
			return err
		}
	}
	return nil
}
