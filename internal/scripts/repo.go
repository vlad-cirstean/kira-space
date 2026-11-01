package scripts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/providers"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

type rowScanner interface {
	Scan(dest ...any) error
}

const selectColumns = `id, name, command, working_dir, dir_mode, color, collection_id, sort_order, created_at, updated_at, kind, params_json, smart_json, use_ade_dir, schedule_json`

// Repo reads and writes the `custom_scripts` table, List ordered deterministically.
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
	var collectionID sql.NullString
	var paramsJSON, smartJSON, scheduleJSON string
	if err := row.Scan(
		&s.ID, &s.Name, &s.Command, &s.WorkingDir, &s.DirMode, &s.Color, &collectionID, &s.SortOrder, &s.CreatedAt, &s.UpdatedAt,
		&s.Kind, &paramsJSON, &smartJSON, &s.UseAdeDir, &scheduleJSON,
	); err != nil {
		return CustomScript{}, err
	}
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
	}
	if collectionID.Valid {
		s.CollectionID = &collectionID.String
	}
	return s, nil
}

// extras are the JSON column values of validated fields.
type extras struct{ params, smart, schedule string }

// encodeExtras encodes the params, smart and schedule columns; nil values encode as "".
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
		sb, err := json.Marshal(f.Schedule)
		if err != nil {
			return extras{}, fmt.Errorf("scripts: encode schedule: %w", err)
		}
		out.schedule = string(sb)
	}
	return out, nil
}

// List orders by sort_order, name for a stable, deterministic tiebreak.
func (r *Repo) List() ([]CustomScript, error) {
	rows, err := r.DB.Query(`SELECT ` + selectColumns + ` FROM custom_scripts ORDER BY sort_order ASC, name ASC`)
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
	}, `SELECT `+selectColumns+` FROM custom_scripts WHERE id = ?`, id)
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

// Create inserts a new row, sort_order set to one past the current max — sqlitex.NextSortOrder's
// own append-at-the-end convention. fields is validated (and its name/command trimmed in place)
// before the insert.
func (r *Repo) Create(fields CustomScriptFields, secretValues map[string]*string) (CustomScript, error) {
	if err := fields.Validate(r.models()); err != nil {
		return CustomScript{}, fmt.Errorf("scripts: %w", err)
	}
	plan, err := r.planSecrets(fields, secretValues, nil)
	if err != nil {
		return CustomScript{}, err
	}
	if fields.DirMode == DirModeHome {
		return CustomScript{}, errHomeRetired
	}
	if err := r.requireCollection(fields.CollectionID); err != nil {
		return CustomScript{}, err
	}
	sortOrder, err := sqlitex.NextSortOrder(r.DB, "custom_scripts", "")
	if err != nil {
		return CustomScript{}, fmt.Errorf("scripts: next sort order: %w", err)
	}
	ex, err := encodeExtras(fields)
	if err != nil {
		return CustomScript{}, err
	}
	now := kiratime.NowISO()
	rec := CustomScript{
		ID:           uuid.NewString(),
		Name:         fields.Name,
		Command:      fields.Command,
		WorkingDir:   fields.WorkingDir,
		DirMode:      fields.DirMode,
		Color:        fields.Color,
		CollectionID: fields.CollectionID,
		SortOrder:    sortOrder,
		CreatedAt:    now,
		UpdatedAt:    now,
		Kind:         fields.Kind,
		Params:       fields.Params,
		Smart:        fields.Smart,
		UseAdeDir:    fields.UseAdeDir,
		Schedule:     fields.Schedule,
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return CustomScript{}, fmt.Errorf("scripts: insert: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		`INSERT INTO custom_scripts (id, name, command, working_dir, dir_mode, color, collection_id, sort_order, created_at, updated_at, kind, params_json, smart_json, use_ade_dir, schedule_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.Name, rec.Command, rec.WorkingDir, rec.DirMode, rec.Color, rec.CollectionID, rec.SortOrder, rec.CreatedAt, rec.UpdatedAt,
		rec.Kind, ex.params, ex.smart, rec.UseAdeDir, ex.schedule,
	); err != nil {
		return CustomScript{}, fmt.Errorf("scripts: insert: %w", err)
	}
	if err := r.applySecrets(tx, rec.ID, plan); err != nil {
		return CustomScript{}, err
	}
	if err := tx.Commit(); err != nil {
		return CustomScript{}, fmt.Errorf("scripts: insert: commit: %w", err)
	}
	rec.SavedSecrets = plan.savedNames(fields.Params)
	return rec, nil
}

// Update writes fields onto id — name/command/workingDir/color/collection only; id, sort_order and
// created_at are untouched. Returns a wrapped sql.ErrNoRows for an unknown id
// (ConnectionsRepo.SetMcpEnabled's own recorded fix, applied here from the start).
func (r *Repo) Update(id string, fields CustomScriptFields, secretValues map[string]*string) (CustomScript, error) {
	if err := fields.Validate(r.models()); err != nil {
		return CustomScript{}, fmt.Errorf("scripts: %w", err)
	}
	saved, err := r.savedNames()
	if err != nil {
		return CustomScript{}, fmt.Errorf("scripts: update %s: %w", id, err)
	}
	plan, err := r.planSecrets(fields, secretValues, saved.of(id))
	if err != nil {
		return CustomScript{}, err
	}
	if err := r.requireCollection(fields.CollectionID); err != nil {
		return CustomScript{}, err
	}
	if fields.DirMode == DirModeHome {
		cur, err := r.Get(id)
		if err != nil {
			return CustomScript{}, err
		}
		if cur != nil && cur.DirMode != DirModeHome {
			return CustomScript{}, errHomeRetired
		}
	}
	ex, err := encodeExtras(fields)
	if err != nil {
		return CustomScript{}, err
	}
	now := kiratime.NowISO()
	tx, err := r.DB.Begin()
	if err != nil {
		return CustomScript{}, fmt.Errorf("scripts: update %s: begin: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(
		`UPDATE custom_scripts SET name = ?, command = ?, working_dir = ?, dir_mode = ?, color = ?, collection_id = ?, updated_at = ?,
		 kind = ?, params_json = ?, smart_json = ?, use_ade_dir = ?, schedule_json = ? WHERE id = ?`,
		fields.Name, fields.Command, fields.WorkingDir, fields.DirMode, fields.Color, fields.CollectionID, now,
		fields.Kind, ex.params, ex.smart, fields.UseAdeDir, ex.schedule, id,
	)
	if err != nil {
		return CustomScript{}, fmt.Errorf("scripts: update %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return CustomScript{}, fmt.Errorf("scripts: update %s: rows affected: %w", id, err)
	}
	if n == 0 {
		return CustomScript{}, fmt.Errorf("scripts: update %s: %w", id, sql.ErrNoRows)
	}
	if err := r.applySecrets(tx, id, plan); err != nil {
		return CustomScript{}, err
	}
	if err := tx.Commit(); err != nil {
		return CustomScript{}, fmt.Errorf("scripts: update %s: commit: %w", id, err)
	}
	rec, err := r.Get(id)
	if err != nil {
		return CustomScript{}, err
	}
	if rec == nil {
		return CustomScript{}, fmt.Errorf("scripts: update %s: %w", id, sql.ErrNoRows)
	}
	return *rec, nil
}

// Delete deletes one row by id. Returns a wrapped sql.ErrNoRows for an unknown id, the same rule
// Update above follows.
func (r *Repo) Delete(id string) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("scripts: delete %s: begin: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`DELETE FROM custom_scripts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("scripts: delete %s: %w", id, err)
	}
	if err := requireRow(res, "delete", id); err != nil {
		return err
	}
	if err := r.pruneSecrets(tx, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("scripts: delete %s: commit: %w", id, err)
	}
	return nil
}

// requireCollection refuses a non-nil id that names no custom_script_collections row.
func (r *Repo) requireCollection(id *string) error {
	if id == nil {
		return nil
	}
	var one int
	err := r.DB.QueryRow(`SELECT 1 FROM custom_script_collections WHERE id = ?`, *id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return invalid("scripts: no such collection")
	}
	if err != nil {
		return fmt.Errorf("scripts: check collection %s: %w", *id, err)
	}
	return nil
}

// Move sets (nil clears) one command's collection and places it in the target set before
// beforeID, or last when nil, in one transaction. beforeID must be another script of the target
// set. The target set is rewritten dense; the source set keeps its gap, which is still in order.
// Returns a wrapped sql.ErrNoRows for an unknown command id.
func (r *Repo) Move(id string, collectionID, beforeID *string) error {
	if err := r.requireCollection(collectionID); err != nil {
		return err
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("scripts: move %s: begin: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(
		`UPDATE custom_scripts SET collection_id = ?, updated_at = ? WHERE id = ?`,
		collectionID, kiratime.NowISO(), id,
	)
	if err != nil {
		return fmt.Errorf("scripts: move %s: %w", id, err)
	}
	if err := requireRow(res, "move", id); err != nil {
		return err
	}
	rows, err := tx.Query(
		`SELECT id FROM custom_scripts WHERE collection_id IS ? AND id <> ? ORDER BY sort_order ASC, name ASC`,
		collectionID, id,
	)
	set, err := sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (string, bool, error) {
		var sid string
		err := rows.Scan(&sid)
		return sid, true, err
	})
	if err != nil {
		return fmt.Errorf("scripts: move %s: read target set: %w", id, err)
	}
	at := len(set)
	if beforeID != nil {
		at = -1
		for i, sid := range set {
			if sid == *beforeID {
				at = i
			}
		}
		if at < 0 {
			return invalid("scripts: before is not in the target set")
		}
	}
	ordered := make([]string, 0, len(set)+1)
	ordered = append(ordered, set[:at]...)
	ordered = append(ordered, id)
	ordered = append(ordered, set[at:]...)
	if err := sqlitex.ReorderSortOrder(tx,
		`SELECT id FROM custom_scripts WHERE collection_id IS ? ORDER BY sort_order ASC, name ASC`,
		`UPDATE custom_scripts SET sort_order = ? WHERE id = ?`,
		ordered, collectionID,
	); err != nil {
		return fmt.Errorf("scripts: move %s: reorder: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("scripts: move %s: commit: %w", id, err)
	}
	return nil
}

// Reorder applies a user's drag order to one set: the ungrouped scripts (nil) or one collection's.
// sort_order becomes dense per set; updated_at stays, so a script's content hash does not move.
func (r *Repo) Reorder(collectionID *string, ids []string) error {
	if err := r.requireCollection(collectionID); err != nil {
		return err
	}
	return r.reorderTx("custom_scripts",
		`SELECT id FROM custom_scripts WHERE collection_id IS ? ORDER BY sort_order ASC, name ASC`,
		`UPDATE custom_scripts SET sort_order = ? WHERE id = ?`,
		ids, collectionID)
}

// ReorderCollections applies a user's drag order to the collections.
func (r *Repo) ReorderCollections(ids []string) error {
	return r.reorderTx("custom_script_collections",
		`SELECT id FROM custom_script_collections ORDER BY sort_order ASC, name ASC`,
		`UPDATE custom_script_collections SET sort_order = ? WHERE id = ?`,
		ids)
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

const collectionColumns = `id, name, sort_order, created_at, updated_at`

func scanCollection(row rowScanner) (Collection, error) {
	var c Collection
	err := row.Scan(&c.ID, &c.Name, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// ListCollections orders by sort_order (user drag order), name as the tiebreak.
func (r *Repo) ListCollections() ([]Collection, error) {
	rows, err := r.DB.Query(`SELECT ` + collectionColumns + ` FROM custom_script_collections ORDER BY sort_order ASC, name ASC`)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (Collection, bool, error) {
		c, err := scanCollection(rows)
		return c, true, err
	})
}

// CreateCollection appends a collection at the end of the order.
func (r *Repo) CreateCollection(name string) (Collection, error) {
	name, err := validCollectionName(name)
	if err != nil {
		return Collection{}, err
	}
	sortOrder, err := sqlitex.NextSortOrder(r.DB, "custom_script_collections", "")
	if err != nil {
		return Collection{}, fmt.Errorf("scripts: next collection sort order: %w", err)
	}
	now := kiratime.NowISO()
	c := Collection{ID: uuid.NewString(), Name: name, SortOrder: sortOrder, CreatedAt: now, UpdatedAt: now}
	if _, err := r.DB.Exec(
		`INSERT INTO custom_script_collections (`+collectionColumns+`) VALUES (?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.SortOrder, c.CreatedAt, c.UpdatedAt,
	); err != nil {
		return Collection{}, fmt.Errorf("scripts: insert collection: %w", err)
	}
	return c, nil
}

// RenameCollection renames one collection. Returns a wrapped sql.ErrNoRows for an unknown id.
func (r *Repo) RenameCollection(id, name string) error {
	name, err := validCollectionName(name)
	if err != nil {
		return err
	}
	res, err := r.DB.Exec(
		`UPDATE custom_script_collections SET name = ?, updated_at = ? WHERE id = ?`,
		name, kiratime.NowISO(), id,
	)
	if err != nil {
		return fmt.Errorf("scripts: rename collection %s: %w", id, err)
	}
	return requireRow(res, "rename collection", id)
}

// DeleteCollection removes a collection; the foreign key cascades to its commands. Returns a
// wrapped sql.ErrNoRows for an unknown id.
func (r *Repo) DeleteCollection(id string) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("scripts: delete collection %s: begin: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query(`SELECT id FROM custom_scripts WHERE collection_id = ?`, id)
	ids, err := sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (string, bool, error) {
		var sid string
		err := rows.Scan(&sid)
		return sid, true, err
	})
	if err != nil {
		return fmt.Errorf("scripts: delete collection %s: list scripts: %w", id, err)
	}
	res, err := tx.Exec(`DELETE FROM custom_script_collections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("scripts: delete collection %s: %w", id, err)
	}
	if err := requireRow(res, "delete collection", id); err != nil {
		return err
	}
	for _, sid := range ids {
		if err := r.pruneSecrets(tx, sid); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("scripts: delete collection %s: commit: %w", id, err)
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
