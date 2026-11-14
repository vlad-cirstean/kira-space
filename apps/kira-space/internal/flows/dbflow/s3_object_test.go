package dbflow

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func s3Put(t *testing.T, fx *testsupport.S3Fixture, bucket, key, body string, in *s3.PutObjectInput) string {
	t.Helper()
	if in == nil {
		in = &s3.PutObjectInput{}
	}
	in.Bucket, in.Key, in.Body = aws.String(bucket), aws.String(key), strings.NewReader(body)
	out, err := fx.Client.PutObject(ctx, in)
	if err != nil {
		t.Fatalf("PutObject %s: %v", key, err)
	}
	return aws.ToString(out.ETag)
}

func s3Body(t *testing.T, fx *testsupport.S3Fixture, bucket, key string) string {
	t.Helper()
	out, err := fx.Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		t.Fatalf("GetObject %s: %v", key, err)
	}
	defer out.Body.Close()
	b, _ := io.ReadAll(out.Body)
	return string(b)
}

// J-KV-012 (contract studio-s3-edit): saving an object body keeps its content type, cache control and
// user metadata, creates a new version in a versioned bucket, and refuses when another client
// replaced the object after the editor opened it (OQ-KV-1: the editor sends the opened ETag).
func TestS3EditKeepsMetadataAndRefusesChangedObject(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartS3(t)
	app := flowharness.New(t)
	in := inputFromConfig(fx.Config)
	// URI mode derives options from the URI query, so the LocalStack endpoint rides there.
	in.URI = aws.String(*fx.Config.URI + "?endpoint=" + url.QueryEscape(fx.Endpoint))
	conn := createConn(t, app, in)
	if st := connect(t, app, conn.ID); st.Error != nil {
		t.Fatalf("connect: %s", *st.Error)
	}
	ds := app.DataStream(t)
	const bucket = testsupport.S3MutableBucket
	path := "bucket:" + bucket

	etag := s3Put(t, fx, bucket, "flow/edit.json", `{"a":1}`, &s3.PutObjectInput{
		ContentType: aws.String("application/json"), CacheControl: aws.String("max-age=60"),
		Metadata: map[string]string{"owner": "kira"},
	})
	ops := []model.MutationRowOp{upd(vals("_key", "flow/edit.json", "$etag", etag), vals("$value", `{"a":2}`))}
	app.Contract(t, "studio-s3-edit", "mutate:ops", flowharness.MutateWire(conn.ID, path, ops), contractMask, flowharness.Mask("$etag"))
	if _, err := ds.Mutate(conn.ID, path, ops); err != nil {
		t.Fatal(err)
	}
	head, err := fx.Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String("flow/edit.json")})
	if err != nil {
		t.Fatal(err)
	}
	if aws.ToString(head.ContentType) != "application/json" || aws.ToString(head.CacheControl) != "max-age=60" || head.Metadata["owner"] != "kira" {
		t.Fatalf("attributes after edit = %q %q %v, want them kept", aws.ToString(head.ContentType), aws.ToString(head.CacheControl), head.Metadata)
	}
	if got := s3Body(t, fx, bucket, "flow/edit.json"); got != `{"a":2}` {
		t.Fatalf("body = %q", got)
	}

	// Another client replaces the object after the editor opened it at `etag`'s successor.
	opened := aws.ToString(head.ETag)
	s3Put(t, fx, bucket, "flow/edit.json", `{"theirs":true}`, nil)
	if _, err := ds.Mutate(conn.ID, path, []model.MutationRowOp{upd(vals("_key", "flow/edit.json", "$etag", opened), vals("$value", "mine"))}); err == nil {
		t.Fatal("save over a replaced object succeeded")
	}
	if got := s3Body(t, fx, bucket, "flow/edit.json"); got != `{"theirs":true}` {
		t.Fatalf("body after refused save = %q, want the other client's", got)
	}

	// Versioned bucket: the save creates a new version and the old one stays readable.
	if _, err := fx.Client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{Bucket: aws.String(bucket),
		VersioningConfiguration: &s3types.VersioningConfiguration{Status: s3types.BucketVersioningStatusEnabled}}); err != nil {
		t.Fatal(err)
	}
	s3Put(t, fx, bucket, "flow/versioned.txt", "v1", nil)
	if _, err := ds.Mutate(conn.ID, path, []model.MutationRowOp{upd(vals("_key", "flow/versioned.txt"), vals("$value", "v2"))}); err != nil {
		t.Fatal(err)
	}
	vers, err := fx.Client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: aws.String(bucket), Prefix: aws.String("flow/versioned.txt")})
	if err != nil || len(vers.Versions) != 2 {
		t.Fatalf("versions = %d, %v, want 2", len(vers.Versions), err)
	}
	for _, v := range vers.Versions {
		out, _ := fx.Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: v.Key, VersionId: v.VersionId})
		b, _ := io.ReadAll(out.Body)
		out.Body.Close()
		if want := map[bool]string{true: "v2", false: "v1"}[aws.ToBool(v.IsLatest)]; string(b) != want {
			t.Fatalf("version latest=%v holds %q, want %q", aws.ToBool(v.IsLatest), b, want)
		}
	}
}

// aclDenyingProxy forwards to the S3 endpoint and answers every ACL request with AccessDenied, like
// credentials that lack s3:GetObjectAcl.
func aclDenyingProxy(t *testing.T, endpoint string) string {
	t.Helper()
	target, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, acl := r.URL.Query()["acl"]; acl {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
			return
		}
		r.Host = target.Host
		rp.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// Decision 6, J-KV-012: where the object's ACL cannot be read, an edit would reset it, so the save is
// refused with E_CONFLICT until the confirm; the confirmed save (same plan plus $aclReset) lands.
func TestS3EditWarnsWhenACLUnreadable(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartS3(t)
	app := flowharness.New(t)
	in := inputFromConfig(fx.Config)
	in.URI = aws.String(*fx.Config.URI + "?endpoint=" + url.QueryEscape(aclDenyingProxy(t, fx.Endpoint)))
	conn := createConn(t, app, in)
	if st := connect(t, app, conn.ID); st.Error != nil {
		t.Fatalf("connect: %s", *st.Error)
	}
	ds := app.DataStream(t)
	const bucket = testsupport.S3MutableBucket
	path := "bucket:" + bucket
	etag := s3Put(t, fx, bucket, "flow/acl.txt", "old", nil)

	guardedOps := []model.MutationRowOp{upd(vals("_key", "flow/acl.txt", "$etag", etag), vals("$value", "new"))}
	app.Contract(t, "studio-s3-edit", "mutate:acl-guarded-ops", flowharness.MutateWire(conn.ID, path, guardedOps), contractMask, flowharness.Mask("$etag"))
	_, err := ds.Mutate(conn.ID, path, guardedOps)
	var derr *flowharness.DataError
	if !errors.As(err, &derr) || derr.Code != "E_CONFLICT" {
		t.Fatalf("save with an unreadable ACL = %v, want E_CONFLICT", err)
	}
	app.Contract(t, "studio-s3-edit", "mutate:acl-conflict", derr, contractMask)
	if got := s3Body(t, fx, bucket, "flow/acl.txt"); got != "old" {
		t.Fatalf("body = %q after the refused save, want it unchanged", got)
	}

	confirmedOps := []model.MutationRowOp{upd(vals("_key", "flow/acl.txt", "$etag", etag, "$aclReset", "1"), vals("$value", "new"))}
	app.Contract(t, "studio-s3-edit", "mutate:acl-confirmed-ops", flowharness.MutateWire(conn.ID, path, confirmedOps), contractMask, flowharness.Mask("$etag"))
	if _, err := ds.Mutate(conn.ID, path, confirmedOps); err != nil {
		t.Fatalf("confirmed save: %v", err)
	}
	if got := s3Body(t, fx, bucket, "flow/acl.txt"); got != "new" {
		t.Fatalf("body = %q after the confirmed save", got)
	}
}
