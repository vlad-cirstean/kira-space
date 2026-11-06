// computeOrderBySql fell back to target.SortingKey verbatim, which is "" for an `ORDER BY tuple()`
// MergeTree table and for Log/TinyLog/Memory/Merge engines — readPage still pages with
// LIMIT/OFFSET regardless, so two pages of the same request became two independent unordered scans
// with no ORDER BY at all, silently able to repeat or drop rows across pages even though
// caps.Pagination advertises "offset" pagination unconditionally.
package clickhouse

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func columns(names ...string) []model.ColumnMeta {
	out := make([]model.ColumnMeta, len(names))
	for i, n := range names {
		out[i] = model.ColumnMeta{Name: n, DataType: "String"}
	}
	return out
}

func TestComputeOrderBySql_EmptySortingKeyFallsBackToEveryColumn(t *testing.T) {
	target := ReadTarget{SortingKey: "", Columns: columns("id", "ts", "payload")}
	got, err := computeOrderBySql(nil, target)
	if err != nil {
		t.Fatalf("computeOrderBySql: %v", err)
	}
	want := "`id` ASC, `ts` ASC, `payload` ASC"
	if got != want {
		t.Errorf("got %q, want %q (a deterministic order, not silence)", got, want)
	}
}

func TestComputeOrderBySql_EmptySortingKeyAndNoColumnsStaysEmpty(t *testing.T) {
	target := ReadTarget{SortingKey: "", Columns: nil}
	got, err := computeOrderBySql(nil, target)
	if err != nil {
		t.Fatalf("computeOrderBySql: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty (nothing to order by)", got)
	}
}

func TestComputeOrderBySql_RealSortingKeyIsUsedVerbatim(t *testing.T) {
	target := ReadTarget{SortingKey: "toYYYYMM(ts), id", Columns: columns("id", "ts")}
	got, err := computeOrderBySql(nil, target)
	if err != nil {
		t.Fatalf("computeOrderBySql: %v", err)
	}
	if got != "toYYYYMM(ts), id" {
		t.Errorf("got %q, want the real sorting key verbatim, not the column fallback", got)
	}
}

func TestComputeOrderBySql_ExplicitSortStillTakesPrecedenceOverTheFallback(t *testing.T) {
	target := ReadTarget{SortingKey: "", Columns: columns("id", "ts")}
	sort := &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "ts", Direction: "desc"}}}
	got, err := computeOrderBySql(sort, target)
	if err != nil {
		t.Fatalf("computeOrderBySql: %v", err)
	}
	if got != "`ts` DESC" {
		t.Errorf("got %q, want the caller's own explicit sort", got)
	}
}
