package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// White-box (package sqlite) allocation regression guard for readPage's row-scan hot path.
// readPage used to rebuild the `cells []*string` scratch handed to
// page.TabularPageBuilder.AppendRow with make() on every row, even though AppendRow only ever
// reads through it synchronously and never retains it (unlike `row []any`, which readPage does
// retain via firstRow/lastRow for keyset boundary extraction, and so must stay freshly allocated
// per row). Hoisting it to one allocation for the whole page is still the correct fix, but
// `go build -gcflags=-m` shows the per-row `cells` slice was already stack-allocated even before
// the hoist — AppendRow's `values []*string` parameter is inferred non-escaping across the
// package boundary — so on the current Go toolchain this benchmark's allocs/op is unchanged by
// the fix; it is kept as a guard against that changing (e.g. if AppendRow's signature or
// inlinability changes and `cells` starts escaping) rather than as evidence of a measured win. An
// in-memory database needs no Docker fixture, so this runs as part of the normal unit test suite
// (`go test -bench`) rather than only against a live container.
func BenchmarkReadPage(b *testing.B) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		b.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		b.Fatalf("Conn: %v", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx,
		"CREATE TABLE bench_rows (id INTEGER PRIMARY KEY, a INTEGER, b TEXT, c REAL)"); err != nil {
		b.Fatalf("CREATE TABLE: %v", err)
	}
	const rowCount = 500
	for i := 0; i < rowCount; i++ {
		if _, err := conn.ExecContext(ctx, "INSERT INTO bench_rows (a, b, c) VALUES (?, ?, ?)",
			i, fmt.Sprintf("row-%d", i), float64(i)*1.5); err != nil {
			b.Fatalf("INSERT: %v", err)
		}
	}

	op := adapters.NewOpCtx("bench")
	exec := execFor(ctx, conn, op)
	target, err := getReadTarget(exec, "main", "bench_rows")
	if err != nil {
		b.Fatalf("getReadTarget: %v", err)
	}
	req := readReq{PageSize: rowCount, Cursor: model.PageCursor{Mode: "offset", Offset: 0}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p, err := readPage(ctx, conn, op, target, req)
		if err != nil {
			b.Fatalf("readPage: %v", err)
		}
		if p.RowCount != rowCount {
			b.Fatalf("got %d rows, want %d", p.RowCount, rowCount)
		}
	}
}
