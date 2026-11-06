package configfs

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// TableProjection is a Projection over the config_last_valid table of an app DB, one row per file of
// a folder.
type TableProjection struct {
	DB      *sql.DB
	Folder  string
	Dialect sqlb.Dialect
}

func (t TableProjection) Snapshot() (map[string]string, error) {
	c := kiraschema.ConfigLastValid
	sb := t.Dialect.Select(c.File, c.ValueJSON).From(c.Table)
	sb.Where(sb.Equal(c.Folder, t.Folder))
	rows, err := sqlb.Query(t.DB, sb)
	if err != nil {
		return nil, fmt.Errorf("configfs: query last valid %s: %w", t.Folder, err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var file, js string
		if err := rows.Scan(&file, &js); err != nil {
			return nil, fmt.Errorf("configfs: scan last valid %s: %w", t.Folder, err)
		}
		out[file] = js
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("configfs: last valid rows %s: %w", t.Folder, err)
	}
	return out, nil
}

func (t TableProjection) Record(file, valueJSON string, now int64) error {
	c := kiraschema.ConfigLastValid
	ib := t.Dialect.InsertInto(c.Table).Cols(c.Folder, c.File, c.ValueJSON, c.RecordedAt).Values(t.Folder, file, valueJSON, now)
	sqlb.Upsert(ib, []string{c.Folder, c.File}, []string{c.ValueJSON, c.RecordedAt}, "")
	if _, err := sqlb.Exec(t.DB, ib); err != nil {
		return fmt.Errorf("configfs: record last valid %s/%s: %w", t.Folder, file, err)
	}
	return nil
}

func (t TableProjection) Forget(file string) error {
	c := kiraschema.ConfigLastValid
	db := t.Dialect.DeleteFrom(c.Table)
	db.Where(db.Equal(c.Folder, t.Folder), db.Equal(c.File, file))
	if _, err := sqlb.Exec(t.DB, db); err != nil {
		return fmt.Errorf("configfs: forget last valid %s/%s: %w", t.Folder, file, err)
	}
	return nil
}
