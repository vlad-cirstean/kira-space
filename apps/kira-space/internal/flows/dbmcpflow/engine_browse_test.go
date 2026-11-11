package dbmcpflow

import (
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestDbMcpEngineBrowse is the real-engine half of browse_table's count, in and paging; the sqlite flow
// tests in browse_test.go are the fast half. The seeded customers table holds ids 1 and 2.
func TestDbMcpEngineBrowse(t *testing.T) {
	engineTest(t, []string{"postgres", "mysql", "mariadb", "clickhouse", "sqlite"}, func(t *testing.T, e engine, f *fixture, sess *mcp.ClientSession) {
		connID := f.engineConn(t, e, nil)
		path := e.customersPathOr()
		args := func(extra map[string]any) map[string]any {
			a := map[string]any{"path": path}
			for k, v := range extra {
				a[k] = v
			}
			return a
		}

		seeded := firstCell(t, e.run(t, sess, connID, "select count(*) as n from "+e.customers, "read"))
		if got := decodeCount(t, browse(t, sess, connID, args(map[string]any{"count": true}))); strconv.FormatInt(got.Count, 10) != seeded || !got.Exact {
			t.Fatalf("%s count all = %+v, want %s exact", e.kind, got, seeded)
		}

		in := []map[string]any{idFilter("in", 1, 2, 99)}
		if got := decodeCount(t, browse(t, sess, connID, args(map[string]any{"count": true, "filters": in}))); got.Count != 2 {
			t.Fatalf("%s count in = %+v, want 2", e.kind, got)
		}
		rows := decodeBrowse(t, browse(t, sess, connID, args(map[string]any{"filters": in})))
		if rows.Returned != 2 {
			t.Fatalf("%s rows in = %+v, want 2", e.kind, rows)
		}

		p1 := decodeBrowse(t, browse(t, sess, connID, args(map[string]any{"limit": 1})))
		if !p1.HasMore || p1.NextOffset == nil {
			t.Fatalf("%s page 1 = %+v, want a next page", e.kind, p1)
		}
		p2 := decodeBrowse(t, browse(t, sess, connID, args(map[string]any{"limit": 1, "offset": *p1.NextOffset})))
		a, b := p1.names("id"), p2.names("id")
		if len(a) != 1 || len(b) != 1 || a[0] == b[0] {
			t.Fatalf("%s pages = %v then %v, want two distinct rows", e.kind, a, b)
		}

		if e.kind == "clickhouse" {
			if p1.StableOrder || !strings.Contains(p1.PagingNote, "no primary or unique NOT NULL key") {
				t.Fatalf("clickhouse page = stable %v note %q, want unstable with the no-key note", p1.StableOrder, p1.PagingNote)
			}
			return
		}
		if !p1.StableOrder || p1.PagingNote != "" || len(p1.Order) == 0 {
			t.Fatalf("%s page = stable %v order %v note %q, want a stable order", e.kind, p1.StableOrder, p1.Order, p1.PagingNote)
		}
	})
}
