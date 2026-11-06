package s3

import (
	"context"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
)

// downloadObject is transfer.ts's downloadObject — the only code that writes to a user's
// filesystem. Every step is transfer.ts translated, in the same order, for the same reasons:
//
//  1. CheckNotStarted: an already-cancelled op never creates a file.
//  2. op.SetCommand before any call (Adapter rule 3).
//  3. HeadObject first — a real "no such object" error before any local file exists.
//  4. tmp is a SIBLING of destPath, never os.TempDir() — the rename must be atomic on one
//     filesystem.
//  5. Create the temp file.
//  6. GetObject.
//  7. io.Copy streams the body in — ctx cancellation surfaces here, mid-stream, since the op's own
//     ctx goes directly to the SDK call (never adapters.RunWithAbortRace, which would let
//     a cancelled download keep writing after the caller's cleanup already ran).
//  8. Sync and close.
//  9. Rename — the last step, never retried; a cross-device rename is impossible by construction
//     (step 4's sibling-temp-file rule), so there is no fallback copy on purpose.
//
// On any error from steps 5-9, the temp file is unlinked (error ignored) before the mapped error
// is returned — and that cleanup runs on the caller's own goroutine, which is only safe because
// the copy itself never outlives this call.
func downloadObject(ctx context.Context, client *s3.Client, bucket, key, destPath string, op *adapters.OpCtx) (model.ObjectTransferResult, error) {
	if err := adapters.CheckNotStarted(ctx); err != nil {
		return model.ObjectTransferResult{}, err
	}
	op.SetCommand("GetObject s3://" + bucket + "/" + key + " -> " + destPath)

	if _, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}); err != nil {
		return model.ObjectTransferResult{}, mapError(err)
	}

	tmpPath := destPath + ".kira-partial-" + ids.New(ids.Temp)
	f, err := os.Create(tmpPath)
	if err != nil {
		return model.ObjectTransferResult{}, mapError(err)
	}
	cleanup := func() { _ = os.Remove(tmpPath) }

	res, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		f.Close()
		cleanup()
		return model.ObjectTransferResult{}, mapError(err)
	}
	if res.Body == nil {
		f.Close()
		cleanup()
		return model.ObjectTransferResult{}, mapError(errNoBody(key))
	}
	defer res.Body.Close()

	n, err := io.Copy(f, res.Body)
	if err != nil {
		f.Close()
		cleanup()
		return model.ObjectTransferResult{}, mapError(err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		cleanup()
		return model.ObjectTransferResult{}, mapError(err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return model.ObjectTransferResult{}, mapError(err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		cleanup()
		return model.ObjectTransferResult{}, mapError(err)
	}

	return model.ObjectTransferResult{Bytes: n}, nil
}

type queryError string

func (e queryError) Error() string { return string(e) }

func errNoBody(key string) error { return queryError(key + " has no body to download") }

// openUploadBody is transfer.ts's openUploadBody — stat()s the file before any network call, so a
// missing/unreadable source or an over-limit file fails before PutObject ever starts. The file is
// seekable and its length known, which lets the SDK checksum by seeking rather than
// falling into the aws-chunked path a non-seekable body would need. Closed by the caller with a
// defer (mutate.go's applyInsert) — the TypeScript never closes its createReadStream because the
// SDK consumes it and Node's GC finishes the job, but a leaked fd in Go is real.
func openUploadBody(sourcePath string) (*os.File, int64, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, 0, adapters.New(adapters.CodeQuery, "could not read local file "+sourcePath+": "+err.Error(), err)
	}
	// A named pipe blocks os.Open outside ctx, and a device such as /dev/zero stats as 0 bytes.
	if !info.Mode().IsRegular() {
		return nil, 0, adapters.New(adapters.CodeQuery, "local path "+sourcePath+" is not a regular file", nil)
	}
	if info.Size() > int64(page.ObjectUploadMaxBytes) {
		return nil, 0, adapters.New(adapters.CodeUnsupported,
			"file is "+formatBytes(info.Size())+", over the "+formatBytes(int64(page.ObjectUploadMaxBytes))+" single-upload limit — multipart upload is not supported", nil)
	}
	f, err := os.Open(sourcePath)
	if err != nil {
		return nil, 0, adapters.New(adapters.CodeQuery, "could not read local file "+sourcePath+": "+err.Error(), err)
	}
	return f, info.Size(), nil
}
