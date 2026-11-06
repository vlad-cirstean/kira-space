package s3_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// The renderer sends the object's own path for delete and a prefix path for upload; mutate must
// accept both (only the bucket root is checked), and still refuse a path not rooted at a bucket.
func TestS3_Preview_AcceptsPathsBelowBucket(t *testing.T) {
	a := newAdapter(t)
	ops := []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "_key", Value: testsupport.Strp("a/b.txt")}}}}
	bucket := model.PathSegment{Kind: "bucket", Name: "bkt"}
	cases := map[string][]model.PathSegment{
		"bucket": {bucket},
		"prefix": {bucket, {Kind: "prefix", Name: "a/"}},
		"object": {bucket, {Kind: "object", Name: "a/b.txt"}},
	}
	for name, segs := range cases {
		out, err := a.Preview(model.MutationPlan{Path: model.NodePath{ConnectionID: "c", Segments: segs}, Ops: ops})
		if err != nil {
			t.Fatalf("%s path: Preview: %v", name, err)
		}
		if len(out) != 1 || out[0] != "DeleteObject s3://bkt/a/b.txt" {
			t.Errorf("%s path: Preview = %v", name, out)
		}
	}
	rootless := model.NodePath{ConnectionID: "c", Segments: []model.PathSegment{{Kind: "prefix", Name: "a/"}}}
	if _, err := a.Preview(model.MutationPlan{Path: rootless, Ops: ops}); err == nil {
		t.Error("path not rooted at a bucket: want an error")
	}
}
