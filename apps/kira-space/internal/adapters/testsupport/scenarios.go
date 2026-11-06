// scenarios.go is the shared scenario library: the functional assertions genuinely common
// across adapters, once the not-shareable half (a plan's own sentinel vocabulary, dialect SQL, the
// expected code for a permission refusal, read-back after a write) is kept out. Every constructor
// here returns a Scenario whose Run drives an already-connected adapters.Adapter; RunMatrix
// (Tier 2) and RunScenarios (Tier 1) are its only two callers.
package testsupport

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// firstPageCursor is the "offset 0" convention every adapter's own suite already uses for "give me
// whatever the first page looks like", regardless of Caps().Pagination — confirmed against every
// package's own first-page Read call (postgres, mongo, kafka, redis, sqs alike).
func firstPageCursor() model.PageCursor { return model.PageCursor{Mode: "offset", Offset: 0} }

// bigPageSize is large enough that every scratch/seeded object this phase's scenarios touch fits in
// one page — CountMatchesRead and FilterNarrowsResult both read this way rather than walking a real
// pagination loop, which the not-shareable list (§2.2) already rules out as adapter-specific.
const bigPageSize = 1000

// ReadFirstPage asserts Read returns a page with at least one row — page.Page's own Rows() is what
// makes this work across tabular, document, key-value and stream pages alike (page/builder.go).
func ReadFirstPage(path model.NodePath) Scenario {
	return Scenario{
		Name: "read: first page has rows",
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			p, err := a.Read(context.Background(), adapters.ReadRequest{
				Path: path, PageSize: bigPageSize, Cursor: firstPageCursor(),
			}, adapters.NewOpCtx("scenario-read-first-page"))
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if p.Rows() == 0 {
				t.Error("Rows() = 0, want at least one row")
			}
		},
	}
}

// ReadIsRefused asserts Read fails with exactly wantCode.
func ReadIsRefused(path model.NodePath, wantCode adapters.ErrorCode) Scenario {
	return Scenario{
		Name: "read: refused with " + string(wantCode),
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			_, err := a.Read(context.Background(), adapters.ReadRequest{
				Path: path, PageSize: bigPageSize, Cursor: firstPageCursor(),
			}, adapters.NewOpCtx("scenario-read-is-refused"))
			if err == nil {
				t.Fatal("Read: want an error, got nil")
			}
			code, _ := adapters.CodeOf(err)
			if code != wantCode {
				t.Errorf("code = %v, want %v (err: %v)", code, wantCode, err)
			}
		},
	}
}

// CountMatchesRead asserts Count agrees with a single-page Read's own row total — gated on
// c.ExactCount, which auto-skips mongo and sqs (§2.2).
func CountMatchesRead(path model.NodePath) Scenario {
	return Scenario{
		Name:     "count matches a full read",
		Requires: func(c adapters.Caps) bool { return c.ExactCount },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			p, err := a.Read(context.Background(), adapters.ReadRequest{
				Path: path, PageSize: bigPageSize, Cursor: firstPageCursor(),
			}, adapters.NewOpCtx("scenario-count-matches-read"))
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			result, err := a.Count(context.Background(), adapters.CountRequest{Path: path}, adapters.NewOpCtx("scenario-count-matches-count"))
			if err != nil {
				t.Fatalf("Count: %v", err)
			}
			if !result.Exact {
				t.Error("Count: Exact = false, want true (Requires gated on ExactCount)")
			}
			if int64(p.Rows()) != result.Value {
				t.Errorf("Count = %d, want %d (the read's own row count)", result.Value, p.Rows())
			}
		},
	}
}

// FilterNarrowsResult asserts a server-side filter narrows the result to wantRows — gated on
// c.ServerFilter, which auto-skips redis, kafka, sqs, s3 (§2.2). filter is dialect text, a
// parameter never a constant in this library.
func FilterNarrowsResult(path model.NodePath, filter string, wantRows int) Scenario {
	return Scenario{
		Name:     "read: filter narrows the result",
		Requires: func(c adapters.Caps) bool { return c.ServerFilter },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			p, err := a.Read(context.Background(), adapters.ReadRequest{
				Path: path, Filter: &filter, PageSize: bigPageSize, Cursor: firstPageCursor(),
			}, adapters.NewOpCtx("scenario-filter-narrows-result"))
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if p.Rows() != wantRows {
				t.Errorf("Rows() = %d, want %d (filter: %s)", p.Rows(), wantRows, filter)
			}
		},
	}
}

// ProjectionLimitsColumns asserts a column projection is honoured — gated on c.Projection, same
// skip set as FilterNarrowsResult. TabularPage only: every adapter that declares Projection true
// today is SQL-shaped (the not-shareable note keeps document/key-value read-back out of this
// library).
func ProjectionLimitsColumns(path model.NodePath, cols []string) Scenario {
	return Scenario{
		Name:     "read: projection limits the columns",
		Requires: func(c adapters.Caps) bool { return c.Projection },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			p, err := a.Read(context.Background(), adapters.ReadRequest{
				Path: path, Projection: cols, PageSize: bigPageSize, Cursor: firstPageCursor(),
			}, adapters.NewOpCtx("scenario-projection-limits-columns"))
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			tp, ok := p.(page.TabularPage)
			if !ok {
				t.Fatalf("page kind = %T, want a TabularPage", p)
			}
			if len(tp.Columns) != len(cols) {
				t.Fatalf("Columns = %+v, want exactly %v", tp.Columns, cols)
			}
			for i, want := range cols {
				if tp.Columns[i].Name != want {
					t.Errorf("Columns[%d] = %q, want %q", i, tp.Columns[i].Name, want)
				}
			}
		},
	}
}

// MutateSucceeds asserts Mutate commits plan and reports wantAffected — gated on c.Writable.
// Read-back after the write stays in the adapter's own file (§2.2): the page shape to read it back
// with differs by DefaultPageKind, and the right reader is already in scope there.
func MutateSucceeds(plan model.MutationPlan, wantAffected int) Scenario {
	return Scenario{
		Name:     "mutate: succeeds",
		Requires: func(c adapters.Caps) bool { return c.Writable },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			result, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("scenario-mutate-succeeds"))
			if err != nil {
				t.Fatalf("Mutate: %v", err)
			}
			if result.AffectedRows != wantAffected {
				t.Errorf("AffectedRows = %d, want %d", result.AffectedRows, wantAffected)
			}
		},
	}
}

// MutateIsRefused asserts Mutate fails with exactly wantCode — gated on c.Writable. Every
// permission-refusal pin is this constructor attached to a least-privilege matrix row, with a
// comment at the call site naming what the code means today.
func MutateIsRefused(plan model.MutationPlan, wantCode adapters.ErrorCode) Scenario {
	return Scenario{
		Name:     "mutate: refused with " + string(wantCode),
		Requires: func(c adapters.Caps) bool { return c.Writable },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			_, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("scenario-mutate-is-refused"))
			if err == nil {
				t.Fatal("Mutate: want an error, got nil")
			}
			code, _ := adapters.CodeOf(err)
			if code != wantCode {
				t.Errorf("code = %v, want %v (err: %v)", code, wantCode, err)
			}
		},
	}
}

// ExecuteIsRefused asserts Execute fails with exactly wantCode — gated on c.SQL.
func ExecuteIsRefused(path model.NodePath, statements []string, wantCode adapters.ErrorCode) Scenario {
	return Scenario{
		Name:     "execute: refused with " + string(wantCode),
		Requires: func(c adapters.Caps) bool { return c.SQL },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			_, err := a.Execute(context.Background(), model.ConsoleRequest{
				Path: path, Statements: statements,
			}, adapters.NewOpCtx("scenario-execute-is-refused"))
			if err == nil {
				t.Fatal("Execute: want an error, got nil")
			}
			code, _ := adapters.CodeOf(err)
			if code != wantCode {
				t.Errorf("code = %v, want %v (err: %v)", code, wantCode, err)
			}
		},
	}
}

// DownloadRoundTrips asserts DownloadObject streams at least one real byte to a fresh temp file —
// s3 only, automatically, via the c.FileTransfer gate (§2.2).
func DownloadRoundTrips(path model.NodePath) Scenario {
	return Scenario{
		Name:     "downloadObject round-trips",
		Requires: func(c adapters.Caps) bool { return c.FileTransfer },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			destPath := filepath.Join(t.TempDir(), "scenario-download")
			result, err := a.DownloadObject(context.Background(), model.ObjectDownloadRequest{
				Path: path, DestPath: destPath,
			}, adapters.NewOpCtx("scenario-download-round-trips"))
			if err != nil {
				t.Fatalf("DownloadObject: %v", err)
			}
			if result.Bytes == 0 {
				t.Error("Bytes = 0, want at least one byte")
			}
			info, err := os.Stat(destPath)
			if err != nil {
				t.Fatalf("Stat(%s): %v", destPath, err)
			}
			if info.Size() != result.Bytes {
				t.Errorf("file size = %d, want %d (result.Bytes)", info.Size(), result.Bytes)
			}
		},
	}
}

// ReadOnlyRequestRefusesWrite asserts ConsoleRequest.ReadOnly makes Execute reject writeSQL on a
// connection that is not read-only, and still runs a read afterward — gated on c.SQL.
func ReadOnlyRequestRefusesWrite(path model.NodePath, writeSQL, readSQL string) Scenario {
	return Scenario{
		Name:     "execute: per-request read-only refuses a write",
		Requires: func(c adapters.Caps) bool { return c.SQL },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			if _, err := a.Execute(context.Background(), model.ConsoleRequest{
				Path: path, Statements: []string{writeSQL}, ReadOnly: true,
			}, adapters.NewOpCtx("scenario-readonly-request-write")); err == nil {
				t.Fatal("Execute(write, ReadOnly): want an error, got nil")
			}
			if _, err := a.Execute(context.Background(), model.ConsoleRequest{
				Path: path, Statements: []string{readSQL}, ReadOnly: true,
			}, adapters.NewOpCtx("scenario-readonly-request-read")); err != nil {
				t.Fatalf("Execute(read, ReadOnly): %v", err)
			}
		},
	}
}

// ReadOnlyRequestKeepsOpenTransaction asserts a ReadOnly request on a writable connection refuses
// while the shared connection holds a user transaction, and leaves that transaction's write pending
// (rolled back only by the user's own ROLLBACK). setupSQL creates the connection-scoped table
// countSQL counts; beginSQL opens the transaction, writeSQL adds one row. Gated on c.SQL.
func ReadOnlyRequestKeepsOpenTransaction(path model.NodePath, setupSQL, beginSQL, writeSQL, countSQL string) Scenario {
	return Scenario{
		Name:     "execute: per-request read-only refuses inside an open transaction",
		Requires: func(c adapters.Caps) bool { return c.SQL },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			run := func(sql string, readOnly bool) ([]page.Page, error) {
				return a.Execute(context.Background(), model.ConsoleRequest{
					Path: path, Statements: []string{sql}, ReadOnly: readOnly,
				}, adapters.NewOpCtx("scenario-readonly-request-open-tx"))
			}
			count := func() string {
				t.Helper()
				pages, err := run(countSQL, false)
				if err != nil {
					t.Fatalf("Execute(%s): %v", countSQL, err)
				}
				p := pages[0].(page.TabularPage)
				return page.CellText(p.Chunks[0], 0)
			}
			for _, sql := range []string{setupSQL, beginSQL, writeSQL} {
				if _, err := run(sql, false); err != nil {
					t.Fatalf("Execute(%s): %v", sql, err)
				}
			}
			t.Cleanup(func() { _, _ = run("ROLLBACK", false) })
			if _, err := run("SELECT 1", true); err == nil {
				t.Fatal("Execute(read, ReadOnly) inside an open transaction: want an error, got nil")
			}
			if got := count(); got != "1" {
				t.Fatalf("count after refused read = %s, want 1 (transaction write still pending)", got)
			}
			if _, err := run("ROLLBACK", false); err != nil {
				t.Fatalf("ROLLBACK: %v", err)
			}
			if got := count(); got != "0" {
				t.Fatalf("count after ROLLBACK = %s, want 0 (transaction still open for the user)", got)
			}
		},
	}
}

// ReadOnlyRequestAllowsBackslashRead asserts an ordinary read whose text holds a backslash or
// READ WRITE literal passes under ReadOnly on a writable connection — gated on c.SQL.
func ReadOnlyRequestAllowsBackslashRead(path model.NodePath, readSQLs ...string) Scenario {
	return Scenario{
		Name:     "execute: per-request read-only allows reads with backslash literals",
		Requires: func(c adapters.Caps) bool { return c.SQL },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			for _, sql := range readSQLs {
				if _, err := a.Execute(context.Background(), model.ConsoleRequest{
					Path: path, Statements: []string{sql}, ReadOnly: true,
				}, adapters.NewOpCtx("scenario-readonly-request-backslash")); err != nil {
					t.Errorf("Execute(%s, ReadOnly): %v", sql, err)
				}
			}
		},
	}
}
