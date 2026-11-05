package maskreview

import (
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tableref"
)

func pgCol(schema, table, column string) LiveColumn {
	return LiveColumn{Ref: tableref.ColumnRef{Ref: tableref.Ref{Database: "app", Schema: schema, Name: table}, Column: column}}
}

func TestEvaluate(t *testing.T) {
	live := []LiveColumn{
		pgCol("public", "users", "id"),
		pgCol("public", "users", "Email"),
		pgCol("public", "users", "note"),
		pgCol("audit", "log", "note"),
		pgCol("hr", "staff", "dotted.col"),
	}
	allow, err := tableref.NewSet("postgres", []string{"app.public.users"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		rules    []model.MaskRule
		reviewed []string
		tables   *tableref.Set
		want     []string
		names    []string
	}{
		{name: "nothing reviewed", want: []string{"app.audit.log.note", "app.hr.staff.\"dotted.col\"", "app.public.users.Email", "app.public.users.id", "app.public.users.note"}, names: []string{"dotted.col", "email", "id", "note"}},
		{name: "reviewed entries cover exactly their column", reviewed: []string{"app.public.users.id", "app.public.users.Email", "app.public.users.note", "app.hr.staff.\"dotted.col\""}, want: []string{"app.audit.log.note"}, names: []string{"note"}},
		{name: "rule covers by name ignoring case, any table with *", rules: []model.MaskRule{{TableName: "*", ColumnName: "EMAIL"}, {TableName: "*", ColumnName: "note"}}, reviewed: []string{"app.public.users.id", "app.hr.staff.\"dotted.col\""}, want: []string{}, names: []string{}},
		{name: "rule table must match the column's table", rules: []model.MaskRule{{TableName: "users", ColumnName: "note"}}, reviewed: []string{"app.public.users.id", "app.public.users.Email", "app.hr.staff.\"dotted.col\""}, want: []string{"app.audit.log.note"}, names: []string{"note"}},
		{name: "allow list drops other tables", tables: allow, reviewed: []string{"app.public.users.id"}, want: []string{"app.public.users.Email", "app.public.users.note"}, names: []string{"email", "note"}},
		{name: "unparsable reviewed entry is ignored", reviewed: []string{"nope", "app.public.users.id"}, want: []string{"app.audit.log.note", "app.hr.staff.\"dotted.col\"", "app.public.users.Email", "app.public.users.note"}, names: []string{"dotted.col", "email", "note"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate("postgres", live, tc.rules, ReviewedKeys("postgres", tc.reviewed), tc.tables)
			if !slices.Equal(got.Pending, tc.want) {
				t.Fatalf("pending = %q, want %q", got.Pending, tc.want)
			}
			var names []string
			for n := range got.Names {
				names = append(names, n)
			}
			slices.Sort(names)
			if !slices.Equal(names, tc.names) {
				t.Fatalf("names = %q, want %q", names, tc.names)
			}
		})
	}
}

func TestEvaluateSQLiteIgnoresCase(t *testing.T) {
	live := []LiveColumn{{Ref: tableref.ColumnRef{Ref: tableref.Ref{Name: "Notes"}, Column: "Body"}}}
	got := Evaluate("sqlite", live, nil, ReviewedKeys("sqlite", []string{"notes.body"}), nil)
	if len(got.Pending) != 0 {
		t.Fatalf("pending = %q, want none: sqlite names are case-insensitive", got.Pending)
	}
}
