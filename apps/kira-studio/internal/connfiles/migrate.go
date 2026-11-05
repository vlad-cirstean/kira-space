package connfiles

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// migrate exports the rows that predate the files: it binds each row to a stem, copies sealed password and
// URI envelopes into the secret store as they are, writes one file per connection, seeds that row's
// config_json only after its file exists, and drops the superseded columns and tables. A row without
// config_json is never projected, so a failed write cannot cost a row. A crash before the marker reruns it
// with the stored stems.
func (l *Loader) migrate() error {
	files, err := l.bindLegacyRows()
	if err != nil {
		return err
	}
	for _, f := range files {
		path := filepath.Join(l.o.Dir, f.name)
		if _, err := os.Stat(path); err != nil {
			if len(f.text) > configfs.MaxFileBytes {
				slog.Warn("connections: migrated file is over the size cap and stays invalid until edited", "scope", "startup", "file", f.name)
			}
			if err := configfs.WriteAtomic(path, f.text, 0o644); err != nil {
				return err
			}
		}
		if _, err := l.o.DB.Exec(`UPDATE connections SET config_json = ? WHERE id = ? AND config_json = ''`, f.json, f.id); err != nil {
			return fmt.Errorf("connfiles: seed %s: %w", f.name, err)
		}
	}
	return l.dropLegacy()
}

// legacyOut is one migrated connection: its file text and the config_json to seed once the file exists.
type legacyOut struct {
	id, name, json string
	text           []byte
}

func (l *Loader) hasColumn(table, column string) (bool, error) {
	rows, err := l.o.DB.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, fmt.Errorf("connfiles: inspect %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return false, err
		}
		if n == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (l *Loader) hasTable(table string) (bool, error) {
	var n int
	err := l.o.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n)
	return n > 0, err
}

type legacyConn struct {
	id, stem string
	fields   model.ConnectionFields
	password sql.NullString
	uri      sql.NullString
}

func (l *Loader) legacyRows(tx *sql.Tx, withSecrets bool) ([]legacyConn, error) {
	secretCols := `NULL, NULL`
	if withSecrets {
		secretCols = `password, uri`
	}
	rows, err := tx.Query(`SELECT id, COALESCE(stem, ''), name, kind, color, mode, read_only, host, port, database, username,
		options_json, preconnect, preconnect_sidecar, preconnect_delay_sec, auto_explain, throttle_per_sec, mcp_enabled,
		mcp_description, mcp_read_mode, mcp_write_mode, mcp_ddl_mode, mcp_auto_explain, mcp_review_results, ` + secretCols + `
		FROM connections ORDER BY sort_order, created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("connfiles: read connections: %w", err)
	}
	defer rows.Close()
	var out []legacyConn
	for rows.Next() {
		var c legacyConn
		var ro, side, auto, mcp, mcpAuto int
		var opts sql.NullString
		var legacyReview string
		f := &c.fields
		if err := rows.Scan(&c.id, &c.stem, &f.Name, &f.Kind, &f.Color, &f.Mode, &ro, &f.Host, &f.Port, &f.Database, &f.Username,
			&opts, &f.Preconnect, &side, &f.PreconnectDelaySec, &auto, &f.ThrottlePerSec, &mcp,
			&f.McpDescription, &f.McpReadMode, &f.McpWriteMode, &f.McpDdlMode, &mcpAuto, &legacyReview,
			&c.password, &c.uri); err != nil {
			return nil, fmt.Errorf("connfiles: scan connection: %w", err)
		}
		f.McpReview = model.McpReview{Read: legacyReview, Write: legacyReview, Ddl: legacyReview}
		f.McpStatementTimeoutSec = model.McpStatementTimeoutDefault // the column never existed on a legacy row
		f.ReadOnly, f.PreconnectSidecar, f.AutoExplain, f.McpEnabled, f.McpAutoExplain = ro != 0, side != 0, auto != 0, mcp != 0, mcpAuto != 0
		f.Options = map[string]any{}
		if opts.Valid && opts.String != "" {
			_ = json.Unmarshal([]byte(opts.String), &f.Options)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (l *Loader) bindLegacyRows() ([]legacyOut, error) {
	hasPassword, err := l.hasColumn("connections", "password")
	if err != nil {
		return nil, err
	}
	hasFilters, err := l.hasTable("connection_tree_filters")
	if err != nil {
		return nil, err
	}
	hasDDL, err := l.hasTable("connection_ddl")
	if err != nil {
		return nil, err
	}
	tx, err := l.o.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("connfiles: begin migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	conns, err := l.legacyRows(tx, hasPassword)
	if err != nil {
		return nil, err
	}
	taken := map[string]bool{}
	for _, c := range conns {
		if c.stem != "" {
			taken[c.stem] = true
		}
	}
	var files []legacyOut
	for _, c := range conns {
		if c.stem == "" {
			c.stem = freeBase(Slug(c.fields.Name), func(s string) bool {
				if taken[s] {
					return true
				}
				_, err := os.Stat(filepath.Join(l.o.Dir, FileName(s)))
				return err == nil
			})
			taken[c.stem] = true
			if _, err := tx.Exec(`UPDATE connections SET stem = ? WHERE id = ?`, c.stem, c.id); err != nil {
				return nil, fmt.Errorf("connfiles: bind connection: %w", err)
			}
		}
		text, js, err := legacyFile(tx, c, hasFilters, hasDDL)
		if err != nil {
			return nil, err
		}
		files = append(files, legacyOut{id: c.id, name: FileName(c.stem), json: js, text: text})
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("connfiles: commit migration: %w", err)
	}
	return files, nil
}

// legacyFile moves one connection's secrets, masks, tree filters and DDL into its file text. It also
// returns the config_json to seed after the file is written.
func legacyFile(tx *sql.Tx, c legacyConn, hasFilters, hasDDL bool) ([]byte, string, error) {
	var passRef, uriRef string
	var err error
	if c.password.Valid {
		if passRef, err = copyEnvelope(tx, secrets.ScopeConnection, c.stem+".password", c.password.String); err != nil {
			return nil, "", err
		}
	}
	if c.uri.Valid {
		if uriRef, err = copyEnvelope(tx, secrets.ScopeConnectionURI, c.stem+".uri", c.uri.String); err != nil {
			return nil, "", err
		}
	}
	masks, err := legacyMasks(tx, c.id)
	if err != nil {
		return nil, "", err
	}
	tree := model.EmptyVisibility()
	if hasFilters {
		if tree, err = legacyTree(tx, c.id); err != nil {
			return nil, "", err
		}
	}
	var ddl string
	if hasDDL {
		if err := tx.QueryRow(`SELECT COALESCE((SELECT ddl FROM connection_ddl WHERE connection_id = ?), '')`, c.id).Scan(&ddl); err != nil {
			return nil, "", fmt.Errorf("connfiles: read ddl: %w", err)
		}
	}
	f := FileOf(c.fields, masks, tree, ddl, passRef, uriRef)
	js, err := json.Marshal(f)
	if err != nil {
		return nil, "", fmt.Errorf("connfiles: encode %s: %w", c.stem, err)
	}
	text, err := configfs.RenderYAML(f, Header)
	return text, string(js), err
}

// copyEnvelope stores a sealed envelope as it is under name and returns the file reference. No decrypt: the
// scope is the envelope's authenticated data and stays the one it was sealed with.
func copyEnvelope(tx *sql.Tx, scope secrets.Scope, name, envelope string) (string, error) {
	if _, err := tx.Exec(`INSERT INTO secret_values (scope, owner, name, value, updated_ms) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (scope, owner, name) DO NOTHING`, string(scope), SecretOwner, name, envelope, time.Now().UnixMilli()); err != nil {
		return "", fmt.Errorf("connfiles: move secret: %w", err)
	}
	return RefOf(name), nil
}

func legacyMasks(tx *sql.Tx, id string) ([]model.MaskRuleFields, error) {
	rows, err := tx.Query(`SELECT table_name, column_name, mask_kind, keep_hint, correlate FROM connection_mask_rules
		WHERE connection_id = ? ORDER BY created_at, id`, id)
	if err != nil {
		return nil, fmt.Errorf("connfiles: read masks: %w", err)
	}
	defer rows.Close()
	var out []model.MaskRuleFields
	for rows.Next() {
		var m model.MaskRuleFields
		var kind string
		var hint, corr int
		if err := rows.Scan(&m.TableName, &m.ColumnName, &kind, &hint, &corr); err != nil {
			return nil, err
		}
		m.Kind, m.KeepHint, m.Correlate = model.MaskKind(kind), hint != 0, corr != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

func legacyTree(tx *sql.Tx, id string) (model.TreeVisibility, error) {
	rows, err := tx.Query(`SELECT scope, value FROM connection_tree_filters WHERE connection_id = ? ORDER BY scope, value`, id)
	if err != nil {
		return model.TreeVisibility{}, fmt.Errorf("connfiles: read tree filters: %w", err)
	}
	defer rows.Close()
	v := model.EmptyVisibility()
	for rows.Next() {
		var scope, value string
		if err := rows.Scan(&scope, &value); err != nil {
			return model.TreeVisibility{}, err
		}
		switch scope {
		case "kind":
			v.HiddenKinds = append(v.HiddenKinds, value)
		case "path":
			v.HiddenPaths = append(v.HiddenPaths, value)
		}
	}
	return v, rows.Err()
}

// dropLegacy removes what the files replaced. Each step skips what is already gone, so a rerun is safe.
func (l *Loader) dropLegacy() error {
	for _, col := range []string{"password", "uri"} {
		has, err := l.hasColumn("connections", col)
		if err != nil {
			return err
		}
		if has {
			if _, err := l.o.DB.Exec(`ALTER TABLE connections DROP COLUMN ` + col); err != nil {
				return fmt.Errorf("connfiles: drop column %s: %w", col, err)
			}
		}
	}
	for _, t := range []string{"connection_tree_filters", "connection_ddl"} {
		if _, err := l.o.DB.Exec(`DROP TABLE IF EXISTS ` + t); err != nil {
			return fmt.Errorf("connfiles: drop table %s: %w", t, err)
		}
	}
	return nil
}

// freeBase returns base, or base-2, base-3 ... the first not taken.
func freeBase(base string, taken func(string) bool) string {
	if !taken(base) {
		return base
	}
	for n := 2; ; n++ {
		suffix := fmt.Sprintf("-%d", n)
		head := base
		if len(head)+len(suffix) > 60 {
			head = strings.Trim(head[:60-len(suffix)], "-")
		}
		if cand := head + suffix; !taken(cand) {
			return cand
		}
	}
}
