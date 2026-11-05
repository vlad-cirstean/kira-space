package tableref

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

func TestParseFormatRoundTrip(t *testing.T) {
	cases := []struct {
		kind  string
		entry string
		want  Ref
	}{
		{"postgres", "shop.public.orders", Ref{"shop", "public", "orders"}},
		{"postgres", `shop."my.schema"."a""b"`, Ref{"shop", "my.schema", `a"b`}},
		{"mysql", `db." lead"`, Ref{Database: "db", Name: " lead"}},
		{"sqlite", `"a.b"`, Ref{Name: "a.b"}},
		{"kibana", "logs-*", Ref{Name: "logs-*"}},
	}
	for _, c := range cases {
		got, err := Parse(c.kind, c.entry)
		if err != nil || got != c.want {
			t.Fatalf("Parse(%s, %q) = %+v, %v; want %+v", c.kind, c.entry, got, err, c.want)
		}
		if f := Format(c.kind, got); f != c.entry {
			t.Fatalf("Format = %q; want %q", f, c.entry)
		}
	}
}

func TestParseRejects(t *testing.T) {
	bad := []struct{ kind, entry string }{
		{"postgres", "shop.orders"},
		{"mysql", "a.b.c"},
		{"sqlite", "a.b"},
		{"mysql", `db."open`},
		{"mysql", `db."x"y`},
		{"mysql", `db.a"b`},
		{"mysql", "db. a"},
		{"mysql", "db."},
		{"redis", "a"},
	}
	for _, c := range bad {
		if _, err := Parse(c.kind, c.entry); err == nil {
			t.Fatalf("Parse(%s, %q) accepted", c.kind, c.entry)
		}
	}
}

func TestKeyFoldsOnlyForSQLite(t *testing.T) {
	s, err := NewSet("sqlite", []string{"Notes"})
	if err != nil || !s.Allows(Ref{Name: "NOTES"}) || !s.AllowsName("notes") {
		t.Fatalf("sqlite set should fold ASCII case: %v", err)
	}
	s, _ = NewSet("mysql", []string{"db.Notes"})
	if s.Allows(Ref{Database: "db", Name: "notes"}) || !s.Allows(Ref{Database: "db", Name: "Notes"}) {
		t.Fatal("mysql set must match exactly")
	}
	s, _ = NewSet("sqlite", []string{"é"})
	if s.Allows(Ref{Name: "É"}) {
		t.Fatal("non-ASCII must not fold")
	}
}

func TestNewSetNilAndEmpty(t *testing.T) {
	if s, err := NewSet("sqlite", nil); s != nil || err != nil || !s.Allows(Ref{Name: "x"}) {
		t.Fatal("nil entries mean all")
	}
	s, err := NewSet("sqlite", []string{})
	if err != nil || s == nil || s.Allows(Ref{Name: "x"}) {
		t.Fatal("empty list allows nothing")
	}
	if _, err := NewSet("sqlite", []string{"a.b"}); err == nil {
		t.Fatal("bad entry must error")
	}
}

func TestFromPath(t *testing.T) {
	seg := func(kv ...string) []model.PathSegment {
		var out []model.PathSegment
		for i := 0; i < len(kv); i += 2 {
			out = append(out, model.PathSegment{Kind: kv[i], Name: kv[i+1]})
		}
		return out
	}
	cases := []struct {
		kind string
		segs []model.PathSegment
		want Ref
		ok   bool
	}{
		{"postgres", seg("database", "d", "schema", "s", "table", "t"), Ref{"d", "s", "t"}, true},
		{"postgres", seg("database", "d", "schema", "s", "table", "t", "column", "c"), Ref{"d", "s", "t"}, true},
		{"postgres", seg("database", "d", "schema", "s", "function", "f"), Ref{}, false},
		{"postgres", seg("database", "d", "schema", "s"), Ref{}, false},
		{"mysql", seg("database", "d", "view", "v"), Ref{Database: "d", Name: "v"}, true},
		{"clickhouse", seg("database", "d", "matview", "v"), Ref{Database: "d", Name: "v"}, true},
		{"mongodb", seg("database", "d", "collection", "c"), Ref{Database: "d", Name: "c"}, true},
		{"sqlite", seg("database", "main", "table", "t"), Ref{Name: "t"}, true},
		{"sqlite", seg("database", "other", "table", "t"), Ref{}, false},
		{"kibana", seg("collection", "id1"), Ref{Name: "id1"}, true},
		{"redis", seg("database", "0"), Ref{}, false},
	}
	for i, c := range cases {
		got, ok := FromPath(c.kind, c.segs)
		if ok != c.ok || got != c.want {
			t.Fatalf("case %d: got %+v %v; want %+v %v", i, got, ok, c.want, c.ok)
		}
	}
}

func TestColumnEntries(t *testing.T) {
	ok := []struct {
		kind, entry string
		want        ColumnRef
	}{
		{"postgres", `shop.public.t."a.b"`, ColumnRef{Ref{"shop", "public", "t"}, "a.b"}},
		{"mysql", `db."t.x"."c""d"`, ColumnRef{Ref{Database: "db", Name: "t.x"}, `c"d`}},
		{"sqlite", "notes.Body", ColumnRef{Ref{Name: "notes"}, "Body"}},
	}
	for _, c := range ok {
		got, err := ParseColumn(c.kind, c.entry)
		if err != nil || got != c.want {
			t.Fatalf("ParseColumn(%s, %q) = %+v, %v; want %+v", c.kind, c.entry, got, err, c.want)
		}
		if f := FormatColumn(c.kind, got); f != c.entry {
			t.Fatalf("FormatColumn = %q; want %q", f, c.entry)
		}
	}
	for _, c := range []struct{ kind, entry string }{
		{"postgres", "shop.public.t"}, {"mysql", "db.t.c.d"}, {"sqlite", "notes"}, {"redis", "a.b"},
	} {
		if _, err := ParseColumn(c.kind, c.entry); err == nil {
			t.Fatalf("ParseColumn(%s, %q) accepted", c.kind, c.entry)
		}
	}
	a := ColumnRef{Ref{Name: "Notes"}, "Body"}
	if ColumnKey("sqlite", a) != "notes.body" || ColumnKey("mysql", ColumnRef{Ref{Database: "d", Name: "N"}, "B"}) != "d.N.B" {
		t.Fatal("only sqlite column keys fold case")
	}
}
