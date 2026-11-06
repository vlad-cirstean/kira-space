package migrations

import (
	"database/sql"
	"fmt"
)

// Funcs 85 and 87 rebuild the two tables whose shape depends on data: connections and
// ade_repo_config keep their superseded columns only while the config-file export has not run.

func markerSet(tx *sql.Tx, name string) (bool, error) {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM settings WHERE key = ? AND value = 'true'`, "config.migrated."+name).Scan(&n); err != nil {
		return false, fmt.Errorf("read marker %s: %w", name, err)
	}
	return n > 0, nil
}

func hasColumn(tx *sql.Tx, table, column string) (bool, error) {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n); err != nil {
		return false, fmt.Errorf("inspect %s.%s: %w", table, column, err)
	}
	return n > 0, nil
}

// rowCount is the row count of table, 0 when the table is gone.
func rowCount(tx *sql.Tx, table string) (int, error) {
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&exists); err != nil {
		return 0, fmt.Errorf("inspect %s: %w", table, err)
	}
	if exists == 0 {
		return 0, nil
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return n, nil
}

func dropTables(tx *sql.Tx, tables ...string) error {
	for _, t := range tables {
		if _, err := tx.Exec(`DROP TABLE IF EXISTS ` + t); err != nil {
			return fmt.Errorf("drop %s: %w", t, err)
		}
	}
	return nil
}

// p552Connections rebuilds connections. password, uri and mcp_review_results carry over only while the
// connections export (marker config.migrated.connections) has not run and rows exist to export.
func p552Connections(tx *sql.Tx) error {
	done, err := markerSet(tx, "connections")
	if err != nil {
		return err
	}
	rows, err := rowCount(tx, "connections")
	if err != nil {
		return err
	}
	carry := !done && rows > 0
	for _, c := range []string{"password", "uri", "mcp_review_results"} {
		has, err := hasColumn(tx, "connections", c)
		if err != nil {
			return err
		}
		carry = carry && has
	}
	ddl := connectionsPlainSQL
	if carry {
		ddl = connectionsCarrySQL
	}
	if _, err := tx.Exec(ddl); err != nil {
		return fmt.Errorf("rebuild connections: %w", err)
	}
	for _, t := range []string{"connection_tree_filters", "connection_ddl"} {
		n, err := rowCount(tx, t)
		if err != nil {
			return err
		}
		if done || n == 0 {
			if err := dropTables(tx, t); err != nil {
				return err
			}
		}
	}
	return nil
}
