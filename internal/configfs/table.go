package configfs

import (
	"database/sql"
	"fmt"
)

// TableProjection is a Projection over the config_last_valid table of an app DB, one row per file of
// a folder.
type TableProjection struct {
	DB     *sql.DB
	Folder string
}

func (t TableProjection) Snapshot() (map[string]string, error) {
	rows, err := t.DB.Query(`SELECT file, value_json FROM config_last_valid WHERE folder = ?`, t.Folder)
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
	if _, err := t.DB.Exec(`INSERT INTO config_last_valid (folder, file, value_json, recorded_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(folder, file) DO UPDATE SET value_json = excluded.value_json, recorded_at = excluded.recorded_at`,
		t.Folder, file, valueJSON, now); err != nil {
		return fmt.Errorf("configfs: record last valid %s/%s: %w", t.Folder, file, err)
	}
	return nil
}

func (t TableProjection) Forget(file string) error {
	if _, err := t.DB.Exec(`DELETE FROM config_last_valid WHERE folder = ? AND file = ?`, t.Folder, file); err != nil {
		return fmt.Errorf("configfs: forget last valid %s/%s: %w", t.Folder, file, err)
	}
	return nil
}
