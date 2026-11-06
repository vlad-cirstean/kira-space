package mongo

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// White-box (package mongo) allocation benchmark for the id/body EJSON conversion in readPage's
// row loop. The old path decoded every returned document's wire bytes into a
// full bson.D tree (a reflection-driven per-field type dispatch) only to walk that tree again to
// produce EJSON text; the fix (this package's read.go) keeps each document as bson.Raw — the
// driver's own wire bytes — and lets bson.MarshalExtJSON's raw-value codec stream those bytes
// straight into the EJSON writer, with no intermediate tree at all. No live MongoDB connection is
// needed: both paths start from the same wire bytes, built once via a real bson.Marshal call.
func benchDoc(b *testing.B) bson.Raw {
	b.Helper()
	doc := bson.D{
		{Key: "_id", Value: bson.NewObjectID()},
		{Key: "name", Value: "widget-42"},
		{Key: "price", Value: 19.99},
		{Key: "inStock", Value: true},
		{Key: "tags", Value: bson.A{"a", "b", "c"}},
		{Key: "meta", Value: bson.D{
			{Key: "createdAt", Value: bson.NewDateTimeFromTime(time.Now())},
			{Key: "revision", Value: int32(3)},
		}},
	}
	raw, err := bson.Marshal(doc)
	if err != nil {
		b.Fatalf("bson.Marshal: %v", err)
	}
	return bson.Raw(raw)
}

// BenchmarkEJSON_FromRaw is the fixed path: bson.Raw straight into bson.MarshalExtJSON.
func BenchmarkEJSON_FromRaw(b *testing.B) {
	raw := benchDoc(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ejsonStringify(raw, true); err != nil {
			b.Fatalf("ejsonStringify: %v", err)
		}
	}
}

// BenchmarkEJSON_FromDecodedD is the old path: decode into bson.D first, then marshal that.
func BenchmarkEJSON_FromDecodedD(b *testing.B) {
	raw := benchDoc(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var decoded bson.D
		if err := bson.Unmarshal(raw, &decoded); err != nil {
			b.Fatalf("bson.Unmarshal: %v", err)
		}
		if _, err := ejsonStringify(decoded, true); err != nil {
			b.Fatalf("ejsonStringify: %v", err)
		}
	}
}
