package dbfold

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// quarantineMigration tags quarantine rows the fold writes: the P552 schema step the rows failed.
const quarantineMigration = 84

// destColumn is one column of a Space table as the STRICT schema declares it.
type destColumn struct {
	name, typ string
	notNull   bool
	dflt      sql.NullString
	check     string
}

var checkRe = regexp.MustCompile(`CHECK \((.*)\)`)

// destColumns reads the declared columns of main.table and the CHECK text of each, which the P552
// schema writes on the column's own line.
func destColumns(t tx, table string) (map[string]destColumn, error) {
	var ddl string
	if err := t.queryRow(`SELECT sql FROM main.sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&ddl); err != nil {
		return nil, fmt.Errorf("read schema of %s: %w", table, err)
	}
	checks := map[string]string{}
	for _, line := range strings.Split(ddl, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if m := checkRe.FindStringSubmatch(line); m != nil {
			checks[strings.Trim(fields[0], `"`)] = strings.TrimSpace(m[1])
		}
	}
	rows, err := t.conn.QueryContext(t.ctx, `SELECT name, type, "notnull", dflt_value FROM pragma_table_info(?, 'main')`, table)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]destColumn{}
	for rows.Next() {
		var c destColumn
		var nn int
		if err := rows.Scan(&c.name, &c.typ, &nn, &c.dflt); err != nil {
			return nil, err
		}
		c.notNull, c.typ, c.check = nn == 1, strings.ToUpper(c.typ), checks[c.name]
		out[c.name] = c
	}
	return out, rows.Err()
}

// typeGuard turns a source value the STRICT column cannot store into NULL, so the row is dropped by
// its NOT NULL or CHECK rule instead of aborting the statement.
func typeGuard(e, typ string) string {
	switch typ {
	case "INTEGER":
		return `CASE typeof(` + e + `) WHEN 'integer' THEN ` + e + `
			WHEN 'real' THEN CASE WHEN ` + e + ` = CAST(` + e + ` AS INTEGER) THEN CAST(` + e + ` AS INTEGER) END
			WHEN 'text' THEN CASE WHEN ` + e + ` GLOB '[0-9]*' AND ` + e + ` NOT GLOB '*[^0-9]*' THEN CAST(` + e + ` AS INTEGER)
				WHEN ` + e + ` GLOB '-[0-9]*' AND substr(` + e + `, 2) NOT GLOB '*[^0-9]*' THEN CAST(` + e + ` AS INTEGER) END
			END`
	case "REAL":
		return `CASE WHEN typeof(` + e + `) IN ('integer', 'real') THEN ` + e + ` END`
	case "TEXT":
		return `CASE WHEN typeof(` + e + `) = 'blob' THEN CAST(` + e + ` AS TEXT) ELSE ` + e + ` END`
	}
	return e
}

var (
	boolCheck = regexp.MustCompile(`^(\w+) IN \(0, 1\)$`)
	enumCheck = regexp.MustCompile(`^(\w+) IN \(('.*')\)$`)
	minCheck  = regexp.MustCompile(`^(\w+) >= (-?\d+)$`)
	minNull   = regexp.MustCompile(`^(\w+) IS NULL OR (\w+) >= (-?\d+)$`)
	rangeNull = regexp.MustCompile(`^(\w+) IS NULL OR (\w+) BETWEEN (-?\d+) AND (-?\d+)$`)
	jsonNull  = regexp.MustCompile(`^(\w+) IS NULL OR json_valid\((\w+)\)$`)
	jsonEmpty = regexp.MustCompile(`^(\w+) = '' OR json_valid\((\w+)\)$`)
	jsonPlain = regexp.MustCompile(`^json_valid\((\w+)\)$`)
)

// repairExpr maps a source value onto the column's CHECK the way the P552 migrations do: booleans
// collapse to 0 or 1, an enum outside its set and an invalid JSON text take the default (or NULL), a
// number below its floor is raised, a port outside its range is cleared. A check of another shape
// is left to the row-level IGNORE.
func repairExpr(c destColumn, e string) string {
	e = typeGuard(e, c.typ)
	dflt := "NULL"
	if c.dflt.Valid {
		dflt = c.dflt.String
	}
	switch ck := c.check; {
	case boolCheck.MatchString(ck):
		if c.notNull {
			return `COALESCE((` + e + `) <> 0, ` + dflt + `)`
		}
		return `((` + e + `) <> 0)`
	case enumCheck.MatchString(ck):
		list := enumCheck.FindStringSubmatch(ck)[2]
		return `CASE WHEN (` + e + `) IN (` + list + `) THEN (` + e + `) ELSE ` + dflt + ` END`
	case minCheck.MatchString(ck):
		floor := minCheck.FindStringSubmatch(ck)[2]
		if c.notNull {
			if !c.dflt.Valid {
				dflt = floor
			}
			return `MAX(` + floor + `, COALESCE(` + e + `, ` + dflt + `))`
		}
		return `MAX(` + floor + `, ` + e + `)`
	case minNull.MatchString(ck):
		return `MAX(` + minNull.FindStringSubmatch(ck)[3] + `, ` + e + `)`
	case rangeNull.MatchString(ck):
		m := rangeNull.FindStringSubmatch(ck)
		return `CASE WHEN (` + e + `) BETWEEN ` + m[3] + ` AND ` + m[4] + ` THEN (` + e + `) END`
	case jsonNull.MatchString(ck):
		return `CASE WHEN (` + e + `) IS NULL OR json_valid(` + e + `) THEN (` + e + `) END`
	case jsonEmpty.MatchString(ck):
		return `CASE WHEN (` + e + `) = '' OR json_valid(` + e + `) THEN (` + e + `) ELSE '' END`
	case jsonPlain.MatchString(ck):
		return `CASE WHEN json_valid(` + e + `) THEN (` + e + `) ELSE ` + dflt + ` END`
	}
	return e
}

// repairList is the source select expression of every shared column: instants converted to unix
// milliseconds, every other value repaired to the destination column's type and CHECK.
func repairList(t tx, dst string, cols []string) (string, error) {
	dest, err := destColumns(t, dst)
	if err != nil {
		return "", err
	}
	conv := map[string]string{}
	for _, c := range instantCols[dst].required {
		conv[c] = instantExpr(c, true)
	}
	for _, c := range instantCols[dst].nullable {
		conv[c] = instantExpr(c, false)
	}
	out := make([]string, len(cols))
	for i, c := range cols {
		if e, ok := conv[c]; ok {
			out[i] = e
		} else {
			out[i] = repairExpr(dest[c], `"`+c+`"`)
		}
	}
	return strings.Join(out, ", "), nil
}

type foreignKey struct {
	from     []string
	nullable bool
}

// foreignKeys lists the foreign keys of main.table by id.
func foreignKeys(t tx, table string) (map[int]*foreignKey, error) {
	nullable := map[string]bool{}
	cols, err := t.conn.QueryContext(t.ctx, `SELECT name, "notnull" FROM pragma_table_info(?, 'main')`, table)
	if err != nil {
		return nil, err
	}
	for cols.Next() {
		var n string
		var nn int
		if err := cols.Scan(&n, &nn); err != nil {
			_ = cols.Close()
			return nil, err
		}
		nullable[n] = nn == 0
	}
	if err := cols.Close(); err != nil {
		return nil, err
	}
	rows, err := t.conn.QueryContext(t.ctx, `SELECT id, "from" FROM pragma_foreign_key_list(?, 'main') ORDER BY id, seq`, table)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int]*foreignKey{}
	for rows.Next() {
		var id int
		var from string
		if err := rows.Scan(&id, &from); err != nil {
			return nil, err
		}
		fk := out[id]
		if fk == nil {
			fk = &foreignKey{nullable: true}
			out[id] = fk
		}
		fk.from = append(fk.from, from)
		fk.nullable = fk.nullable && nullable[from]
	}
	return out, rows.Err()
}

type violation struct {
	table string
	rowid sql.NullInt64
	fkid  int
}

func foreignKeyViolations(t tx) ([]violation, error) {
	rows, err := t.conn.QueryContext(t.ctx, `PRAGMA main.foreign_key_check`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []violation
	for rows.Next() {
		var v violation
		var parent string
		if err := rows.Scan(&v.table, &v.rowid, &parent, &v.fkid); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// sweepOrphans settles every row whose parent the import did not take, before the deferred foreign
// key check runs at COMMIT: a nullable reference is cleared, any other row is removed with the rows
// that depend on it and recorded in schema_quarantine. It repeats until nothing dangles.
func sweepOrphans(t tx, now time.Time) error {
	fks := map[string]map[int]*foreignKey{}
	for {
		vs, err := foreignKeyViolations(t)
		if err != nil {
			return fmt.Errorf("foreign key check: %w", err)
		}
		if len(vs) == 0 {
			return nil
		}
		for _, v := range vs {
			if !v.rowid.Valid {
				return fmt.Errorf("foreign key violation in %s has no rowid", v.table)
			}
			if fks[v.table] == nil {
				if fks[v.table], err = foreignKeys(t, v.table); err != nil {
					return err
				}
			}
			fk := fks[v.table][v.fkid]
			q := `"` + v.table + `"`
			if fk != nil && fk.nullable {
				sets := make([]string, len(fk.from))
				for i, c := range fk.from {
					sets[i] = `"` + c + `" = NULL`
				}
				_, err = t.Exec(`UPDATE main.`+q+` SET `+strings.Join(sets, ", ")+` WHERE rowid = ?`, v.rowid.Int64)
			} else {
				err = removeRow(t, v.table, v.rowid.Int64, now)
			}
			if err != nil {
				return fmt.Errorf("settle %s row: %w", v.table, err)
			}
		}
	}
}

// removeRow records the row in schema_quarantine, then deletes it.
func removeRow(t tx, table string, rowid int64, now time.Time) error {
	cols, err := columns(t, "main", table)
	if err != nil {
		return err
	}
	pairs := make([]string, 0, 2*len(cols))
	for _, c := range cols {
		pairs = append(pairs, `'`+c+`'`, `"`+c+`"`)
	}
	q := `"` + table + `"`
	if _, err := t.Exec(`INSERT INTO main.schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
		SELECT ?, json_object(`+strings.Join(pairs, ", ")+`), 'parent row not imported', 'removed', ?, ?
		FROM main.`+q+` WHERE rowid = ?`, table, quarantineMigration, now.UnixMilli(), rowid); err != nil {
		return err
	}
	_, err = t.Exec(`DELETE FROM main.`+q+` WHERE rowid = ?`, rowid)
	return err
}
