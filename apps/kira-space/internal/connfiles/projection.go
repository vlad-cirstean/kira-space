package connfiles

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// Change is one connection a load or a write made stale. Old is nil for a new connection and New is
// nil for a removed one.
type Change struct {
	ID       string
	Old, New *File
}

// Masks reports whether the mask rules differ between the two sides.
func (c Change) Masks() bool { return !sameJSON(masksOf(c.Old), masksOf(c.New)) }

// MaskReview reports whether column review (on flag or reviewed list) differs.
func (c Change) MaskReview() bool { return !sameJSON(reviewOf(c.Old), reviewOf(c.New)) }

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

func reviewOf(f *File) *MaskReview {
	if f == nil {
		return nil
	}
	return f.MaskReview
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
	c := kiraschema.Connections
	sb := p.l.o.Dialect.Select(c.Stem, c.ConfigJSON).From(c.Table)
	sb.Where(sb.IsNotNull(c.Stem), sb.NotEqual(c.ConfigJSON, ""))
	rows, err := sqlb.Query(p.l.o.DB, sb)
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
	c := kiraschema.Connections
	sb := p.l.o.Dialect.Select(c.ID, c.ConfigJSON).From(c.Table)
	sb.Where(sb.Equal(c.Stem, stem))
	err := sqlb.QueryRow(p.l.o.DB, sb).Scan(&id, &js)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("connfiles: read %s: %w", stem, err)
	}
	if p.l.o.OnForget != nil {
		p.l.o.OnForget(id)
	}
	del := p.l.o.Dialect.DeleteFrom(c.Table)
	del.Where(del.Equal(c.ID, id))
	if _, err := sqlb.Exec(p.l.o.DB, del); err != nil {
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

// configColumns lists the columns configArgs fills, in the same order.
var configColumns = func() []string {
	c := kiraschema.Connections
	return []string{
		c.Name, c.Kind, c.Color, c.Mode, c.ReadOnly, c.Host, c.Port, c.Database, c.Username, c.OptionsJSON,
		c.Preconnect, c.PreconnectSidecar, c.PreconnectDelaySec, c.AutoExplain, c.ThrottlePerSec,
		c.MCPEnabled, c.MCPDescription, c.MCPReadMode, c.MCPWriteMode, c.MCPDdlMode, c.MCPMaskedMode,
		c.MCPAutoExplain, c.MCPReviewRead, c.MCPReviewWrite, c.MCPReviewDdl, c.MCPReviewMasked,
		c.MCPStatementTimeoutSec, c.MCPTablesJSON, c.MaskReviewOn,
	}
}()

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
	var tables any
	if c.McpTables != nil {
		js, err := json.Marshal(c.McpTables)
		if err != nil {
			return nil, fmt.Errorf("connfiles: encode mcp tables: %w", err)
		}
		tables = string(js)
	}
	return []any{
		c.Name, c.Kind, c.Color, c.Mode, b2i(c.ReadOnly), c.Host, c.Port, c.Database, c.Username, string(opts),
		c.Preconnect, b2i(c.PreconnectSidecar), c.PreconnectDelaySec, b2i(c.AutoExplain), c.ThrottlePerSec,
		b2i(c.McpEnabled), c.McpDescription, c.McpReadMode, c.McpWriteMode, c.McpDdlMode, c.McpMaskedMode, b2i(c.McpAutoExplain),
		c.McpReview.Read, c.McpReview.Write, c.McpReview.Ddl, c.McpReview.Masked, c.McpStatementTimeoutSec, tables, b2i(c.MaskReviewOn),
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
	now := kiratime.NowMs()
	c := kiraschema.Connections
	d := l.o.Dialect
	var id, prev string
	sel := d.Select(c.ID, c.ConfigJSON).From(c.Table)
	sel.Where(sel.Equal(c.Stem, stem))
	err = sqlb.QueryRow(tx, sel).Scan(&id, &prev)
	var old *File
	switch {
	case errors.Is(err, sql.ErrNoRows):
		id = l.o.newID()
		order, err := sqlb.NextSortOrder(tx, d, c.Table, c.SortOrder, nil)
		if err != nil {
			return "", nil, err
		}
		cols := append(append([]string{c.ID}, configColumns...), c.CreatedAt, c.UpdatedAt, c.SortOrder, c.Stem, c.ConfigJSON)
		ib := d.InsertInto(c.Table).Cols(cols...).Values(append(append([]any{id}, args...), now, now, order, stem, valueJSON)...)
		if _, err := sqlb.Exec(tx, ib); err != nil {
			return "", nil, fmt.Errorf("connfiles: insert %s: %w", stem, err)
		}
	case err != nil:
		return "", nil, fmt.Errorf("connfiles: read %s: %w", stem, err)
	default:
		var o File
		if prev != "" && json.Unmarshal([]byte(prev), &o) == nil {
			old = &o
		}
		ub := d.Update(c.Table)
		for i, col := range configColumns {
			ub.SetMore(ub.Assign(col, args[i]))
		}
		ub.SetMore(ub.Assign(c.ConfigJSON, valueJSON), ub.Assign(c.UpdatedAt, now)).Where(ub.Equal(c.ID, id))
		if _, err := sqlb.Exec(tx, ub); err != nil {
			return "", nil, fmt.Errorf("connfiles: update %s: %w", stem, err)
		}
	}
	if err := syncMasks(tx, d, id, f.Masks, now); err != nil {
		return "", nil, err
	}
	if err := syncReviewed(tx, d, id, f.ReviewedColumns()); err != nil {
		return "", nil, err
	}
	return id, old, nil
}

// syncReviewed makes the reviewed column rows of connection id equal want, touching only the difference.
func syncReviewed(tx *sql.Tx, d sqlb.Dialect, id string, want []string) error {
	c := kiraschema.ConnectionMaskReviewed
	sel := d.Select(c.ColumnRef).From(c.Table)
	sel.Where(sel.Equal(c.ConnectionID, id))
	rows, err := sqlb.Query(tx, sel)
	if err != nil {
		return fmt.Errorf("connfiles: read reviewed columns: %w", err)
	}
	have := map[string]bool{}
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			rows.Close()
			return fmt.Errorf("connfiles: scan reviewed columns: %w", err)
		}
		have[ref] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	keep := make(map[string]bool, len(want))
	for _, ref := range want {
		keep[ref] = true
		if have[ref] {
			continue
		}
		ib := d.InsertInto(c.Table).Cols(c.ConnectionID, c.ColumnRef).Values(id, ref)
		if _, err := sqlb.Exec(tx, ib); err != nil {
			return fmt.Errorf("connfiles: insert reviewed column: %w", err)
		}
	}
	for ref := range have {
		if !keep[ref] {
			del := d.DeleteFrom(c.Table)
			del.Where(del.Equal(c.ConnectionID, id), del.Equal(c.ColumnRef, ref))
			if _, err := sqlb.Exec(tx, del); err != nil {
				return fmt.Errorf("connfiles: delete reviewed column: %w", err)
			}
		}
	}
	return nil
}

type maskRow struct {
	id        string
	createdAt int64
}

// syncMasks makes the rule rows of connection id equal the file's masks, keeping the id and creation
// time of a rule that stays.
func syncMasks(tx *sql.Tx, d sqlb.Dialect, id string, masks []Mask, now int64) error {
	c := kiraschema.ConnectionMaskRules
	sel := d.Select(c.ID, "lower("+c.TableName+")", "lower("+c.ColumnName+")", c.CreatedAt).From(c.Table)
	sel.Where(sel.Equal(c.ConnectionID, id))
	rows, err := sqlb.Query(tx, sel)
	if err != nil {
		return fmt.Errorf("connfiles: read masks: %w", err)
	}
	have := map[string]maskRow{}
	for rows.Next() {
		var r maskRow
		var t, col string
		if err := rows.Scan(&r.id, &t, &col, &r.createdAt); err != nil {
			rows.Close()
			return fmt.Errorf("connfiles: scan masks: %w", err)
		}
		have[t+"\x00"+col] = r
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
			ub := d.Update(c.Table)
			ub.Set(ub.Assign(c.TableName, mf.TableName), ub.Assign(c.ColumnName, mf.ColumnName), ub.Assign(c.MaskKind, string(mf.Kind)),
				ub.Assign(c.KeepHint, b2i(mf.KeepHint)), ub.Assign(c.Correlate, b2i(mf.Correlate)), ub.Assign(c.UpdatedAt, now)).
				Where(ub.Equal(c.ID, r.id))
			if _, err := sqlb.Exec(tx, ub); err != nil {
				return fmt.Errorf("connfiles: update mask: %w", err)
			}
			continue
		}
		ib := d.InsertInto(c.Table).Cols(c.ID, c.ConnectionID, c.TableName, c.ColumnName, c.MaskKind, c.KeepHint, c.Correlate, c.CreatedAt, c.UpdatedAt).
			Values(ids.NewID(), id, mf.TableName, mf.ColumnName, string(mf.Kind), b2i(mf.KeepHint), b2i(mf.Correlate), now, now)
		if _, err := sqlb.Exec(tx, ib); err != nil {
			return fmt.Errorf("connfiles: insert mask: %w", err)
		}
	}
	for key, r := range have {
		if !keep[key] {
			del := d.DeleteFrom(c.Table)
			del.Where(del.Equal(c.ID, r.id))
			if _, err := sqlb.Exec(tx, del); err != nil {
				return fmt.Errorf("connfiles: delete mask: %w", err)
			}
		}
	}
	return nil
}
