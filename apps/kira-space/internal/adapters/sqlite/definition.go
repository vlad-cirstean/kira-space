package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// buildConstraints is definition.ts's own — SQLite has no CHECK-constraint catalog at
// all; a CHECK is visible only inside the CREATE statement's own text, which the Source pane
// already shows verbatim. Listing PK/UNIQUE/FK here (composed from the same pragmas describe()
// uses) and saying so for CHECK beats an empty Constraints section that looks like there
// are none.
func buildConstraints(exec QueryExecutor, schema, table string, target ReadTarget) ([]model.ConstraintMeta, error) {
	var constraints []model.ConstraintMeta
	if len(target.PrimaryKey) > 0 {
		quoted := make([]string, len(target.PrimaryKey))
		for i, c := range target.PrimaryKey {
			quoted[i] = quoteIdent(c)
		}
		constraints = append(constraints, model.ConstraintMeta{
			Name: table + "_pkey", Type: "primaryKey",
			Definition: "(" + strings.Join(quoted, ", ") + ")",
		})
	}

	indexes, err := listIndexes(exec, schema, table)
	if err != nil {
		return nil, err
	}
	for _, idx := range indexes {
		if idx.Primary || !idx.Unique { // PK covered above; a plain index isn't a constraint
			continue
		}
		quoted := make([]string, len(idx.Columns))
		for i, c := range idx.Columns {
			quoted[i] = quoteIdent(c)
		}
		constraints = append(constraints, model.ConstraintMeta{
			Name: idx.Name, Type: "unique", Definition: "(" + strings.Join(quoted, ", ") + ")",
		})
	}

	fks, err := listForeignKeys(exec, schema, table)
	if err != nil {
		return nil, err
	}
	for _, fk := range fks {
		refPath, err := model.DecodePath("", fk.ReferencedPath)
		refTable := ""
		if err == nil && len(refPath.Segments) > 0 {
			refTable = refPath.Segments[len(refPath.Segments)-1].Name
		}
		quotedCols := make([]string, len(fk.Columns))
		for i, c := range fk.Columns {
			quotedCols[i] = quoteIdent(c)
		}
		quotedRefCols := make([]string, len(fk.ReferencedColumns))
		for i, c := range fk.ReferencedColumns {
			quotedRefCols[i] = quoteIdent(c)
		}
		constraints = append(constraints, model.ConstraintMeta{
			Name: fk.Name, Type: "foreignKey",
			Definition: "(" + strings.Join(quotedCols, ", ") + ") REFERENCES " + quoteIdent(refTable) +
				" (" + strings.Join(quotedRefCols, ", ") + ")",
		})
	}

	return constraints, nil
}

// buildDefinition is definition.ts's own. SHOW CREATE TABLE has no SQLite analogue —
// sqlite_master.sql already *is* the CREATE statement as the user (or a prior migration) wrote it,
// verbatim, so this is "asked, never composed" in its simplest possible form.
func buildDefinition(exec QueryExecutor, segments []model.PathSegment, schema string, kind, name string) (model.ObjectDefinition, error) {
	masterType := "table"
	if kind == "view" {
		masterType = "view"
	}
	// F9: quoted "<schema>".sqlite_master — an unqualified sqlite_master always resolves to "main",
	// which can shadow a same-named table/view in an ATTACHed or TEMP schema.
	var raw sql.NullString
	found := false
	err := exec("SELECT sql FROM "+quoteIdent(schema)+".sqlite_master WHERE type = ? AND name = ?", []any{masterType, name}, func(r *sql.Rows) error {
		found = true
		return r.Scan(&raw)
	})
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	if !found || !raw.Valid || raw.String == "" {
		return model.ObjectDefinition{}, adapters.New(adapters.CodeQuery, `sqlite_master returned no definition for "`+schema+`"."`+name+`"`, nil)
	}
	statement := adapters.StripOneTrailingSemicolon(raw.String)

	var constraints []model.ConstraintMeta
	var notes []string
	if kind == "table" {
		target, err := getReadTarget(exec, schema, name)
		if err != nil {
			return model.ObjectDefinition{}, err
		}
		constraints, err = buildConstraints(exec, schema, name, target)
		if err != nil {
			return model.ObjectDefinition{}, err
		}
		notes = []string{
			"CHECK constraints, if any, appear only in the Source text above — SQLite has no separate " +
				"catalog for them.",
		}
	}

	return model.ObjectDefinition{
		Path: model.EncodePath(segments), Kind: kind, QualifiedName: schema + "." + name,
		Statements: []string{statement}, Language: "sql", Origin: "server", Notes: notes,
		Constraints: constraints, DocumentSchema: nil, Sections: []model.DefinitionSection{},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}
