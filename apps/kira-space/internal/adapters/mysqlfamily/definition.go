package mysqlfamily

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

var constraintTypeName = map[string]string{
	"PRIMARY KEY": "primaryKey", "UNIQUE": "unique", "FOREIGN KEY": "foreignKey", "CHECK": "check",
}

type constraintRow struct {
	name, ctype string
	checkClause *string
}

type keyColumnRow struct {
	name, col        string
	refTable, refCol *string
}

// fetchConstraintRows is listConstraints's first query: the table's own constraints and, for a
// CHECK constraint, its clause text.
func fetchConstraintRows(ctx context.Context, exec queryExec, database, table string) ([]constraintRow, error) {
	var constraints []constraintRow
	err := exec(ctx, `SELECT tc.CONSTRAINT_NAME AS name, tc.CONSTRAINT_TYPE AS type, cc.CHECK_CLAUSE AS check_clause
	 FROM information_schema.TABLE_CONSTRAINTS tc
	 LEFT JOIN information_schema.CHECK_CONSTRAINTS cc
	   ON cc.CONSTRAINT_SCHEMA = tc.CONSTRAINT_SCHEMA AND cc.CONSTRAINT_NAME = tc.CONSTRAINT_NAME
	 WHERE tc.TABLE_SCHEMA = ? AND tc.TABLE_NAME = ?
	 ORDER BY FIELD(tc.CONSTRAINT_TYPE, 'PRIMARY KEY', 'UNIQUE', 'CHECK', 'FOREIGN KEY'),
	          tc.CONSTRAINT_NAME`, []any{database, table}, func(rows *sql.Rows) error {
		var r constraintRow
		if err := rows.Scan(&r.name, &r.ctype, &r.checkClause); err != nil {
			return err
		}
		constraints = append(constraints, r)
		return nil
	})
	return constraints, err
}

// fetchKeyColumnRows is listConstraints's second query: each key-bearing constraint's columns
// (and, for a foreign key, the referenced table/columns).
func fetchKeyColumnRows(ctx context.Context, exec queryExec, database, table string) ([]keyColumnRow, error) {
	var keyColumns []keyColumnRow
	err := exec(ctx, `SELECT CONSTRAINT_NAME AS name, COLUMN_NAME AS col,
	        REFERENCED_TABLE_NAME AS ref_table, REFERENCED_COLUMN_NAME AS ref_col
	 FROM information_schema.KEY_COLUMN_USAGE
	 WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
	 ORDER BY CONSTRAINT_NAME, ORDINAL_POSITION`, []any{database, table}, func(rows *sql.Rows) error {
		var r keyColumnRow
		if err := rows.Scan(&r.name, &r.col, &r.refTable, &r.refCol); err != nil {
			return err
		}
		keyColumns = append(keyColumns, r)
		return nil
	})
	return keyColumns, err
}

// constraintMetaFor composes one constraint's ConstraintMeta from its row and its (possibly
// empty) key columns — a CHECK constraint's clause text, else a column list, plus a foreign key's
// REFERENCES clause.
func constraintMetaFor(c constraintRow, cols []keyColumnRow) model.ConstraintMeta {
	typ, ok := constraintTypeName[c.ctype]
	if !ok {
		typ = "check"
	}
	if typ == "check" {
		def := ""
		if c.checkClause != nil {
			def = *c.checkClause
		}
		return model.ConstraintMeta{Name: c.name, Type: typ, Definition: def}
	}
	colNames := make([]string, len(cols))
	for j, r := range cols {
		colNames[j] = r.col
	}
	columnList := "(" + strings.Join(colNames, ", ") + ")"
	if typ != "foreignKey" {
		return model.ConstraintMeta{Name: c.name, Type: typ, Definition: columnList}
	}
	refTable := ""
	if len(cols) > 0 && cols[0].refTable != nil {
		refTable = *cols[0].refTable
	}
	refCols := make([]string, len(cols))
	for j, r := range cols {
		if r.refCol != nil {
			refCols[j] = *r.refCol
		}
	}
	return model.ConstraintMeta{
		Name: c.name, Type: typ,
		Definition: columnList + " REFERENCES " + refTable + " (" + strings.Join(refCols, ", ") + ")",
	}
}

// listConstraints is definition.ts's listConstraints. MariaDB/MySQL have no
// pg_get_constraintdef-style builtin (P19 D11) — the key-column list and the FK's referenced
// table/columns are composed from information_schema itself.
func listConstraints(ctx context.Context, exec queryExec, database, table string) ([]model.ConstraintMeta, error) {
	constraints, err := fetchConstraintRows(ctx, exec, database, table)
	if err != nil {
		return nil, err
	}
	if len(constraints) == 0 {
		return nil, nil
	}

	keyColumns, err := fetchKeyColumnRows(ctx, exec, database, table)
	if err != nil {
		return nil, err
	}
	columnsByConstraint := map[string][]keyColumnRow{}
	for _, r := range keyColumns {
		columnsByConstraint[r.name] = append(columnsByConstraint[r.name], r)
	}

	metas := make([]model.ConstraintMeta, len(constraints))
	for i, c := range constraints {
		metas[i] = constraintMetaFor(c, columnsByConstraint[c.name])
	}
	return metas, nil
}

// buildDefinition is definition.ts's buildDefinition: passes SHOW CREATE … through verbatim —
// MariaDB/MySQL are asked, never composed.
func buildDefinition(ctx context.Context, exec queryExec, segments []model.PathSegment, database string, objectKind, objectName string) (model.ObjectDefinition, error) {
	var tableType string
	found := false
	err := exec(ctx, `SELECT TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`,
		[]any{database, objectName}, func(rows *sql.Rows) error {
			found = true
			return rows.Scan(&tableType)
		})
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	if !found {
		return model.ObjectDefinition{}, adapters.New(adapters.CodeNotFound, "relation \""+database+"\".\""+objectName+"\" not found", nil)
	}

	qualified := quoteIdent(database) + "." + quoteIdent(objectName)
	var statement string
	var notes []string
	var constraintMetas []model.ConstraintMeta

	switch tableType {
	case "BASE TABLE":
		raw, err := showCreate(ctx, exec, "SHOW CREATE TABLE "+qualified, "Create Table")
		if err != nil {
			return model.ObjectDefinition{}, err
		}
		if raw == "" {
			return model.ObjectDefinition{}, adapters.New(adapters.CodeQuery, "SHOW CREATE TABLE returned no definition for \""+database+"\".\""+objectName+"\"", nil)
		}
		statement = adapters.StripOneTrailingSemicolon(raw)
		notes = []string{"Triggers and grants are not included in SHOW CREATE TABLE."}
		constraintMetas, err = listConstraints(ctx, exec, database, objectName)
		if err != nil {
			return model.ObjectDefinition{}, err
		}
	case "VIEW":
		raw, err := showCreate(ctx, exec, "SHOW CREATE VIEW "+qualified, "Create View")
		if err != nil {
			return model.ObjectDefinition{}, err
		}
		if raw == "" {
			return model.ObjectDefinition{}, adapters.New(adapters.CodeQuery, "SHOW CREATE VIEW returned no definition for \""+database+"\".\""+objectName+"\"", nil)
		}
		statement = adapters.StripOneTrailingSemicolon(raw)
		notes = []string{"This is the server's own SHOW CREATE VIEW text, including its DEFINER and SQL SECURITY clauses."}
	default:
		return model.ObjectDefinition{}, adapters.New(adapters.CodeUnsupported, "definition is not supported for "+tableType, nil)
	}

	return model.ObjectDefinition{
		Path: model.EncodePath(segments), Kind: objectKind, QualifiedName: database + "." + objectName,
		Statements: []string{statement}, Language: "sql", Origin: "server", Notes: notes,
		Constraints: constraintMetas, DocumentSchema: nil, Sections: []model.DefinitionSection{},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

// showCreate runs a SHOW CREATE … statement and returns its named result column (e.g. "Create
// Table") as a string — go's database/sql scans by position, so this reads every column into
// sql.RawBytes and picks the one whose name matches.
func showCreate(ctx context.Context, exec queryExec, statement, columnName string) (string, error) {
	var out string
	err := exec(ctx, statement, nil, func(rows *sql.Rows) error {
		cols, err := rows.Columns()
		if err != nil {
			return err
		}
		raw := make([]sql.RawBytes, len(cols))
		dest := make([]any, len(cols))
		for i := range raw {
			dest[i] = &raw[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		for i, c := range cols {
			if c == columnName {
				out = string(raw[i])
			}
		}
		return nil
	})
	return out, err
}
