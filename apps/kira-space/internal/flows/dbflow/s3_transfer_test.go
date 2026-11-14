package dbflow

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func s3ObjectPath(bucket, key string) string {
	return model.EncodePath([]model.PathSegment{{Kind: "bucket", Name: bucket}, {Kind: "object", Name: key}})
}

func s3FlowConn(t *testing.T, app *flowharness.App, fx *testsupport.S3Fixture, endpoint string) model.ConnectionSummary {
	t.Helper()
	in := inputFromConfig(fx.Config)
	in.URI = aws.String(*fx.Config.URI + "?endpoint=" + url.QueryEscape(endpoint))
	conn := createConn(t, app, in)
	if st := connect(t, app, conn.ID); st.Error != nil {
		t.Fatalf("connect: %s", *st.Error)
	}
	return conn
}

func noPartials(t *testing.T, dir string) {
	t.Helper()
	left, _ := filepath.Glob(filepath.Join(dir, "*.kira-partial-*"))
	if len(left) > 0 {
		t.Fatalf("temp files left behind: %v", left)
	}
}

// J-KV-013 download half: bytes equal the object, an existing file is replaced only at the final
// rename, failures leave no target and no temp sibling, and a hostile key never reaches outside the
// chosen destination.
func TestS3DownloadSafety(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartS3(t)
	app := flowharness.New(t)
	const bucket = testsupport.S3MutableBucket
	proxy := testsupport.StartPausableProxyTo(t, strings.TrimPrefix(fx.Endpoint, "http://"), fx.Config)
	conn := s3FlowConn(t, app, fx, "http://"+proxy.Addr())
	ds := app.DataStream(t)

	big := strings.Repeat("0123456789abcdef", 1<<16) // 1 MiB
	s3Put(t, fx, bucket, "dl/big.bin", big, nil)
	dir := t.TempDir()

	dest := filepath.Join(dir, "a.bin")
	n, err := ds.Download(conn.ID, s3ObjectPath(bucket, "dl/big.bin"), dest)
	if err != nil || n != int64(len(big)) {
		t.Fatalf("download = %d, %v", n, err)
	}
	if got, _ := os.ReadFile(dest); string(got) != big {
		t.Fatal("downloaded bytes differ from the object")
	}
	noPartials(t, dir)

	// Existing file: old content survives a failed download, is replaced by a good one.
	if err := os.WriteFile(dest, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.Download(conn.ID, s3ObjectPath(bucket, "dl/missing.bin"), dest); err == nil {
		t.Fatal("download of a deleted key succeeded")
	}
	if got, _ := os.ReadFile(dest); string(got) != "old" {
		t.Fatalf("existing file = %q after a failed download, want it intact", got)
	}
	noPartials(t, dir)

	// Cancel with the response body withheld: no target, no temp file.
	cancelDest := filepath.Join(dir, "cancelled.bin")
	proxy.PauseWhenSent("GET /" + bucket + "/dl/big.bin")
	opID, wait := ds.DownloadFire(conn.ID, s3ObjectPath(bucket, "dl/big.bin"), cancelDest)
	deadline := time.Now().Add(15 * time.Second)
	for {
		if m, _ := filepath.Glob(cancelDest + ".kira-partial-*"); len(m) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("download never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if ok, err := app.W.Router.Cancel(ctx, opID); err != nil || !ok {
		t.Fatalf("Cancel = %v, %v", ok, err)
	}
	proxy.Resume()
	if _, err := wait(); err == nil {
		t.Fatal("cancelled download reported success")
	}
	if _, err := os.Stat(cancelDest); !os.IsNotExist(err) {
		t.Fatalf("cancelled download left a target: %v", err)
	}
	noPartials(t, dir)

	// Unwritable destination directory: error, nothing created.
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 {
		if _, err := ds.Download(conn.ID, s3ObjectPath(bucket, "dl/big.bin"), filepath.Join(ro, "x.bin")); err == nil {
			t.Fatal("download into a read-only directory succeeded")
		}
		noPartials(t, ro)
	}
	// Destination directory that does not exist.
	if _, err := ds.Download(conn.ID, s3ObjectPath(bucket, "dl/big.bin"), filepath.Join(dir, "nope", "x.bin")); err == nil {
		t.Fatal("download into a missing directory succeeded")
	}

	// A key that looks like a traversal writes only where the user chose.
	s3Put(t, fx, bucket, "../escape.txt", "x", nil)
	chosen := filepath.Join(dir, "sub", "picked.txt")
	if err := os.Mkdir(filepath.Dir(chosen), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.Download(conn.ID, s3ObjectPath(bucket, "../escape.txt"), chosen); err != nil {
		t.Fatalf("download of ../escape.txt: %v", err)
	}
	for _, p := range []string{filepath.Join(dir, "escape.txt"), filepath.Join(filepath.Dir(dir), "escape.txt")} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("traversal key wrote %s", p)
		}
	}
	if got, _ := os.ReadFile(chosen); string(got) != "x" {
		t.Fatalf("chosen file = %q", got)
	}
}

// J-KV-013 upload half: odd keys land verbatim, an upload over an existing key is refused, and bad
// sources are refused before any request leaves.
func TestS3UploadKeysAndRefusals(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartS3(t)
	app := flowharness.New(t)
	const bucket = testsupport.S3MutableBucket
	conn := s3FlowConn(t, app, fx, fx.Endpoint)
	ds := app.DataStream(t)
	path := "bucket:" + bucket
	dir := t.TempDir()
	src := filepath.Join(dir, "up.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"in/with space.txt", "in/日本語.txt", "in/a//b.txt", "in/100%2Fx.txt", "in/dir/"} {
		_, err := ds.Mutate(conn.ID, path, []model.MutationRowOp{ins(vals("_key", key, "$file", src, "$contentType", "text/plain"))})
		if err != nil {
			t.Fatalf("upload %q: %v", key, err)
		}
		head, err := fx.Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		if err != nil {
			t.Fatalf("object %q not stored verbatim: %v", key, err)
		}
		if aws.ToString(head.ContentType) != "text/plain" {
			t.Fatalf("content type of %q = %q", key, aws.ToString(head.ContentType))
		}
	}

	// Existing key: refused, content unchanged.
	if _, err := ds.Mutate(conn.ID, path, []model.MutationRowOp{ins(vals("_key", "in/with space.txt", "$file", src))}); err == nil {
		t.Fatal("upload over an existing key succeeded")
	}

	// Bad sources: refused with the path in the message.
	missing := filepath.Join(dir, "missing.bin")
	fifo := filepath.Join(dir, "pipe")
	if err := syscallMkfifo(fifo); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]string{"missing": missing, "directory": dir, "pipe": fifo, "device": "/dev/zero"} {
		_, err := ds.Mutate(conn.ID, path, []model.MutationRowOp{ins(vals("_key", "in/bad-"+name, "$file", bad))})
		if err == nil || !strings.Contains(err.Error(), bad) {
			t.Fatalf("upload of %s: err = %v, want a refusal naming %s", name, err, bad)
		}
		if _, err := fx.Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String("in/bad-" + name)}); err == nil {
			t.Fatalf("refused upload of %s still created an object", name)
		}
	}
}
