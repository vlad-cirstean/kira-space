package sqlb

import (
	"strings"

	"github.com/huandu/go-sqlbuilder"
)

type adeInsertSelect struct {
	d Dialect
	b sqlbuilder.Builder
}

func (s adeInsertSelect) Build() (string, []any) { return s.b.BuildWithFlavor(s.d.fl()) }

// AdeInsertSelect builds INSERT INTO table (cols) <sel> [tail]; sel's arguments keep their order.
// tail is fixed text (e.g. an ON CONFLICT clause) and must not contain $.
func AdeInsertSelect(d Dialect, table string, cols []string, sel *sqlbuilder.SelectBuilder, tail string) Builder {
	format := "INSERT INTO " + table + " (" + strings.Join(cols, ", ") + ") $0"
	if tail != "" {
		format += " " + tail
	}
	return adeInsertSelect{d: d, b: sqlbuilder.Build(format, sel)}
}
