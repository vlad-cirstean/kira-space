// sort_internal_test.go: listTablesAndViews' sort used to be a hand-rolled O(n²) insertion sort,
// justified only by "sort.SliceStable would need the `sort`
// import" — already wrong, since sqlite/catalog.go imports it for exactly this. Rewritten to
// sort.SliceStable over a precomputed lowercased-name slice; this pins the rewrite as behavior-
// preserving (tables before views/matviews, then a case-insensitive name compare, stable on ties).
package clickhouse

import (
	"sort"
	"testing"
)

func sortedNames(rows []systemTableRow) []string {
	lower := make([]string, len(rows))
	for i, r := range rows {
		lower[i] = toLowerASCII(r.Name)
	}
	idx := make([]int, len(rows))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return lessTableIdx(rows, lower, idx[i], idx[j]) })
	out := make([]string, len(idx))
	for i, id := range idx {
		out[i] = rows[id].Name
	}
	return out
}

// toLowerASCII avoids importing strings just for this test file's own helper — the production
// path already lowercases via strings.ToLower in listTablesAndViews.
func toLowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

func TestSortedTables_TablesBeforeViewsThenCaseInsensitiveName(t *testing.T) {
	rows := []systemTableRow{
		{Name: "zebra_view", Engine: "View"},
		{Name: "Order Items", Engine: "MergeTree"},
		{Name: "big_rows", Engine: "MergeTree"},
		{Name: "a_matview", Engine: "MaterializedView"},
		{Name: "another_view", Engine: "View"},
	}
	got := sortedNames(rows)
	want := []string{"big_rows", "Order Items", "a_matview", "another_view", "zebra_view"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v (mismatch at index %d)", got, want, i)
			break
		}
	}
}

func TestSortedTables_StableOnEqualKeys(t *testing.T) {
	// Two tables that compare equal under the case-insensitive name rule (same name, different
	// case) must keep their original relative order — sort.SliceStable's own contract, which the
	// old insertion sort also happened to honour.
	rows := []systemTableRow{
		{Name: "Users", Engine: "MergeTree"},
		{Name: "users", Engine: "MergeTree"},
	}
	got := sortedNames(rows)
	want := []string{"Users", "users"}
	if got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v (stability broken)", got, want)
	}
}

func TestSortedTables_EmptyAndSingleton(t *testing.T) {
	if got := sortedNames(nil); len(got) != 0 {
		t.Errorf("empty input: got %v, want []", got)
	}
	got := sortedNames([]systemTableRow{{Name: "solo", Engine: "MergeTree"}})
	if len(got) != 1 || got[0] != "solo" {
		t.Errorf("singleton: got %v, want [solo]", got)
	}
}
