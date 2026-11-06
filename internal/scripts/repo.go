package scripts

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/providers"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

type rowScanner interface {
	Scan(dest ...any) error
}

const selectColumns = `id, name, command, working_dir, dir_mode, color, collection, sort_order, created_at, updated_at, kind, params_json, smart_json, use_ade_dir, schedule_json, schedule_task_id, schedule_branch_id, config_json`

// Repo reads and writes the `scripts` table: the projection of the run files plus the state
// columns (order, schedule target). List is ordered deterministically.
type Repo struct {
	DB *sql.DB
	// Agents supplies the model aliases a smart script may name; nil means the built-in providers.
	Agents *harness.Registry
	// Secrets holds saved secret param values; set by each appwire.
	Secrets *secrets.Store
}

func (r *Repo) models() harness.Catalog {
	if r.Agents == nil {
		return providers.New().Models()
	}
	return r.Agents.Models()
}

func scanRow(row rowScanner) (CustomScript, error) {
	var s CustomScript
	var created, updated int64
	var paramsJSON, smartJSON, scheduleJSON, taskID, branchID, configJSON string
	if err := row.Scan(
		&s.ID, &s.Name, &s.Command, &s.WorkingDir, &s.DirMode, &s.Color, &s.Collection, &s.SortOrder, &created, &updated,
		&s.Kind, &paramsJSON, &smartJSON, &s.UseAdeDir, &scheduleJSON, &taskID, &branchID, &configJSON,
	); err != nil {
		return CustomScript{}, err
	}
	s.CreatedAt, s.UpdatedAt = kiratime.FormatISO(time.UnixMilli(created)), kiratime.FormatISO(time.UnixMilli(updated))
	s.Params = []Param{}
	if err := json.Unmarshal([]byte(paramsJSON), &s.Params); err != nil {
		return CustomScript{}, fmt.Errorf("scripts: decode params of %s: %w", s.ID, err)
	}
	if smartJSON != "" {
		s.Smart = &Smart{}
		if err := json.Unmarshal([]byte(smartJSON), s.Smart); err != nil {
			return CustomScript{}, fmt.Errorf("scripts: decode smart settings of %s: %w", s.ID, err)
		}
	}
	if scheduleJSON != "" {
		s.Schedule = &Schedule{}
		if err := json.Unmarshal([]byte(scheduleJSON), s.Schedule); err != nil {
			return CustomScript{}, fmt.Errorf("scripts: decode schedule of %s: %w", s.ID, err)
		}
		s.Schedule.TaskID, s.Schedule.BranchID = taskID, branchID
	}
	if configJSON != "" {
		f, err := DecodeJSON(configJSON)
		if err != nil {
			return CustomScript{}, fmt.Errorf("scripts: decode config of %s: %w", s.ID, err)
		}
		if s.MCP = f.Fields().MCP; s.MCP != nil && s.MCP.Run == "" {
			s.MCP.Run = MCPRunPrompt
		}
	}
	return s, nil
}

// extras are the JSON column values of validated fields.
type extras struct{ params, smart, schedule string }

// encodeExtras encodes the params, smart and schedule columns; nil values encode as "". The schedule
// target is state, so it never enters the column.
func encodeExtras(f CustomScriptFields) (extras, error) {
	pb, err := json.Marshal(f.Params)
	if err != nil {
		return extras{}, fmt.Errorf("scripts: encode params: %w", err)
	}
	out := extras{params: string(pb)}
	if f.Smart != nil {
		sb, err := json.Marshal(f.Smart)
		if err != nil {
			return extras{}, fmt.Errorf("scripts: encode smart settings: %w", err)
		}
		out.smart = string(sb)
	}
	if f.Schedule != nil {
		sc := *f.Schedule
		sc.TaskID, sc.BranchID = "", ""
		sb, err := json.Marshal(sc)
		if err != nil {
			return extras{}, fmt.Errorf("scripts: encode schedule: %w", err)
		}
		out.schedule = string(sb)
	}
	return out, nil
}

// List orders by sort_order, name for a stable, deterministic tiebreak.
func (r *Repo) List() ([]CustomScript, error) {
	rows, err := r.DB.Query(`SELECT ` + selectColumns + ` FROM scripts ORDER BY sort_order ASC, name ASC`)
	list, err := sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (CustomScript, bool, error) {
		rec, err := scanRow(rows)
		return rec, true, err
	})
	if err != nil {
		return nil, err
	}
	saved, err := r.savedNames()
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].SavedSecrets = saved.of(list[i].ID)
	}
	return list, nil
}

// Get reads one row by id, (nil, nil) when not found.
func (r *Repo) Get(id string) (*CustomScript, error) {
	rec, err := sqlitex.QueryOne(r.DB, func(row *sql.Row) (*CustomScript, error) {
		s, err := scanRow(row)
		if err != nil {
			return nil, err
		}
		return &s, nil
	}, `SELECT `+selectColumns+` FROM scripts WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("scripts: get %s: %w", id, err)
	}
	if rec == nil {
		return nil, nil
	}
	saved, err := r.savedNames()
	if err != nil {
		return nil, fmt.Errorf("scripts: get %s: %w", id, err)
	}
	rec.SavedSecrets = saved.of(id)
	return rec, nil
}

// Reorder applies a user's drag order to one set: the ungrouped scripts ("") or one collection's.
// sort_order becomes dense per set.
func (r *Repo) Reorder(collection string, ids []string) error {
	return r.reorderTx("scripts",
		`SELECT id FROM scripts WHERE collection = ? ORDER BY sort_order ASC, name ASC`,
		`UPDATE scripts SET sort_order = ? WHERE id = ?`,
		ids, collection)
}

// ReorderCollections applies a user's drag order to the collections, named by name.
func (r *Repo) ReorderCollections(names []string) error {
	return r.reorderTx("script_collections",
		`SELECT name FROM script_collections ORDER BY sort_order ASC, name ASC`,
		`UPDATE script_collections SET sort_order = ? WHERE name = ?`,
		names)
}

func (r *Repo) reorderTx(table, selectSQL, updateSQL string, ids []string, args ...any) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("scripts: reorder %s: begin: %w", table, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := sqlitex.ReorderSortOrder(tx, selectSQL, updateSQL, ids, args...); err != nil {
		return fmt.Errorf("scripts: reorder %s: %w", table, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("scripts: reorder %s: commit: %w", table, err)
	}
	return nil
}

// ListCollections orders by sort_order (user drag order), name as the tiebreak.
func (r *Repo) ListCollections() ([]Collection, error) {
	rows, err := r.DB.Query(`SELECT name, sort_order FROM script_collections ORDER BY sort_order ASC, name ASC`)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (Collection, bool, error) {
		var c Collection
		err := rows.Scan(&c.Name, &c.SortOrder)
		return c, true, err
	})
}

// SetScheduleTarget sets the ADE task and branch a schedule runs in. Returns a wrapped sql.ErrNoRows
// for an unknown id.
func (r *Repo) SetScheduleTarget(id, taskID, branchID string) error {
	if taskID == "" {
		branchID = ""
	}
	res, err := r.DB.Exec(`UPDATE scripts SET schedule_task_id = ?, schedule_branch_id = ? WHERE id = ?`, taskID, branchID, id)
	if err != nil {
		return fmt.Errorf("scripts: set schedule target of %s: %w", id, err)
	}
	return requireRow(res, "set schedule target", id)
}

// pruneCollections drops order rows no run names.
func (r *Repo) pruneCollections() error {
	if _, err := r.DB.Exec(`DELETE FROM script_collections WHERE name NOT IN (SELECT collection FROM scripts)`); err != nil {
		return fmt.Errorf("scripts: prune collections: %w", err)
	}
	return nil
}

func requireRow(res sql.Result, op, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("scripts: %s %s: rows affected: %w", op, id, err)
	}
	if n == 0 {
		return fmt.Errorf("scripts: %s %s: %w", op, id, sql.ErrNoRows)
	}
	return nil
}

// projection implements configfs.Projection over scripts: the last good file value lives in
// config_json and the config columns follow it. State columns are never written here.
type projection struct{ r *Repo }

// Projection returns the configfs projection of the repo.
func (r *Repo) Projection() *projection { return &projection{r} }

func (p *projection) Snapshot() (map[string]string, error) {
	rows, err := p.r.DB.Query(`SELECT id, config_json FROM scripts WHERE config_json <> ''`)
	if err != nil {
		return nil, fmt.Errorf("scripts: read last good: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, js string
		if err := rows.Scan(&id, &js); err != nil {
			return nil, fmt.Errorf("scripts: scan last good: %w", err)
		}
		out[FileName(id)] = js
	}
	return out, rows.Err()
}

func (p *projection) Record(file, valueJSON string, now int64) error {
	r := p.r
	id, ok := StemOf(file)
	if !ok {
		return fmt.Errorf("scripts: record %s: not a run id", file)
	}
	f, err := DecodeJSON(valueJSON)
	if err != nil {
		return err
	}
	fields := f.Fields()
	if err := fields.Validate(r.models()); err != nil {
		return fmt.Errorf("scripts: record %s: %w", file, err)
	}
	ex, err := encodeExtras(fields)
	if err != nil {
		return err
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("scripts: record %s: begin: %w", file, err)
	}
	defer func() { _ = tx.Rollback() }()
	order, err := sqlitex.NextSortOrder(tx, "scripts", "collection = ?", fields.Collection)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO scripts (id, name, command, working_dir, dir_mode, color, collection, sort_order, created_at, updated_at,
		   kind, params_json, smart_json, use_ade_dir, schedule_json, config_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name = excluded.name, command = excluded.command, working_dir = excluded.working_dir,
		   dir_mode = excluded.dir_mode, color = excluded.color, collection = excluded.collection, updated_at = excluded.updated_at,
		   kind = excluded.kind, params_json = excluded.params_json, smart_json = excluded.smart_json,
		   use_ade_dir = excluded.use_ade_dir, schedule_json = excluded.schedule_json, config_json = excluded.config_json`,
		id, fields.Name, fields.Command, fields.WorkingDir, fields.DirMode, fields.Color, fields.Collection, order, now, now,
		fields.Kind, ex.params, ex.smart, fields.UseAdeDir, ex.schedule, valueJSON,
	); err != nil {
		return fmt.Errorf("scripts: record %s: %w", file, err)
	}
	if fields.Collection != "" {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO script_collections (name, sort_order)
			VALUES (?, (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM script_collections))`, fields.Collection); err != nil {
			return fmt.Errorf("scripts: record %s: collection: %w", file, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("scripts: record %s: commit: %w", file, err)
	}
	return r.pruneUnsecret(id, fields.Params)
}

func (p *projection) Forget(file string) error {
	r := p.r
	id, ok := StemOf(file)
	if !ok {
		return nil
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("scripts: forget %s: begin: %w", file, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM scripts WHERE id = ?`, id); err != nil {
		return fmt.Errorf("scripts: forget %s: %w", file, err)
	}
	if err := r.pruneSecrets(tx, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("scripts: forget %s: commit: %w", file, err)
	}
	return nil
}
