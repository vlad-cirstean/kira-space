package s3_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// F16: an edit re-sends the object's ACL; PutObject alone would reset it to owner-only.
func TestS3_Mutate_UpdatePreservesACL(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()
	const key = "acl-probe/index.html"
	if _, err := fixture.Client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(testsupport.S3MutableBucket), Key: aws.String(key),
		Body: strings.NewReader("<p>old</p>"), ACL: s3types.ObjectCannedACLPublicRead,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	publicRead := func() bool {
		acl, err := fixture.Client.GetObjectAcl(ctx, &awss3.GetObjectAclInput{Bucket: aws.String(testsupport.S3MutableBucket), Key: aws.String(key)})
		if err != nil {
			t.Fatalf("GetObjectAcl: %v", err)
		}
		for _, g := range acl.Grants {
			if g.Grantee != nil && g.Grantee.URI != nil && strings.HasSuffix(*g.Grantee.URI, "/global/AllUsers") && g.Permission == s3types.PermissionRead {
				return true
			}
		}
		return false
	}
	if !publicRead() {
		t.Skip("endpoint did not store the public-read ACL")
	}

	body := "<p>new</p>"
	plan := model.MutationPlan{
		Path: bucketPath(fixture, testsupport.S3MutableBucket),
		Ops:  []model.MutationRowOp{{Kind: "update", Key: model.RowValues{{Name: "_key", Value: testsupport.Strp(key)}}, Changes: model.RowValues{{Name: "$value", Value: &body}}}},
	}
	if _, err := a.Mutate(ctx, plan, adapters.NewOpCtx("op-acl")); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if !publicRead() {
		t.Fatal("the edit dropped the public-read grant")
	}
}

// F17: options.bucket scopes every path, not only the root listing.
func TestS3_ScopedBucket_RefusesOtherBuckets(t *testing.T) {
	fixture := testsupport.StartS3(t)
	cfg := fixture.Config
	cfg.Options = map[string]any{"endpoint": fixture.Endpoint, "bucket": testsupport.S3MainBucket}
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("op-scope-connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	ctx := context.Background()
	other := testsupport.S3MutableBucket

	wantNotFound := func(what string, err error) {
		t.Helper()
		if code, _ := adapters.CodeOf(err); code != adapters.CodeNotFound {
			t.Errorf("%s: err = %v, want E_NOT_FOUND", what, err)
		}
	}
	_, err := a.Children(ctx, bucketPath(fixture, other), adapters.NewOpCtx("op-scope-children"))
	wantNotFound("Children", err)
	_, err = a.Read(ctx, offsetRead(objectPath(fixture, other, testsupport.S3EditableObjectKey)), adapters.NewOpCtx("op-scope-read"))
	wantNotFound("Read", err)
	_, err = a.Count(ctx, adapters.CountRequest{Path: objectPath(fixture, other, testsupport.S3EditableObjectKey)}, adapters.NewOpCtx("op-scope-count"))
	wantNotFound("Count", err)
	_, err = a.Mutate(ctx, model.MutationPlan{
		Path: bucketPath(fixture, other),
		Ops:  []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "_key", Value: testsupport.Strp(testsupport.S3EditableObjectKey)}}}},
	}, adapters.NewOpCtx("op-scope-mutate"))
	wantNotFound("Mutate", err)

	if _, err := a.Children(ctx, bucketPath(fixture, testsupport.S3MainBucket), adapters.NewOpCtx("op-scope-ok")); err != nil {
		t.Errorf("Children on the scoped bucket: %v", err)
	}
}

// F19: an upload source must be a regular file.
func TestS3_Mutate_InsertRefusesNonRegularSource(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)
	plan := model.MutationPlan{
		Path: bucketPath(fixture, testsupport.S3MutableBucket),
		Ops:  []model.MutationRowOp{{Kind: "insert", Values: model.RowValues{{Name: "_key", Value: testsupport.Strp("from-dir.txt")}, {Name: "$file", Value: testsupport.Strp(t.TempDir())}}}},
	}
	_, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-nonregular"))
	if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("err = %v, want E_QUERY not a regular file", err)
	}
}

func TestS3_Connect_CancelledCtxReturns(t *testing.T) {
	fixture := testsupport.StartS3(t)
	proxy := testsupport.StartPausableProxyTo(t, strings.TrimPrefix(fixture.Endpoint, "http://"), fixture.Config)
	cfg := fixture.Config
	cfg.Options = map[string]any{"endpoint": "http://" + proxy.Addr()}
	testsupport.ConnectCancelScenario(t, newAdapter(t), cfg, proxy)
}
