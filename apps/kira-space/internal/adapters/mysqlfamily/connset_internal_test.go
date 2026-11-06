package mysqlfamily

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// White-box (package mysqlfamily) coverage for ConnSet.Acquire's per-connection lock.
// A real dial (BuildConfig -> mysql.NewConnector -> db.Conn) needs a live
// server, so these tests wire a fake Dial that hands back a pre-built connEntry instead, reaching
// Acquire's cache-hit path the same way an already-open connection would, without ever touching the
// network. *sql.Conn is never dereferenced by these tests (no query is ever run on it), so a nil
// one is a faithful enough stand-in for "some already-open connection". The LRU/single-flight
// behaviour of the pool underneath Acquire is covered once, generically, by internal/adapters's
// own connset_test.go — not re-tested per adapter.

func connSetWithFakeEntries(entries map[string]*connEntry) *ConnSet {
	return &ConnSet{inner: adapters.NewConnSet(adapters.ConnSetOptions[string, *connEntry]{
		Dial:    func(_ context.Context, key string) (*connEntry, error) { return entries[key], nil },
		Close:   func(context.Context, *connEntry) {},
		Max:     maxConns,
		Primary: primaryKey,
	})}
}

// TestAcquireSerializesTwoCallersOnTheSameConnection is the regression: before this
// fix, ConnSet.Get handed back a bare *sql.Conn with no lock at all — two concurrent callers (two
// data tabs on the same database, or a Read racing a console Execute) could run statements on the
// same pinned, non-shareable connection at the same time. Acquire must now serialize them: the
// second caller's Acquire must not return until the first caller's release() runs.
func TestAcquireSerializesTwoCallersOnTheSameConnection(t *testing.T) {
	s := connSetWithFakeEntries(map[string]*connEntry{
		primaryKey: {conn: (*sql.Conn)(nil), threadID: 42},
	})
	ctx := context.Background()

	_, release1, err := s.Acquire(ctx, "")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	secondAcquired := make(chan struct{})
	go func() {
		_, release2, err := s.Acquire(ctx, "")
		if err != nil {
			t.Errorf("second Acquire: %v", err)
			close(secondAcquired)
			return
		}
		close(secondAcquired)
		release2()
	}()

	select {
	case <-secondAcquired:
		t.Fatal("second Acquire returned before the first caller's release() ran — the two callers were not serialized")
	case <-time.After(50 * time.Millisecond):
		// Expected: the second Acquire is still blocked on the entry's own lock.
	}

	release1()

	select {
	case <-secondAcquired:
		// The second Acquire unblocked once the first released, as it must.
	case <-time.After(1 * time.Second):
		t.Fatal("second Acquire never unblocked after the first caller's release()")
	}
}

// TestAcquireOnDifferentDatabasesDoesNotSerialize confirms the fix is a per-connection lock, not
// an accidental whole-ConnSet lock: two callers acquiring *different* databases' connections must
// not block each other at all.
func TestAcquireOnDifferentDatabasesDoesNotSerialize(t *testing.T) {
	s := connSetWithFakeEntries(map[string]*connEntry{
		primaryKey: {conn: (*sql.Conn)(nil), threadID: 42},
		"dbtwo":    {conn: (*sql.Conn)(nil), threadID: 43},
	})
	ctx := context.Background()

	_, release1, err := s.Acquire(ctx, "")
	if err != nil {
		t.Fatalf("Acquire(primary): %v", err)
	}
	defer release1()

	done := make(chan struct{})
	go func() {
		_, release2, err := s.Acquire(ctx, "dbtwo")
		if err != nil {
			t.Errorf("Acquire(dbtwo): %v", err)
		} else {
			release2()
		}
		close(done)
	}()

	select {
	case <-done:
		// Expected: a different database's entry acquires immediately.
	case <-time.After(1 * time.Second):
		t.Fatal("Acquire on a different database blocked behind an unrelated connection's lock")
	}
}
