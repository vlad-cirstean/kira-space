package redis_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// A namespaced-key delete sends the key's own path below the database; mutate must accept it (only
// the database root is checked) and still refuse a path not rooted at a database.
func TestRedis_Preview_AcceptsPathsBelowDatabase(t *testing.T) {
	a := newAdapter(t)
	ops := []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "_key", Value: strp("ns:k")}}}}
	db := model.PathSegment{Kind: "database", Name: "db1"}
	for name, segs := range map[string][]model.PathSegment{
		"database": {db},
		"key":      {db, {Kind: "key", Name: "ns:k"}},
	} {
		out, err := a.Preview(model.MutationPlan{Path: model.NodePath{ConnectionID: "c", Segments: segs}, Ops: ops})
		if err != nil {
			t.Fatalf("%s path: Preview: %v", name, err)
		}
		if len(out) != 1 {
			t.Errorf("%s path: Preview = %v", name, out)
		}
	}
	rootless := model.NodePath{ConnectionID: "c", Segments: []model.PathSegment{{Kind: "key", Name: "ns:k"}}}
	if _, err := a.Preview(model.MutationPlan{Path: rootless, Ops: ops}); err == nil {
		t.Error("path not rooted at a database: want an error")
	}
}
