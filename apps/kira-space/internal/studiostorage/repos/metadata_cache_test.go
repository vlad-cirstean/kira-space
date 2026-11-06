package repos_test

import (
	"encoding/json"
	"fmt"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"testing"
)

// TestMetadataCacheDifferentKindsShareOneRow pins Put's merge semantics: the unique index is
// (connection_id, path) and `kind` is a key INSIDE the stored payload object, so caching a
// 'describe' for a path must merge into that path's existing row rather than overwrite the
// 'children' already there. A plain upsert silently destroys the other kind.
func TestMetadataCacheDifferentKindsShareOneRow(t *testing.T) {
	r := newMetadataCacheRepo(t)
	seedConnection(t, r.DB, "c1")

	if err := r.Put("c1", "db:t", "children", json.RawMessage(`["a"]`)); err != nil {
		t.Fatalf("Put children: %v", err)
	}
	if err := r.Put("c1", "db:t", "describe", json.RawMessage(`{"cols":1}`)); err != nil {
		t.Fatalf("Put describe: %v", err)
	}

	children, _, err := r.Get("c1", "db:t", "children")
	if err != nil {
		t.Fatalf("Get children: %v", err)
	}
	describe, _, err := r.Get("c1", "db:t", "describe")
	if err != nil {
		t.Fatalf("Get describe: %v", err)
	}
	if string(children) != `["a"]` || string(describe) != `{"cols":1}` {
		t.Errorf("children=%s describe=%s, want both readable from the same row", children, describe)
	}

	var rowCount int
	if err := r.DB.QueryRow(`SELECT COUNT(*) FROM metadata_cache WHERE connection_id = 'c1' AND path = 'db:t'`).Scan(&rowCount); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("row count for one path with two kinds = %d, want 1", rowCount)
	}
}

// TestMetadataCacheEvictionKeepsNewestAndIsolatesConnections covers the per-connection eviction
// pass: the 200-row cap is partitioned BY connection (one busy connection must not evict
// another's rows) and the rows kept are the newest by (fetched_at, rowid).
func TestMetadataCacheEvictionKeepsNewestAndIsolatesConnections(t *testing.T) {
	r := newMetadataCacheRepo(t)
	seedConnection(t, r.DB, "c1")
	seedConnection(t, r.DB, "c2")

	if err := r.Put("c2", "db:other", "children", json.RawMessage(`["untouched"]`)); err != nil {
		t.Fatalf("Put c2: %v", err)
	}

	for i := 0; i < 205; i++ {
		path := fmt.Sprintf("db:t%d", i)
		if err := r.Put("c1", path, "children", json.RawMessage(`[]`)); err != nil {
			t.Fatalf("Put c1 %d: %v", i, err)
		}
	}

	var c1Count int
	if err := r.DB.QueryRow(`SELECT COUNT(*) FROM metadata_cache WHERE connection_id = 'c1'`).Scan(&c1Count); err != nil {
		t.Fatalf("count c1: %v", err)
	}
	if c1Count != 200 {
		t.Errorf("c1 row count after 205 puts = %d, want 200 (evicted to cap)", c1Count)
	}

	newest, _, err := r.Get("c1", "db:t204", "children")
	if err != nil {
		t.Fatalf("Get newest: %v", err)
	}
	if newest == nil {
		t.Error("newest path (db:t204) was evicted, want it to survive")
	}

	untouched, _, err := r.Get("c2", "db:other", "children")
	if err != nil {
		t.Fatalf("Get c2: %v", err)
	}
	if string(untouched) != `["untouched"]` {
		t.Errorf("c2's row was affected by c1's eviction: %s", untouched)
	}
}

// TestMetadataCachePerKindFetchedAtIsIndependent pins the hazard directly at the repo
// layer: a row-level fetched_at cannot express "this kind's payload is old, that kind's is new."
// A 'children' payload written long ago and a 'columns' payload written just now, sharing one row,
// must keep two independent fetchedAt entries — writing one must never move the other's.
func TestMetadataCachePerKindFetchedAtIsIndependent(t *testing.T) {
	r := newMetadataCacheRepo(t)
	seedConnection(t, r.DB, "c1")

	// Seed 'children' directly with an old fetchedAt, simulating a payload written minutes ago —
	// Put() itself always stamps "now", so a real time gap can only be constructed this way.
	oldFetchedAt := "2020-01-01T00:00:00.000Z"
	oldPayload := fmt.Sprintf(`{"children":["a"],"fetchedAt":{"children":%q}}`, oldFetchedAt)
	if _, err := r.DB.Exec(
		`INSERT INTO metadata_cache (connection_id, path, kind, payload_json, fetched_at, etag)
		 VALUES ('c1', 'db:t', 'children', ?, ?, NULL)`,
		oldPayload, oldFetchedAt,
	); err != nil {
		t.Fatalf("seed old children row: %v", err)
	}

	if err := r.Put("c1", "db:t", "columns", json.RawMessage(`[{"name":"x"}]`)); err != nil {
		t.Fatalf("Put columns: %v", err)
	}

	children, childrenAt, err := r.Get("c1", "db:t", "children")
	if err != nil {
		t.Fatalf("Get children: %v", err)
	}
	if string(children) != `["a"]` {
		t.Errorf("children payload = %s, want unchanged by the columns write", children)
	}
	if childrenAt != oldFetchedAt {
		t.Errorf("children fetchedAt = %q, want unchanged old value %q (writing columns must not move it)", childrenAt, oldFetchedAt)
	}

	columns, columnsAt, err := r.Get("c1", "db:t", "columns")
	if err != nil {
		t.Fatalf("Get columns: %v", err)
	}
	if string(columns) != `[{"name":"x"}]` {
		t.Errorf("columns payload = %s, want the just-written value", columns)
	}
	if columnsAt == "" || columnsAt == oldFetchedAt {
		t.Errorf("columns fetchedAt = %q, want a fresh timestamp distinct from children's", columnsAt)
	}

	// Now write 'children' again — its own fetchedAt must move, but 'columns' must stay exactly
	// where it was, proving the independence holds in both directions.
	if err := r.Put("c1", "db:t", "children", json.RawMessage(`["b"]`)); err != nil {
		t.Fatalf("Put children again: %v", err)
	}
	_, childrenAt2, err := r.Get("c1", "db:t", "children")
	if err != nil {
		t.Fatalf("Get children (after rewrite): %v", err)
	}
	if childrenAt2 == oldFetchedAt {
		t.Errorf("children fetchedAt did not move after Put, still %q", childrenAt2)
	}
	_, columnsAt2, err := r.Get("c1", "db:t", "columns")
	if err != nil {
		t.Fatalf("Get columns (after children rewrite): %v", err)
	}
	if columnsAt2 != columnsAt {
		t.Errorf("columns fetchedAt moved from %q to %q after writing an unrelated kind", columnsAt, columnsAt2)
	}
}

// TestMetadataCacheLegacyRowFetchedAtIsEmpty covers the upgrade path: a row (payload_json with
// no reserved fetchedAt key at all) must read back with fetchedAt == "" rather than erroring —
// the empty string is what internal/tree.Service's freshness comparison treats as older than any
// real timestamp.
func TestMetadataCacheLegacyRowFetchedAtIsEmpty(t *testing.T) {
	r := newMetadataCacheRepo(t)
	seedConnection(t, r.DB, "c1")

	if _, err := r.DB.Exec(
		`INSERT INTO metadata_cache (connection_id, path, kind, payload_json, fetched_at, etag)
		 VALUES ('c1', 'db:legacy', 'children', '{"children":["a"]}', ?, NULL)`,
		kiratime.NowISO(),
	); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	payload, fetchedAt, err := r.Get("c1", "db:legacy", "children")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(payload) != `["a"]` {
		t.Errorf("payload = %s, want the legacy row's own value", payload)
	}
	if fetchedAt != "" {
		t.Errorf("fetchedAt = %q, want \"\" for a pre-P24 row with no fetchedAt key", fetchedAt)
	}
}
