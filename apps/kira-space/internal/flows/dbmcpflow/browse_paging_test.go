package dbmcpflow

import (
	"fmt"
	"strconv"
	"testing"
)

// J-MCP-005: an agent pages browse_table to the end while a second client keeps inserting. Rows
// arrive in key order, so the union holds every pre-existing row once and each page is a window of
// that order. A filter value with a quote and a semicolon is data, not syntax. The offset cap is
// explicit.
func TestDbMcpBrowsePagingUnderInserts(t *testing.T) {
	f := newFixture(t)
	f.execSQL(t, "create table people (id integer primary key, name text, email text, note text)")
	for i := 1; i <= 23; i++ {
		f.execSQL(t, fmt.Sprintf("insert into people (id, name) values (%d, 'p%d')", i, i))
	}
	sess := f.session(t)

	var ids []string
	next, inserted := 0, 23
	for pages := 0; ; pages++ {
		if pages > 20 {
			t.Fatal("paging did not end")
		}
		out := decodeBrowse(t, browse(t, sess, f.connID, map[string]any{"limit": 5, "offset": next}))
		ids = append(ids, out.names("id")...)
		// The second client inserts after every page, always at the end of the key order.
		inserted++
		f.execSQL(t, fmt.Sprintf("insert into people (id, name) values (%d, 'late%d')", inserted, inserted))
		if !out.HasMore {
			break
		}
		next = *out.NextOffset
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("id %s on two pages: %v", id, ids)
		}
		seen[id] = true
	}
	for i := 1; i <= 23; i++ {
		if !seen[strconv.Itoa(i)] {
			t.Fatalf("pre-existing id %d missing from the pages: %v", i, ids)
		}
	}

	// The count matches the table, new rows included.
	if got := decodeCount(t, browse(t, sess, f.connID, map[string]any{"count": true})); got.Count != int64(inserted) {
		t.Fatalf("count = %d, want %d", got.Count, inserted)
	}

	// A quote and a semicolon in a filter value match literally and run nothing else.
	f.execSQL(t, `insert into people (id, name) values (900, 'x'';drop table people;--')`)
	hit := decodeBrowse(t, browse(t, sess, f.connID, map[string]any{"filters": []map[string]any{{"column": "name", "op": "eq", "value": `x';drop table people;--`}}}))
	if got := hit.names("id"); len(got) != 1 || got[0] != "900" {
		t.Fatalf("hostile eq = %v, want the one row", got)
	}
	if got := decodeCount(t, browse(t, sess, f.connID, map[string]any{"count": true})); got.Count != int64(inserted)+1 {
		t.Fatalf("count after the hostile filter = %d, want %d: the table must be intact", got.Count, inserted+1)
	}

	// Past the cap, the refusal says the range.
	wantRefusal(t, browse(t, sess, f.connID, map[string]any{"offset": 100001}), "/properties/offset", "100000")
}
