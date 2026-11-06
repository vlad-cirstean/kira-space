package maskreview

import (
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
)

func liveCol(kind, table, col string) LiveColumn {
	return LiveColumn{Ref: tableref.ColumnRef{Ref: tableref.Ref{Name: table}, Column: col}}
}

func TestKnownUnsettled(t *testing.T) {
	customers, users := tableref.Ref{Name: "customers"}, tableref.Ref{Name: "users"}
	live := []LiveColumn{liveCol("postgres", "customers", "email"), liveCol("postgres", "customers", "ssn"), liveCol("postgres", "users", "id")}
	cases := []struct {
		name   string
		kind   string
		tables []tableref.Ref
		parsed bool
		names  []string
		want   []string
	}{
		{"column of the statement's table", "postgres", []tableref.Ref{customers}, true, []string{"email"}, nil},
		{"name known only on another table", "postgres", []tableref.Ref{users}, true, []string{"email"}, []string{"email"}},
		{"postgres columns differing in case differ", "postgres", []tableref.Ref{customers}, true, []string{"SSN"}, []string{"SSN"}},
		{"mysql folds case", "mysql", []tableref.Ref{customers}, true, []string{"SSN"}, nil},
		{"no relation settles every name", "postgres", nil, true, []string{"1"}, nil},
		{"unparsed falls back to every relation", "postgres", nil, false, []string{"email", "x"}, []string{"x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			k := NewKnown(c.kind, live)
			if got := k.Unsettled(c.tables, c.parsed, c.names); !slices.Equal(got, c.want) {
				t.Fatalf("Unsettled = %v, want %v", got, c.want)
			}
		})
	}
}

func TestKnownRemembersAliasPerScope(t *testing.T) {
	customers, users := tableref.Ref{Name: "customers"}, tableref.Ref{Name: "users"}
	k := NewKnown("postgres", []LiveColumn{liveCol("postgres", "customers", "id"), liveCol("postgres", "users", "id")})
	req := Rescan{Tables: []tableref.Ref{customers}, Parsed: true, Names: []string{"total"}}
	k = k.carry(nil, req)
	if got := k.Unsettled(req.Tables, true, req.Names); got != nil {
		t.Fatalf("remembered alias still unsettled: %v", got)
	}
	if got := k.Unsettled([]tableref.Ref{users}, true, req.Names); len(got) != 1 {
		t.Fatalf("alias from another statement scope must refresh: %v", got)
	}
}
