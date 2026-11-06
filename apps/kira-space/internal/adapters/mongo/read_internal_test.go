package mongo

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// White-box (package mongo) coverage for mergeKeysetIDCondition — task #94 (P2 R2): a scalar _id
// already present in the caller's own filter (a single-document lookup) used to be silently
// dropped once a keyset pagination token was also in play, because the old merge only recognized
// an existing _id value shaped as an operator document (bson.D), not a bare scalar. This exercises
// the merge logic directly against synthetic filters, without a live MongoDB connection.
//
// F6 widened the boundary condition itself (keysetIDCondition) to an $or across every BSON type
// that sorts on the correct side of the boundary's own type — every case below that used to expect
// a bare {_id:{$gt/$lt: boundary}} now expects that widened shape instead, since a string/ObjectId/…
// boundary always has at least one type on either side of it in bsonSortTiers.

var stringAfterTypes = []string{"object", "array", "binData", "objectId", "bool", "date", "timestamp", "regex", "maxKey"}

func TestMergeKeysetIDCondition_NoExistingID(t *testing.T) {
	base := bson.D{{Key: "status", Value: "active"}}
	got := mergeKeysetIDCondition(base, "$gt", "abc123")
	want := bson.D{{Key: "$and", Value: bson.A{
		base,
		bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: "abc123"}}}},
			bson.D{{Key: "_id", Value: bson.D{{Key: "$type", Value: stringAfterTypes}}}},
		}}},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestMergeKeysetIDCondition_ScalarExistingID_NotDropped(t *testing.T) {
	// A single-document lookup by _id, combined with a keyset boundary that never legitimately
	// applies to more than one document at once — the bug this guards against silently discarded
	// this exact value and turned the query into "any document past the boundary".
	base := bson.D{{Key: "_id", Value: "scalar-id-value"}}
	got := mergeKeysetIDCondition(base, "$gt", "boundary-id")

	andValue, ok := lookupField(got, "$and")
	if !ok {
		t.Fatalf("got %+v, want an $and wrapping both the scalar _id and the keyset boundary", got)
	}
	clauses, ok := andValue.(bson.A)
	if !ok || len(clauses) != 2 {
		t.Fatalf("$and = %+v, want a 2-element bson.A", andValue)
	}
	if !reflect.DeepEqual(clauses[0], base) {
		t.Errorf("$and[0] = %+v, want the original scalar _id filter %+v unchanged", clauses[0], base)
	}
	wantIDCond := bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: "boundary-id"}}}},
		bson.D{{Key: "_id", Value: bson.D{{Key: "$type", Value: stringAfterTypes}}}},
	}}}
	if !reflect.DeepEqual(clauses[1], wantIDCond) {
		t.Errorf("$and[1] = %+v, want %+v", clauses[1], wantIDCond)
	}
}

func TestMergeKeysetIDCondition_OperatorDocumentExistingID_BothPreserved(t *testing.T) {
	// The pre-existing (already-working) shape: the user's own filter constrains _id with an
	// operator document of its own (e.g. {"_id": {"$in": [...]}}) — must still keep both
	// constraints, now via $and rather than folding into one _id document.
	existing := bson.D{{Key: "$in", Value: bson.A{"a", "b"}}}
	base := bson.D{{Key: "_id", Value: existing}}
	got := mergeKeysetIDCondition(base, "$lt", "boundary-id")

	andValue, ok := lookupField(got, "$and")
	if !ok {
		t.Fatalf("got %+v, want an $and wrapping both _id conditions", got)
	}
	clauses, ok := andValue.(bson.A)
	if !ok || len(clauses) != 2 {
		t.Fatalf("$and = %+v, want a 2-element bson.A", andValue)
	}
	if !reflect.DeepEqual(clauses[0], base) {
		t.Errorf("$and[0] = %+v, want the original filter %+v unchanged", clauses[0], base)
	}
}

// TestKeysetIDCondition_TypeWidening is the regression: a bare {_id:{$gt/$lt: boundary}} only
// ever matches _id values of the boundary's own BSON type — reproduced here directly against
// keysetIDCondition (not a live server) since this is the boundary-arithmetic decision CLAUDE.md's
// test bar calls out. Confirmed to fail against the pre-fix mergeKeysetIDCondition (which built a
// bare, non-widened condition unconditionally) via a scoped `git stash` before this fix landed.
func TestKeysetIDCondition_TypeWidening(t *testing.T) {
	cases := []struct {
		name      string
		cmpOp     string
		boundary  any
		wantTypes []string
	}{
		{
			name:      "a string boundary going forward ($gt) widens to every type that sorts after string",
			cmpOp:     "$gt",
			boundary:  "abc",
			wantTypes: []string{"object", "array", "binData", "objectId", "bool", "date", "timestamp", "regex", "maxKey"},
		},
		{
			name:      "a string boundary going backward ($lt) widens to every type that sorts before string",
			cmpOp:     "$lt",
			boundary:  "abc",
			wantTypes: []string{"minKey", "null", "double", "int", "long", "decimal"},
		},
		{
			name:      "an ObjectId boundary going forward ($gt) widens to every type that sorts after objectId",
			cmpOp:     "$gt",
			boundary:  bson.NewObjectID(),
			wantTypes: []string{"bool", "date", "timestamp", "regex", "maxKey"},
		},
		{
			name:      "an ObjectId boundary going backward ($lt) widens to every type that sorts before objectId",
			cmpOp:     "$lt",
			boundary:  bson.NewObjectID(),
			wantTypes: []string{"minKey", "null", "double", "int", "long", "decimal", "string", "object", "array", "binData"},
		},
		{
			name:      "a numeric boundary still widens to non-numeric types after it ($gt) — Mongo already compares numeric subtypes by value, but not against string/object/etc",
			cmpOp:     "$gt",
			boundary:  int32(5),
			wantTypes: []string{"string", "object", "array", "binData", "objectId", "bool", "date", "timestamp", "regex", "maxKey"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := keysetIDCondition(c.cmpOp, c.boundary)
			orValue, ok := lookupField(got, "$or")
			if !ok {
				t.Fatalf("keysetIDCondition(%q, %v) = %+v, want a $or-widened condition", c.cmpOp, c.boundary, got)
			}
			clauses, ok := orValue.(bson.A)
			if !ok || len(clauses) != 2 {
				t.Fatalf("$or = %+v, want a 2-element bson.A", orValue)
			}
			typeClause, ok := clauses[1].(bson.D)
			if !ok {
				t.Fatalf("$or[1] = %+v, want a bson.D", clauses[1])
			}
			idValue, ok := lookupField(typeClause, "_id")
			if !ok {
				t.Fatalf("$or[1] = %+v, want an _id key", typeClause)
			}
			typeOp, ok := idValue.(bson.D)
			if !ok {
				t.Fatalf("$or[1]._id = %+v, want a bson.D", idValue)
			}
			gotTypes, ok := lookupField(typeOp, "$type")
			if !ok {
				t.Fatalf("$or[1]._id = %+v, want a $type key", typeOp)
			}
			if !reflect.DeepEqual(gotTypes, c.wantTypes) {
				t.Errorf("$type = %+v, want %+v", gotTypes, c.wantTypes)
			}
		})
	}
}
