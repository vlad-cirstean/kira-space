package adapters

import (
	"strconv"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func rowValues(pairs ...any) model.RowValues {
	var out model.RowValues
	for i := 0; i < len(pairs); i += 2 {
		name := pairs[i].(string)
		var value *string
		if s, ok := pairs[i+1].(string); ok {
			value = &s
		}
		out = append(out, model.RowValue{Name: name, Value: value})
	}
	return out
}

// OrderedOps must be stable: two ops of the same kind keep their plan order (A4's own emphasis —
// sort.SliceStable, not sort.Slice).
func TestOrderedOps_StableWithinKind(t *testing.T) {
	ops := []model.MutationRowOp{
		{Kind: "insert", Values: rowValues("a", "1")},
		{Kind: "delete", Key: rowValues("id", "1")},
		{Kind: "insert", Values: rowValues("a", "2")},
		{Kind: "update", Key: rowValues("id", "2"), Changes: rowValues("a", "3")},
		{Kind: "delete", Key: rowValues("id", "2")},
	}
	got := OrderedOps(ops)
	wantKinds := []string{"delete", "delete", "update", "insert", "insert"}
	for i, k := range wantKinds {
		if got[i].Kind != k {
			t.Fatalf("position %d: got kind %q, want %q", i, got[i].Kind, k)
		}
	}
	// The two deletes and two inserts must keep their original relative order.
	if got[0].Key[0].Value == nil || *got[0].Key[0].Value != "1" {
		t.Errorf("first delete lost its identity: %+v", got[0])
	}
	if got[1].Key[0].Value == nil || *got[1].Key[0].Value != "2" {
		t.Errorf("second delete lost its identity: %+v", got[1])
	}
	if got[3].Values[0].Value == nil || *got[3].Values[0].Value != "1" {
		t.Errorf("first insert lost its identity: %+v", got[3])
	}
	if got[4].Values[0].Value == nil || *got[4].Values[0].Value != "2" {
		t.Errorf("second insert lost its identity: %+v", got[4])
	}
}

// RenderRowOp must emit columns in the wire's own key order (A4) — a map would randomise this.
func TestRenderRowOp_PreservesColumnOrder(t *testing.T) {
	quote := func(s string) string { return `"` + s + `"` }
	render := NewParamRenderer(func(n int) string { return "$" + strconv.FormatInt(int64(n), 10) }, func(string) bool { return false })
	var params []any

	insert := model.MutationRowOp{
		Kind:   "insert",
		Values: rowValues("tenant_id", "3", "entity_id", "1", "name", "new tenant"),
	}
	got, err := RenderRowOp(`"app"."composite_pk"`, insert, render, &params, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `INSERT INTO "app"."composite_pk" ("tenant_id", "entity_id", "name") VALUES ($1, $2, $3)`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	params = nil
	del := model.MutationRowOp{
		Kind: "delete",
		Key:  rowValues("tenant_id", "2", "entity_id", "1"),
	}
	got, err = RenderRowOp(`"app"."composite_pk"`, del, render, &params, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want = `DELETE FROM "app"."composite_pk" WHERE "tenant_id" = $1 AND "entity_id" = $2`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A binary column's edited value, still spelled in the app's own "0x<hex>" display convention,
// must decode to raw bytes rather than reach the driver as its own display text (P2 R1).
func TestNewParamRenderer_DecodesBinaryColumns(t *testing.T) {
	quote := func(s string) string { return `"` + s + `"` }
	isBinary := func(name string) bool { return name == "payload" }
	render := NewParamRenderer(func(n int) string { return "$" + strconv.FormatInt(int64(n), 10) }, isBinary)
	var params []any

	update := model.MutationRowOp{
		Kind:    "update",
		Key:     rowValues("id", "1"),
		Changes: rowValues("payload", "0x4142", "name", "0x4142"),
	}
	got, err := RenderRowOp(`"app"."blobs"`, update, render, &params, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `UPDATE "app"."blobs" SET "payload" = $1, "name" = $2 WHERE "id" = $3`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(params))
	}
	decoded, ok := params[0].([]byte)
	if !ok {
		t.Fatalf("expected the binary column's param to be []byte, got %T", params[0])
	}
	if string(decoded) != "AB" {
		t.Errorf("got decoded bytes %q, want %q", decoded, "AB")
	}
	if _, ok := params[1].(*string); !ok {
		t.Errorf("expected the non-binary column's param to remain *string, got %T", params[1])
	}

	var badParams []any
	badUpdate := model.MutationRowOp{
		Kind:    "update",
		Key:     rowValues("id", "1"),
		Changes: rowValues("payload", "not-hex"),
	}
	if _, err := RenderRowOp(`"app"."blobs"`, badUpdate, render, &badParams, quote); err == nil {
		t.Error("expected an error for a malformed binary value, got nil")
	}
}
