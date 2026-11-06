package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// TestBuildReadPage_MissingID is the regression: a document with no _id at all (reachable via
// a $project: {_id: 0} view, which this app lists and reads elsewhere) used to fail the whole page
// outright — doc.Lookup("_id") returns an invalid BSON type IDText can't stringify. Reproduced here
// directly against buildReadPage (no live server needed).
func TestBuildReadPage_MissingID(t *testing.T) {
	withID, err := bson.Marshal(bson.D{{Key: "_id", Value: bson.NewObjectID()}, {Key: "name", Value: "a"}})
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}
	withoutID, err := bson.Marshal(bson.D{{Key: "name", Value: "b"}}) // a $project: {_id: 0} view row
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}
	docs := []bson.Raw{bson.Raw(withID), bson.Raw(withoutID)}

	req := readReq{PageSize: 10, Cursor: model.PageCursor{Mode: "after"}}
	plan := readSortPlan{idOnlySort: true, wantsKeyset: true, scanDirection: 1}

	pg, err := buildReadPage(req, plan, docs, "fp")
	if err != nil {
		t.Fatalf("buildReadPage returned an error instead of tolerating a missing _id: %v", err)
	}
	if pg.RowCount != 2 {
		t.Fatalf("RowCount = %d, want 2", pg.RowCount)
	}
	// F7: no real _id boundary exists across this result set, so the page falls back to plain
	// offset pagination rather than emitting a keyset token it cannot correctly resume from.
	if pg.Position.Strategy != "offset" {
		t.Errorf("Strategy = %q, want %q (a missing _id disables keyset pagination for this page)", pg.Position.Strategy, "offset")
	}
	if pg.Position.NextToken != nil {
		t.Errorf("NextToken = %v, want nil (no keyset boundary to resume from)", *pg.Position.NextToken)
	}
}

// TestBuildReadPage_AllPresentID is the control case: every document does carry an _id, so the
// normal keyset strategy still applies exactly as before.
func TestBuildReadPage_AllPresentID(t *testing.T) {
	doc1, err := bson.Marshal(bson.D{{Key: "_id", Value: bson.NewObjectID()}, {Key: "name", Value: "a"}})
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}
	doc2, err := bson.Marshal(bson.D{{Key: "_id", Value: bson.NewObjectID()}, {Key: "name", Value: "b"}})
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}
	docs := []bson.Raw{bson.Raw(doc1), bson.Raw(doc2)}

	req := readReq{PageSize: 10, Cursor: model.PageCursor{Mode: "after"}}
	plan := readSortPlan{idOnlySort: true, wantsKeyset: true, scanDirection: 1}

	pg, err := buildReadPage(req, plan, docs, "fp")
	if err != nil {
		t.Fatalf("buildReadPage: %v", err)
	}
	if pg.Position.Strategy != "keyset" {
		t.Errorf("Strategy = %q, want %q", pg.Position.Strategy, "keyset")
	}
}
