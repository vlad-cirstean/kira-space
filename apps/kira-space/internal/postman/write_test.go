package postman_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/postman"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// TestWriteSkipsGrpcItems is the one postman/write_test.go case: a tree
// containing a gRPC item exports valid v2.1 JSON containing only the HTTP items — Collection
// v2.1 has no representation for a gRPC request, so CollectionsService.Export counts the same
// skip (scanning the tree itself) to report it, rather than this package silently dropping data.
func TestWriteSkipsGrpcItems(t *testing.T) {
	tree := &postman.Tree{
		Name:   "Mixed",
		Origin: map[string]json.RawMessage{},
		Items: []postman.Item{
			{
				Parent: postman.RootParent, Kind: postman.KindRequest, Name: "Get order", Order: 0,
				Protocol: "http",
				Request:  model.SavedRequest{Method: "GET", URL: "https://api.example.com/orders", BodyMode: "none", CodeLanguage: "json"},
				Origin:   map[string]json.RawMessage{},
			},
			{
				Parent: postman.RootParent, Kind: postman.KindRequest, Name: "Echo unary", Order: 1,
				Protocol: "grpc",
				Origin:   map[string]json.RawMessage{},
			},
			{
				Parent: postman.RootParent, Kind: postman.KindRequest, Name: "List widgets", Order: 2,
				Protocol: "http",
				Request:  model.SavedRequest{Method: "GET", URL: "https://api.example.com/widgets", BodyMode: "none", CodeLanguage: "json"},
				Origin:   map[string]json.RawMessage{},
			},
		},
	}

	var buf bytes.Buffer
	if err := postman.Write(&buf, tree); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("the written file is not valid JSON: %v", err)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(doc["item"], &items); err != nil {
		t.Fatalf("item is not an array: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("wrote %d items, want 2 (the gRPC item skipped)", len(items))
	}
	names := map[string]bool{}
	for _, it := range items {
		var name string
		if err := json.Unmarshal(it["name"], &name); err != nil {
			t.Fatalf("item name: %v", err)
		}
		names[name] = true
	}
	if !names["Get order"] || !names["List widgets"] {
		t.Fatalf("wrote items %v, want exactly the two HTTP items", names)
	}
	if names["Echo unary"] {
		t.Fatal("the gRPC item was written — it should have been skipped (F22)")
	}
}
