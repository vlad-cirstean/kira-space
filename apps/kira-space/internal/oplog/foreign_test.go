package oplog

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

type fakeForeign struct {
	mu      sync.Mutex
	version int64
	rows    []Record // index+1 is the rowid
	failed  []string
}

func (f *fakeForeign) DataVersion() (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.version, nil
}
func (f *fakeForeign) MaxRowID() (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.rows)), nil
}
func (f *fakeForeign) ForeignAfter(rowid int64, ids []string) ([]Record, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Record
	top := rowid
	for i, r := range f.rows {
		if int64(i+1) > rowid || slices.Contains(ids, r.ID) {
			out = append(out, r)
			top = max(top, int64(i+1))
		}
	}
	return out, top, nil
}
func (f *fakeForeign) RunningForeignIDs() ([]string, error) { return nil, nil }
func (f *fakeForeign) FailRunning(ids []string, msg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rows {
		if slices.Contains(ids, f.rows[i].ID) && f.rows[i].Status == StatusRunning {
			f.rows[i].Status, f.rows[i].Error = StatusError, &msg
			f.failed = append(f.failed, f.rows[i].ID)
		}
	}
	f.version++
	return nil
}
func (f *fakeForeign) add(r Record) {
	f.mu.Lock()
	f.rows = append(f.rows, r)
	f.version++
	f.mu.Unlock()
}

func frec(id, started, status string) Record {
	return Record{ID: id, StartedAt: started, Status: status, Module: ModuleMemory, Repeat: 1}
}

type spy struct {
	l       *Log
	updates []Record
}

func newSpy(opts ...Option) *spy {
	s := &spy{l: New(opts...)}
	s.l.OnUpdate(func(r Record) { s.updates = append(s.updates, r) })
	return s
}

type countStore struct{ upserts int }

func (c *countStore) Upsert(Record) error            { c.upserts++; return nil }
func (c *countStore) Prune(_ time.Time, _ int) error { return nil }

func (c *countStore) PruneCategory(string, time.Time, int) error { return nil }

func TestIngestMergesByIDAndStartOrder(t *testing.T) {
	s := newSpy()
	s.l.Ingest([]Record{frec("a.1", "2026-01-01T00:00:02.000Z", StatusRunning), frec("b.1", "2026-01-01T00:00:01.000Z", StatusOK)})
	s.l.Ingest([]Record{frec("c.1", "2026-01-01T00:00:03.000Z", StatusOK)})
	var ids []string
	for _, r := range s.l.Recent(10) {
		ids = append(ids, r.ID)
	}
	if want := []string{"c.1", "a.1", "b.1"}; !slices.Equal(ids, want) {
		t.Fatalf("order = %v, want %v", ids, want)
	}
	n := len(s.updates)
	s.l.Ingest([]Record{frec("a.1", "2026-01-01T00:00:02.000Z", StatusRunning)})
	if len(s.updates) != n {
		t.Fatal("an unchanged re-read emitted")
	}
	s.l.Ingest([]Record{frec("a.1", "2026-01-01T00:00:02.000Z", StatusOK)})
	if len(s.updates) != n+1 || len(s.l.Recent(10)) != 3 {
		t.Fatalf("finish: updates %d, rows %d", len(s.updates)-n, len(s.l.Recent(10)))
	}
}

func TestIngestKeepsCapsAndNeverPersists(t *testing.T) {
	cs := &countStore{}
	l := New(WithStore(cs))
	var recs []Record
	for i := range FgCapacity + 5 {
		recs = append(recs, frec(fmt.Sprintf("a.%d", i), fmt.Sprintf("2026-01-01T00:%02d:%02d.000Z", i/60, i%60), StatusOK))
	}
	l.Ingest(recs)
	if got := len(l.Recent(FgCapacity + 10)); got != FgCapacity {
		t.Fatalf("lane len = %d, want %d", got, FgCapacity)
	}
	bg := frec("a.bg", "2026-02-01T00:00:00.000Z", StatusError)
	bg.Background = true
	l.Ingest([]Record{bg})
	l.Close()
	if cs.upserts != 0 {
		t.Fatalf("ingest persisted %d rows", cs.upserts)
	}
}

func TestOwnerLiveness(t *testing.T) {
	dir := t.TempDir() + "/owners"
	o, err := ClaimOwner(dir)
	if err != nil {
		t.Fatal(err)
	}
	id := o.Token() + ".x"
	if tok, ok := OwnerToken(id); !ok || tok != o.Token() {
		t.Fatalf("OwnerToken(%q) = %q, %v", id, tok, ok)
	}
	if _, ok := OwnerToken("plainid"); ok {
		t.Fatal("a local id parsed as foreign")
	}
	if !OwnerAlive(dir, o.Token()) {
		t.Fatal("held lock reads dead")
	}
	o.Release()
	if OwnerAlive(dir, o.Token()) {
		t.Fatal("released lock reads alive")
	}
	if OwnerAlive(dir, "missing") {
		t.Fatal("missing lock reads alive")
	}
}

func TestWatcherMissesNoRowAcrossSeed(t *testing.T) {
	dir := t.TempDir()
	o, err := ClaimOwner(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Release()
	id1, id2 := o.Token()+".1", o.Token()+".2"
	f := &fakeForeign{}
	f.add(frec(id1, "2026-01-01T00:00:01.000Z", StatusOK))
	cur, _ := NewForeignCursor(f)
	f.add(frec(id2, "2026-01-01T00:00:02.000Z", StatusRunning)) // lands between cursor and Seed
	l := New()
	l.Seed([]Record{f.rows[0], f.rows[1]})
	cur = tickForeign(l, f, dir, cur)
	if cur.RowID != 2 {
		t.Fatalf("cursor = %d, want 2", cur.RowID)
	}
	// id2 finishes (same rowid): only the tracked-id re-read sees it.
	f.mu.Lock()
	f.rows[1].Status = StatusOK
	f.version++
	f.mu.Unlock()
	tickForeign(l, f, dir, cur)
	for _, r := range l.Recent(10) {
		if r.ID == id2 && r.Status != StatusOK {
			t.Fatalf("%s stayed %s", id2, r.Status)
		}
	}
}

func TestWatcherFailsOrphanedRow(t *testing.T) {
	dir := t.TempDir()
	o, err := ClaimOwner(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeForeign{}
	cur, _ := NewForeignCursor(f)
	f.add(frec(o.Token()+".1", "2026-01-01T00:00:01.000Z", StatusRunning))
	l := New()
	cur = tickForeign(l, f, dir, cur)
	if len(f.failed) != 0 {
		t.Fatal("live owner's row failed")
	}
	o.Release()
	tickForeign(l, f, dir, cur)
	rec := l.Recent(1)[0]
	if len(f.failed) != 1 || rec.Status != StatusError || derefStr(rec.Error) != ForeignStoppedMsg {
		t.Fatalf("orphan: failed %v, rec %+v", f.failed, rec)
	}
}
