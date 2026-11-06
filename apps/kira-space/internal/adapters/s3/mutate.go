package s3

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// P33 D2/F10's sentinels, mirrored from packages/shared/domain/objectStore.ts — edit, delete and
// upload ride the existing mutate() path with the same sentinel-through-RowValues technique
// redis/mutate.go established.
const (
	objectKeySentinel         = "_key"
	objectFileSentinel        = "$file"
	objectContentTypeSentinel = "$contentType"
)

// resolveBucketSegment is mutate's own bucket-rooted path check, same shape as redis's
// resolveDatabaseSegment: the renderer sends an object's own path (delete) or a prefix path (upload),
// so only the leading bucket segment is checked, at any depth.
func resolveBucketSegment(path model.NodePath) (string, error) {
	segs, err := adapters.RequirePathPrefix(path, "mutate", adapters.Seg("bucket"))
	if err != nil {
		return "", err
	}
	return segs[0].Name, nil
}

func keyFrom(values model.RowValues, label string) (string, error) {
	raw, ok := values.Get(objectKeySentinel)
	if !ok || raw == nil || *raw == "" {
		return "", adapters.Contract(fmt.Sprintf("an s3 %s mutation requires a non-empty %s", label, objectKeySentinel), nil)
	}
	return *raw, nil
}

func fileFrom(values model.RowValues) (string, error) {
	raw, ok := values.Get(objectFileSentinel)
	if !ok || raw == nil || *raw == "" {
		return "", adapters.New(adapters.CodeUnsupported, "an s3 insert mutation requires a non-empty "+objectFileSentinel, nil)
	}
	return *raw, nil
}

func contentTypeFrom(values model.RowValues) *string {
	raw, ok := values.Get(objectContentTypeSentinel)
	if !ok || raw == nil || *raw == "" {
		return nil
	}
	return raw
}

// preview is mutate.ts's preview — synchronous (Adapter rule 3), no network call.
func preview(plan model.MutationPlan) ([]string, error) {
	bucket, err := resolveBucketSegment(plan.Path)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(plan.Ops))
	for i, op := range plan.Ops {
		text, err := renderOpText(bucket, op)
		if err != nil {
			return nil, err
		}
		out[i] = text
	}
	return out, nil
}

func renderOpText(bucket string, op model.MutationRowOp) (string, error) {
	switch op.Kind {
	case "update":
		key, err := keyFrom(op.Key, "update")
		if err != nil {
			return "", err
		}
		value, err := adapters.ValueFrom(op.Changes, "update")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("PutObject s3://%s/%s (%s)", bucket, key, formatBytes(int64(len(value)))), nil
	case "delete":
		key, err := keyFrom(op.Key, "delete")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("DeleteObject s3://%s/%s", bucket, key), nil
	default: // insert
		key, err := keyFrom(op.Values, "insert")
		if err != nil {
			return "", err
		}
		file, err := fileFrom(op.Values)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("PutObject s3://%s/%s <- %s", bucket, key, file), nil
	}
}

// preservedAttributes is mutate.ts's preservedAttributes: carries over every attribute
// HeadObject returns and PutObject accepts. PutObject replaces an object wholesale, so anything
// not resent here is gone — silently turning application/json into binary/octet-stream, or
// dropping Content-Encoding: gzip, would change how the object is served to everything downstream.
//
// Also carries SSE (ServerSideEncryption/SSEKMSKeyId/BucketKeyEnabled — a real security
// downgrade left out: an SSE-KMS-encrypted object silently re-encrypted under the bucket default
// after any edit, no longer needing kms:Decrypt to read), Expires, WebsiteRedirectLocation, and the
// three object-lock fields, all confirmed present on both HeadObjectOutput and PutObjectInput.
// Tags are handled separately — see applyUpdate's own TagCount check — rather than here.
func applyPreservedAttributes(in *s3.PutObjectInput, head *s3.HeadObjectOutput) {
	if head.ContentType != nil {
		in.ContentType = head.ContentType
	}
	if head.CacheControl != nil {
		in.CacheControl = head.CacheControl
	}
	if head.ContentEncoding != nil {
		in.ContentEncoding = head.ContentEncoding
	}
	if head.ContentDisposition != nil {
		in.ContentDisposition = head.ContentDisposition
	}
	if head.ContentLanguage != nil {
		in.ContentLanguage = head.ContentLanguage
	}
	if head.StorageClass != "" {
		in.StorageClass = head.StorageClass
	}
	if head.Metadata != nil {
		in.Metadata = head.Metadata
	}
	if head.Expires != nil {
		in.Expires = head.Expires
	}
	if head.WebsiteRedirectLocation != nil {
		in.WebsiteRedirectLocation = head.WebsiteRedirectLocation
	}
	if head.ServerSideEncryption != "" {
		in.ServerSideEncryption = head.ServerSideEncryption
	}
	if head.SSEKMSKeyId != nil {
		in.SSEKMSKeyId = head.SSEKMSKeyId
	}
	if head.BucketKeyEnabled != nil {
		in.BucketKeyEnabled = head.BucketKeyEnabled
	}
	if head.ObjectLockMode != "" {
		in.ObjectLockMode = head.ObjectLockMode
	}
	if head.ObjectLockRetainUntilDate != nil {
		in.ObjectLockRetainUntilDate = head.ObjectLockRetainUntilDate
	}
	if head.ObjectLockLegalHoldStatus != "" {
		in.ObjectLockLegalHoldStatus = head.ObjectLockLegalHoldStatus
	}
}

// grantHeaders are PutObject's ACL grant headers (`id="..."`, `uri="..."`, comma-joined); nil when
// the object holds nothing beyond its owner's FULL_CONTROL.
type grantHeaders struct{ read, readACP, writeACP, fullControl *string }

// aclGrantHeaders reads the object's ACL so applyUpdate can re-send it: PutObject replaces the ACL
// with the default (owner only), which would silently make a public-read object private. Where the
// ACL cannot be read (ACLs disabled, no s3:GetObjectAcl, a store without ACL support) the object
// is edited as before and the gap is logged; only a cancelled op or an unrepresentable grant fails.
func aclGrantHeaders(ctx context.Context, client *s3.Client, bucket, key string, log func(level, message string)) (grantHeaders, error) {
	acl, err := client.GetObjectAcl(ctx, &s3.GetObjectAclInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		if cancelled := adapters.CheckCancelled(ctx); cancelled != nil {
			return grantHeaders{}, cancelled
		}
		log("warn", "s3: could not read the ACL of s3://"+bucket+"/"+key+", it is not preserved across this edit: "+err.Error())
		return grantHeaders{}, nil
	}
	byPermission := map[types.Permission][]string{}
	for _, g := range acl.Grants {
		if g.Grantee == nil {
			continue
		}
		if g.Grantee.Type == types.TypeCanonicalUser && acl.Owner != nil && g.Grantee.ID != nil && acl.Owner.ID != nil &&
			*g.Grantee.ID == *acl.Owner.ID && g.Permission == types.PermissionFullControl {
			continue
		}
		var grantee string
		switch {
		case g.Grantee.Type == types.TypeCanonicalUser && g.Grantee.ID != nil:
			grantee = `id="` + *g.Grantee.ID + `"`
		case g.Grantee.Type == types.TypeGroup && g.Grantee.URI != nil:
			grantee = `uri="` + *g.Grantee.URI + `"`
		case g.Grantee.Type == types.TypeAmazonCustomerByEmail && g.Grantee.EmailAddress != nil:
			grantee = `emailAddress="` + *g.Grantee.EmailAddress + `"`
		default:
			return grantHeaders{}, adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.s3AclGrant.body", nil, nil)
		}
		switch g.Permission {
		case types.PermissionRead, types.PermissionReadAcp, types.PermissionWriteAcp, types.PermissionFullControl:
			byPermission[g.Permission] = append(byPermission[g.Permission], grantee)
		default:
			return grantHeaders{}, adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.s3AclGrant.body", nil, nil)
		}
	}
	header := func(p types.Permission) *string {
		if len(byPermission[p]) == 0 {
			return nil
		}
		return aws.String(strings.Join(byPermission[p], ","))
	}
	return grantHeaders{
		read: header(types.PermissionRead), readACP: header(types.PermissionReadAcp),
		writeACP: header(types.PermissionWriteAcp), fullControl: header(types.PermissionFullControl),
	}, nil
}

// isConditionalPutUnsupported reports whether err is an S3-compatible endpoint's own signal that it
// doesn't honour IfMatch/IfNoneMatch on PutObject — real for some older S3-compatible object
// stores (pre-conditional-write MinIO/Ceph releases), which reject the header outright with
// NotImplemented rather than evaluating it.
func isConditionalPutUnsupported(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.ErrorCode() == "NotImplemented"
}

// isPreconditionFailed reports whether err is S3's own 412 for a failed IfMatch/IfNoneMatch check.
func isPreconditionFailed(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed"
}

func applyUpdate(ctx context.Context, client *s3.Client, bucket string, op model.MutationRowOp, log func(level, message string)) (int, error) {
	key, err := keyFrom(op.Key, "update")
	if err != nil {
		return 0, err
	}
	value, err := adapters.ValueFrom(op.Changes, "update")
	if err != nil {
		return 0, err
	}
	head, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return 0, mapError(err)
	}
	// This editor has no path to carry object tags across a PutObject replace (which drops
	// them outright) without a second GetObjectTagging/PutObjectTagging round trip — refusing the
	// edit is the more contained fix (over adding that second call's own surface) and never
	// silently drops data the user did not know was there.
	if head.TagCount != nil && *head.TagCount > 0 {
		return 0, adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.s3Tags.body", nil, nil)
	}
	grants, err := aclGrantHeaders(ctx, client, bucket, key, log)
	if err != nil {
		return 0, err
	}
	in := &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key), Body: strings.NewReader(value),
		GrantRead: grants.read, GrantReadACP: grants.readACP, GrantWriteACP: grants.writeACP, GrantFullControl: grants.fullControl,
		// F5: IfMatch closes the lost-update race — a concurrent writer's own change between this
		// HeadObject and the PutObject below is refused rather than silently overwritten.
		IfMatch: head.ETag,
	}
	applyPreservedAttributes(in, head)
	_, err = client.PutObject(ctx, in)
	if err == nil {
		return 1, nil
	}
	if isPreconditionFailed(err) {
		return 0, adapters.T(adapters.CodeQuery, "backend.studio.adapter.s3ObjectChanged.body", nil, nil)
	}
	if isConditionalPutUnsupported(err) {
		log("warn", "s3: endpoint rejected the conditional If-Match update; retrying unconditionally for s3://"+bucket+"/"+key)
		in.IfMatch = nil
		in.Body = strings.NewReader(value)
		if _, err := client.PutObject(ctx, in); err != nil {
			return 0, mapError(err)
		}
		return 1, nil
	}
	return 0, mapError(err)
}

func applyInsert(ctx context.Context, client *s3.Client, bucket string, op model.MutationRowOp, log func(level, message string)) (int, error) {
	key, err := keyFrom(op.Values, "insert")
	if err != nil {
		return 0, err
	}
	sourcePath, err := fileFrom(op.Values)
	if err != nil {
		return 0, err
	}
	body, size, err := openUploadBody(sourcePath)
	if err != nil {
		return 0, err
	}
	defer body.Close()

	in := &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key), Body: body,
		ContentLength: aws.Int64(size), ContentType: contentTypeFrom(op.Values),
		// F5: PutObjectInput.IfNoneMatch("*") is a real conditional-create — it closes the
		// HeadObject-then-Put race the previous "HeadObject first, then Put" shape could not: two
		// concurrent inserts of the same key could both pass the HeadObject check before either
		// one's Put landed.
		IfNoneMatch: aws.String("*"),
	}
	_, err = client.PutObject(ctx, in)
	if err == nil {
		return 1, nil
	}
	if isPreconditionFailed(err) {
		return 0, adapters.New(adapters.CodeQuery, "key already exists: "+key, nil)
	}
	if !isConditionalPutUnsupported(err) {
		return 0, mapError(err)
	}
	// Fallback for an S3-compatible endpoint that rejects the conditional header outright: the
	// pre-F5 HeadObject-then-Put shape, logged so the safety check being skipped is visible.
	log("warn", "s3: endpoint rejected the conditional If-None-Match insert; falling back to a HeadObject-then-Put race for s3://"+bucket+"/"+key)
	_, headErr := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if headErr == nil {
		return 0, adapters.New(adapters.CodeQuery, "key already exists: "+key, nil)
	}
	var notFound *types.NotFound
	if !errors.As(headErr, &notFound) {
		return 0, mapError(headErr)
	}
	if _, err := body.Seek(0, 0); err != nil {
		return 0, adapters.New(adapters.CodeQuery, "could not rewind local file "+sourcePath+": "+err.Error(), err)
	}
	in.IfNoneMatch = nil
	if _, err := client.PutObject(ctx, in); err != nil {
		return 0, mapError(err)
	}
	return 1, nil
}

func applyDelete(ctx context.Context, client *s3.Client, bucket string, op model.MutationRowOp) (int, error) {
	key, err := keyFrom(op.Key, "delete")
	if err != nil {
		return 0, err
	}
	if _, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}); err != nil {
		return 0, mapError(err)
	}
	if _, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}); err != nil {
		return 0, mapError(err)
	}
	return 1, nil
}

// mutate is mutate.ts's mutate.
func mutate(ctx context.Context, client *s3.Client, op *adapters.OpCtx, readOnly bool, scopedBucket string, plan model.MutationPlan, log func(level, message string)) (model.MutationResult, error) {
	if err := adapters.AssertWritable(readOnly); err != nil {
		return model.MutationResult{}, err
	}
	bucket, err := resolveBucketSegment(plan.Path)
	if err != nil {
		return model.MutationResult{}, err
	}
	if err := enforceScope(scopedBucket, bucket); err != nil {
		return model.MutationResult{}, err
	}
	statements, err := preview(plan)
	if err != nil {
		return model.MutationResult{}, err
	}

	return adapters.RunKindDispatched(ctx, op, plan, readOnly, statements, adapters.DispatchUpdateDeleteInsert(
		func(ctx context.Context, rowOp model.MutationRowOp) (int, error) {
			return applyUpdate(ctx, client, bucket, rowOp, log)
		},
		func(ctx context.Context, rowOp model.MutationRowOp) (int, error) {
			return applyDelete(ctx, client, bucket, rowOp)
		},
		func(ctx context.Context, rowOp model.MutationRowOp) (int, error) {
			return applyInsert(ctx, client, bucket, rowOp, log)
		},
	))
}
