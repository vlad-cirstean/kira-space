package connfiles

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// Change is one connection a load or a write made stale. Old is nil for a new connection and New is
// nil for a removed one.
type Change struct {
	ID       string
	Old, New *File
}

// Masks reports whether the mask rules differ between the two sides.
func (c Change) Masks() bool { return !sameJSON(masksOf(c.Old), masksOf(c.New)) }

// DDL reports whether the schema DDL differs.
func (c Change) DDL() bool { return ddlOf(c.Old) != ddlOf(c.New) }

// Tree reports whether the left panel visibility differs.
func (c Change) Tree() bool { return !sameJSON(treeOf(c.Old), treeOf(c.New)) }

func masksOf(f *File) []Mask {
	if f == nil {
		return nil
	}
	return f.Masks
}

func ddlOf(f *File) string {
	if f == nil {
		return ""
	}
	return f.SchemaDDL
}

func treeOf(f *File) model.TreeVisibility {
	if f == nil {
		return model.EmptyVisibility()
	}
	return f.Visibility()
}

func sameJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

// projection keeps the connections table and the mask rule rows in step with the files.
type projection struct{ l *Loader }

func (p *projection) Snapshot() (map[string]string, error) {
	rows, err := p.l.o.DB.Query(`SELECT stem, config_json FROM connections WHERE stem IS NOT NULL AND config_json <> ''`)
	if err != nil {
		return nil, fmt.Errorf("connfiles: query last valid: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var stem, js string
		if err := rows.Scan(&stem, &js); err != nil {
			return nil, fmt.Errorf("connfiles: scan last valid: %w", err)
		}
		out[FileName(stem)] = js
	}
	return out, rows.Err()
}

func (p *projection) Record(file, valueJSON string, _ int64) error {
	stem, ok := StemOf(file)
	if !ok {
		return nil
	}
	var f File
	if err := json.Unmarshal([]byte(valueJSON), &f); err != nil {
		return fmt.Errorf("connfiles: decode %s: %w", file, err)
	}
	tx, err := p.l.o.DB.Begin()
	if err != nil {
		return fmt.Errorf("connfiles: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	id, old, err := p.l.upsert(tx, stem, f, valueJSON)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("connfiles: commit: %w", err)
	}
	p.l.pend = append(p.l.pend, Change{ID: id, Old: old, New: &f})
	return nil
}

func (p *projection) Forget(file string) error {
	stem, ok := StemOf(file)
	if !ok {
		return nil
	}
	var id, js string
	err := p.l.o.DB.QueryRow(`SELECT id, config_json FROM connections WHERE stem = ?`, stem).Scan(&id, &js)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("connfiles: read %s: %w", stem, err)
	}
	if p.l.o.OnForget != nil {
		p.l.o.OnForget(id)
	}
	if _, err := p.l.o.DB.Exec(`DELETE FROM connections WHERE id = ?`, id); err != nil {
		return fmt.Errorf("connfiles: delete %s: %w", stem, err)
	}
	var old File
	if json.Unmarshal([]byte(js), &old) == nil {
		p.l.pend = append(p.l.pend, Change{ID: id, Old: &old})
	} else {
		p.l.pend = append(p.l.pend, Change{ID: id})
	}
	return nil
}

const configColumns = `name = ?, kind = ?, color = ?, mode = ?, read_only = ?, host = ?, port = ?,
	database = ?, username = ?, options_json = ?, preconnect = ?,
	preconnect_sidecar = ?, preconnect_delay_sec = ?, auto_explain = ?, throttle_per_sec = ?,
	mcp_enabled = ?, mcp_description = ?, mcp_read_mode = ?, mcp_write_mode = ?, mcp_ddl_mode = ?,
	mcp_auto_explain = ?, mcp_review_results = ?, mcp_statement_timeout_sec = ?`

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func configArgs(c model.ConnectionFields) ([]any, error) {
	opts, err := json.Marshal(c.Options)
	if err != nil {
		return nil, fmt.Errorf("connfiles: encode options: %w", err)
	}
	return []any{
		c.Name, c.Kind, c.Color, c.Mode, b2i(c.ReadOnly), c.Host, c.Port, c.Database, c.Username, string(opts),
		c.Preconnect, b2i(c.PreconnectSidecar), c.PreconnectDelaySec, b2i(c.AutoExplain), c.ThrottlePerSec,
		b2i(c.McpEnabled), c.McpDescription, c.McpReadMode, c.McpWriteMode, c.McpDdlMode, b2i(c.McpAutoExplain),
		c.McpReviewResults, c.McpStatementTimeoutSec,
	}, nil
}

// upsert writes the connection row of stem and its mask rule rows from f. It returns the row id and the
// file the row held before, nil for a new row.
func (l *Loader) upsert(tx *sql.Tx, stem string, f File, valueJSON string) (string, *File, error) {
	fields := f.Fields()
	fields.Name = strings.TrimSpace(fields.Name)
	args, err := configArgs(fields)
	if err != nil {
		return "", nil, err
	}
	now := kiratime.NowISO()
	var id, prev string
	err = tx.QueryRow(`SELECT id, config_json FROM connections WHERE stem = ?`, stem).Scan(&id, &prev)
	var old *File
	switch {
	case errors.Is(err, sql.ErrNoRows):
		id = l.o.newID()
		order, err := sqlitex.NextSortOrder(tx, "connections", "")
		if err != nil {
			return "", nil, err
		}
		if _, err := tx.Exec(`INSERT INTO connections (id, name, kind, color, mode, read_only, host, port, database, username,
			options_json, preconnect, preconnect_sidecar, preconnect_delay_sec, auto_explain, throttle_per_sec, mcp_enabled,
			mcp_description, mcp_read_mode, mcp_write_mode, mcp_ddl_mode, mcp_auto_explain, mcp_review_results, mcp_statement_timeout_sec,
			created_at, updated_at, sort_order, stem, config_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			append(append([]any{id}, args...), now, now, order, stem, valueJSON)...); err != nil {
			return "", nil, fmt.Errorf("connfiles: insert %s: %w", stem, err)
		}
	case err != nil:
		return "", nil, fmt.Errorf("connfiles: read %s: %w", stem, err)
	default:
		var o File
		if prev != "" && json.Unmarshal([]byte(prev), &o) == nil {
			old = &o
		}
		if _, err := tx.Exec(`UPDATE connections SET `+configColumns+`, config_json = ?, updated_at = ? WHERE id = ?`,
			append(args, valueJSON, now, id)...); err != nil {
			return "", nil, fmt.Errorf("connfiles: update %s: %w", stem, err)
		}
	}
	if err := syncMasks(tx, id, f.Masks, now); err != nil {
		return "", nil, err
	}
	return id, old, nil
}

type maskRow struct{ id, createdAt string }

// syncMasks makes the rule rows of connection id equal the file's masks, keeping the id and creation
// time of a rule that stays.
func syncMasks(tx *sql.Tx, id string, masks []Mask, now string) error {
	rows, err := tx.Query(`SELECT id, lower(table_name), lower(column_name), created_at FROM connection_mask_rules WHERE connection_id = ?`, id)
	if err != nil {
		return fmt.Errorf("connfiles: read masks: %w", err)
	}
	have := map[string]maskRow{}
	for rows.Next() {
		var r maskRow
		var t, c string
		if err := rows.Scan(&r.id, &t, &c, &r.createdAt); err != nil {
			rows.Close()
			return fmt.Errorf("connfiles: scan masks: %w", err)
		}
		have[t+"\x00"+c] = r
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	keep := map[string]bool{}
	for _, m := range masks {
		mf := m.MaskFields()
		key := strings.ToLower(mf.TableName) + "\x00" + strings.ToLower(mf.ColumnName)
		keep[key] = true
		if r, ok := have[key]; ok {
			if _, err := tx.Exec(`UPDATE connection_mask_rules SET table_name = ?, column_name = ?, mask_kind = ?, keep_hint = ?, correlate = ?, updated_at = ? WHERE id = ?`,
				mf.TableName, mf.ColumnName, string(mf.Kind), b2i(mf.KeepHint), b2i(mf.Correlate), now, r.id); err != nil {
				return fmt.Errorf("connfiles: update mask: %w", err)
			}
			continue
		}
		if _, err := tx.Exec(`INSERT INTO connection_mask_rules (id, connection_id, table_name, column_name, mask_kind, keep_hint, correlate, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ids.NewID(), id, mf.TableName, mf.ColumnName, string(mf.Kind), b2i(mf.KeepHint), b2i(mf.Correlate), now, now); err != nil {
			return fmt.Errorf("connfiles: insert mask: %w", err)
		}
	}
	for key, r := range have {
		if !keep[key] {
			if _, err := tx.Exec(`DELETE FROM connection_mask_rules WHERE id = ?`, r.id); err != nil {
				return fmt.Errorf("connfiles: delete mask: %w", err)
			}
		}
	}
	return nil
}
