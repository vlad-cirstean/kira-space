package s3

import (
	"context"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// maxListRounds mirrors catalog.ts's MAX_LIST_ROUNDS — never an unbudgeted listing.
const maxListRounds = 20

// listBuckets is catalog.ts's listBuckets. scopedBucket (options.bucket) is the escape hatch for
// a very common IAM shape: credentials scoped to exactly one bucket, which commonly deny
// s3:ListAllMyBuckets outright. HeadBucket only needs access to that one bucket, so a scoped
// connection never calls ListBuckets at all.
func listBuckets(ctx context.Context, client *s3.Client, scopedBucket string) ([]model.TreeNode, error) {
	if scopedBucket != "" {
		if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(scopedBucket)}); err != nil {
			return nil, mapError(err)
		}
		return []model.TreeNode{{
			Kind: "bucket", Name: scopedBucket,
			Path:        model.EncodePath([]model.PathSegment{{Kind: "bucket", Name: scopedBucket}}),
			HasChildren: false, // a bucket's prefix/object space is unbounded — the tree stops here
		}}, nil
	}

	result, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, mapError(err)
	}
	nodes := make([]model.TreeNode, 0, len(result.Buckets))
	for _, b := range result.Buckets {
		if b.Name == nil {
			continue
		}
		nodes = append(nodes, model.TreeNode{
			Kind: "bucket", Name: *b.Name,
			Path:        model.EncodePath([]model.PathSegment{{Kind: "bucket", Name: *b.Name}}),
			HasChildren: false,
		})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	return nodes, nil
}

// prefixLister is the one *s3.Client method listPrefixChildren needs (P58f D14) — declared here so
// catalog_test.go can drive the truncation conjunction with a fake instead of a live server.
type prefixLister interface {
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// listPrefixChildren is catalog.ts's listPrefixChildren: a prefix tree from ListObjectsV2 with
// Delimiter — CommonPrefixes are the "folders" one level down, Contents (minus the exact-prefix-
// match "directory marker" object some tools create) are the leaf objects at this level.
func listPrefixChildren(ctx context.Context, client prefixLister, bucket string, prefixSegments []string, op *adapters.OpCtx) (adapters.TreeChildren, error) {
	prefix := ""
	if len(prefixSegments) > 0 {
		prefix = strings.Join(prefixSegments, "/") + "/"
	}
	ancestor := make([]model.PathSegment, 0, len(prefixSegments)+1)
	ancestor = append(ancestor, model.PathSegment{Kind: "bucket", Name: bucket})
	for _, s := range prefixSegments {
		ancestor = append(ancestor, model.PathSegment{Kind: "prefix", Name: s})
	}

	var prefixNodes, objectNodes []model.TreeNode
	var continuationToken *string
	rounds := 0

	for {
		if err := adapters.CheckCancelled(ctx); err != nil {
			return adapters.TreeChildren{}, err
		}
		result, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket: aws.String(bucket), Prefix: aws.String(prefix), Delimiter: aws.String("/"),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return adapters.TreeChildren{}, mapError(err)
		}

		for _, cp := range result.CommonPrefixes {
			if cp.Prefix == nil {
				continue
			}
			// "a/b/" -> local segment "b" (strip the parent prefix and the trailing delimiter).
			segment := (*cp.Prefix)[len(prefix) : len(*cp.Prefix)-1]
			path := append(append([]model.PathSegment{}, ancestor...), model.PathSegment{Kind: "prefix", Name: segment})
			prefixNodes = append(prefixNodes, model.TreeNode{
				Kind: "prefix", Name: segment, Path: model.EncodePath(path), HasChildren: true,
			})
		}
		for _, obj := range result.Contents {
			if obj.Key == nil || *obj.Key == prefix {
				continue // skip the exact-prefix "directory marker"
			}
			path := append(append([]model.PathSegment{}, ancestor...), model.PathSegment{Kind: "object", Name: *obj.Key})
			objectNodes = append(objectNodes, model.TreeNode{
				// The full key verbatim, not just this local segment — same split as redis's
				// namespace/key distinction: every intermediate "folder" node carries its own
				// segment, but the leaf carries the identifier a consumer actually needs.
				Kind: "object", Name: *obj.Key, Path: model.EncodePath(path), HasChildren: false,
			})
		}

		continuationToken = result.NextContinuationToken
		rounds++
		if continuationToken == nil || rounds >= maxListRounds {
			break
		}
	}

	sort.Slice(prefixNodes, func(i, j int) bool { return prefixNodes[i].Name < prefixNodes[j].Name })
	sort.Slice(objectNodes, func(i, j int) bool { return objectNodes[i].Name < objectNodes[j].Name })
	prefixNodes = append(prefixNodes, objectNodes...)

	// P43 iter2 F16/D21: true only when the round cap cut the listing short — never for an
	// ordinary complete listing that happened to take fewer rounds.
	if continuationToken != nil && rounds >= maxListRounds {
		truncated := true
		return adapters.TreeChildren{Nodes: prefixNodes, Truncated: &truncated}, nil
	}
	return adapters.TreeChildren{Nodes: prefixNodes}, nil
}
