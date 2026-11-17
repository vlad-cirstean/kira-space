package dbflow

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// P662 backend half: a console result is bounded (rows, bytes, per-cell bytes) and one slow data op
// never blocks the session. tests/ui/studio/console-large-result.spec.ts copies these shapes.

// 12,000 rows of one 80,000-char cell: past the row cap, and past MaxCellBytes per cell, so the byte
// budget stops the page after about 56 MiB / 64 KiB rows.
func TestConsoleLargeResultCapped(t *testing.T) {
	app := flowharness.New(t)
	path := filepath.Join(t.TempDir(), "large.db")
	sqliteExec(t, path, "create table seed (id integer)")
	conn := createConn(t, app, sqliteInput("large", path))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)

	pages, err := ds.Execute(conn.ID, "", `with recursive n(i) as (select 1 union all select i + 1 from n where i < 12000)
select i, hex(zeroblob(40000)) as body from n`)
	if err != nil || len(pages) != 1 {
		t.Fatalf("Execute = %d pages, %v", len(pages), err)
	}
	p := pages[0]
	if !p.PosTruncated {
		t.Fatalf("page of %d rows is not reported truncated", p.RowCount)
	}
	if p.RowCount == 0 || p.RowCount > adapters.DefaultConsoleCap.Rows || p.RowCount >= 12000 {
		t.Fatalf("RowCount = %d, want 0 < rows < 12000 and <= %d", p.RowCount, adapters.DefaultConsoleCap.Rows)
	}
	t.Logf("capped page: %d rows", p.RowCount)
	total := 0
	for i, row := range p.Rows {
		body, _ := row[1].(string)
		if len(body) > page.MaxCellBytes {
			t.Fatalf("row %d cell is %d bytes, want <= %d", i, len(body), page.MaxCellBytes)
		}
		if !p.Truncated[i] {
			t.Fatalf("row %d: an 80,000-char cell is not flagged truncated", i)
		}
		total += len(body)
	}
	if total > adapters.DefaultConsoleCap.Bytes {
		t.Fatalf("page holds %d cell bytes, want <= %d", total, adapters.DefaultConsoleCap.Bytes)
	}
}

// A slow statement runs on its own op: a read on a second connection still answers, Cancel reaches
// the statement, and the execute ends E_CANCELLED.
func TestConsoleSlowQueryResponsiveAndCancellable(t *testing.T) {
	app := flowharness.New(t)
	path := filepath.Join(t.TempDir(), "slow.db")
	sqliteExec(t, path, "create table items (id integer primary key, label text)", "insert into items (label) values ('a'), ('b')")
	slow := createConn(t, app, sqliteInput("slow", path))
	other := createConn(t, app, sqliteInput("other", path))
	connect(t, app, slow.ID)
	connect(t, app, other.ID)
	ds := app.DataStream(t)

	opID, wait := ds.ExecuteFire(slow.ID, "", `with recursive c(x) as (select 1 union all select x + 1 from c where x < 1000000000) select count(*) from c`)

	read := make(chan error, 1)
	go func() {
		_, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: other.ID, Path: tableNodePath(t, app, other.ID, "items"), PageSize: 10})
		read <- err
	}()
	select {
	case err := <-read:
		if err != nil {
			t.Fatalf("Read beside a running statement: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Read did not answer within 5s while a statement ran")
	}

	if ok, err := app.W.Router.Cancel(ctx, opID); err != nil || !ok {
		t.Fatalf("Cancel(%s) = %v, %v", opID, ok, err)
	}
	done := make(chan error, 1)
	go func() { _, err := wait(); done <- err }()
	select {
	case err := <-done:
		var derr *flowharness.DataError
		if !errors.As(err, &derr) || derr.Code != "E_CANCELLED" {
			t.Fatalf("cancelled execute: err = %v, want E_CANCELLED", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled execute did not end within 5s")
	}
}
