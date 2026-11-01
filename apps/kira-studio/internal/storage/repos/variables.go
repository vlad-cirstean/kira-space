package repos

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/sqlitex"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/postman"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// variableHistoryLimit mirrors historyLimit (filter_history.go) — the same bound, applied to a
// different table (P5 D13).
const variableHistoryLimit = 20

// VariablesRepo owns three tables: api_environments, api_variables and
// api_variable_history. One repo for both scopes (collection and environment), not two: a
// collection variable and an environment variable share every field, every secret rule, the same
// history table and the same dense-sort_order arithmetic — the asymmetry that split
// api_collections from api_items does not exist here.
//
// No prepared statement: these queries run on dialog open/edit, not per keystroke (mirrors
// CollectionsRepo's own reasoning).
type VariablesRepo struct {
	db     *sql.DB
	cipher Cipher
}

// NewVariables mirrors NewSecrets: constructed separately from repos.New's aggregate (not because
// the cipher is unavailable at that point — it already exists by then — but to keep the same
// "a repo that touches secret_value takes its Cipher explicitly, at its own call site" shape
// repos/secrets.go established, rather than widening repos.New's own signature for one repo among
// many that do not need it).
func NewVariables(db *sql.DB, cipher Cipher) *VariablesRepo {
	return &VariablesRepo{db: db, cipher: cipher}
}

// ---- environments (D3) ----

// Unlike repos/connections.go:53 (which *drops* a row whose colour it does not
// recognise), an environment owns variables and a collection's requests reference it — losing one
// over a cosmetic column is a data-loss bug waiting for a hand-edited database. An unrecognised
// stored value is coerced to 'none' instead, with the same slog.Warn.
func coerceEnvironmentColor(id, color string) string {
	if model.ValidPaletteColor(color) {
		return color
	}
	slog.Warn("repos/variables: environment has unrecognised colour, coercing to none", "scope", "storage/variables", "id", id, "color", color)
	return "none"
}

func (r *VariablesRepo) ListEnvironments() ([]model.Environment, error) {
	rows, err := r.db.Query(`SELECT id, name, sort_order, is_active, description, color FROM api_environments ORDER BY sort_order, name`)
	if err != nil {
		return nil, fmt.Errorf("repos/variables: list environments: %w", err)
	}
	defer rows.Close()

	out := []model.Environment{}
	for rows.Next() {
		var (
			e        model.Environment
			isActive int
		)
		if err := rows.Scan(&e.ID, &e.Name, &e.SortOrder, &isActive, &e.Description, &e.Color); err != nil {
			return nil, fmt.Errorf("repos/variables: scan environment: %w", err)
		}
		e.IsActive = isActive != 0
		e.Color = coerceEnvironmentColor(e.ID, e.Color)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos/variables: environment rows: %w", err)
	}
	return out, nil
}

func (r *VariablesRepo) CreateEnvironment(name, description, color string) (model.Environment, error) {
	if name == "" {
		return model.Environment{}, fmt.Errorf("repos/variables: name is required")
	}
	order, err := sqlitex.NextSortOrder(r.db, "api_environments", "")
	if err != nil {
		return model.Environment{}, fmt.Errorf("repos/variables: next environment order: %w", err)
	}
	now := kiratime.NowISO()
	e := model.Environment{ID: uuid.NewString(), Name: name, SortOrder: order, Description: description, Color: color}
	if _, err := r.db.Exec(
		`INSERT INTO api_environments (id, name, sort_order, is_active, description, color, created_at, updated_at)
		 VALUES (?, ?, ?, 0, ?, ?, ?, ?)`,
		e.ID, e.Name, e.SortOrder, e.Description, e.Color, now, now,
	); err != nil {
		return model.Environment{}, fmt.Errorf("repos/variables: insert environment: %w", err)
	}
	return e, nil
}

// UpdateEnvironment replaces RenameEnvironment (P17 D14): renaming, describing and (P18) colouring
// an environment are one row update — one IPC call for one blur/swatch-click is worse than one
// call carrying all three fields.
func (r *VariablesRepo) UpdateEnvironment(id, name, description, color string) error {
	if id == "" || name == "" {
		return fmt.Errorf("repos/variables: id and name are required")
	}
	res, err := r.db.Exec(
		`UPDATE api_environments SET name = ?, description = ?, color = ?, updated_at = ? WHERE id = ?`,
		name, description, color, kiratime.NowISO(), id,
	)
	if err != nil {
		return fmt.Errorf("repos/variables: update environment %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "environment "+id)
}

// DuplicateEnvironment is a raw-column copy of one environment and its variables — the precedent
// (connections.Service.Duplicate's "a raw column copy, not decrypt-then-
// re-encrypt — the plaintext is never used, so there is no reason for this path to need the OS
// key at all"), applied verbatim here since api_variables.secret_value holds the same kira:v3:
// envelope connections.password does. This is a same-kind copy (variable → variable, ScopeVariable
// both sides), which is exactly what the scope-bound AAD leaves untouched.
//
//  1. A new environment row, name+" copy", description copied, sort_order = MAX+1,
//     is_active = 0 — the active environment is a single app-global selection; duplicating
//     the active one must not create a second active row or silently steal the selection.
//  2. Every variable row copied with a fresh id, secret_value copied as the raw ciphertext column
//     (never decrypted) — duplication needs no OS key, prompts no reveal gate, and works on a
//     machine whose keychain entry is missing.
//  3. No api_variable_history rows are copied — a clone's rows were created just now, and it has
//     no prior values; copying history would put a *second* variable's secret ciphertext into a
//     history row the user never wrote.
func (r *VariablesRepo) DuplicateEnvironment(id string) (model.Environment, error) {
	if id == "" {
		return model.Environment{}, fmt.Errorf("repos/variables: id is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return model.Environment{}, fmt.Errorf("repos/variables: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var srcName, srcDescription, srcColor string
	err = tx.QueryRow(`SELECT name, description, color FROM api_environments WHERE id = ?`, id).Scan(&srcName, &srcDescription, &srcColor)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Environment{}, fmt.Errorf("repos/variables: no environment %s", id)
	}
	if err != nil {
		return model.Environment{}, fmt.Errorf("repos/variables: read environment %s: %w", id, err)
	}
	srcColor = coerceEnvironmentColor(id, srcColor)

	order, err := sqlitex.NextSortOrder(tx, "api_environments", "")
	if err != nil {
		return model.Environment{}, fmt.Errorf("repos/variables: next environment order: %w", err)
	}

	now := kiratime.NowISO()
	newEnv := model.Environment{
		ID: uuid.NewString(), Name: srcName + " copy", SortOrder: order, IsActive: false,
		Description: srcDescription, Color: srcColor,
	}
	if _, err := tx.Exec(
		`INSERT INTO api_environments (id, name, sort_order, is_active, description, color, created_at, updated_at)
		 VALUES (?, ?, ?, 0, ?, ?, ?, ?)`,
		newEnv.ID, newEnv.Name, newEnv.SortOrder, newEnv.Description, newEnv.Color, now, now,
	); err != nil {
		return model.Environment{}, fmt.Errorf("repos/variables: insert duplicated environment: %w", err)
	}

	rows, err := tx.Query(
		`SELECT name, value, is_secret, secret_value, sort_order, description FROM api_variables
		  WHERE environment_id = ? ORDER BY sort_order`,
		id,
	)
	if err != nil {
		return model.Environment{}, fmt.Errorf("repos/variables: read source variables: %w", err)
	}
	type srcRow struct {
		name, value, description string
		isSecret                 int
		secretValue              sql.NullString
		sortOrder                int
	}
	var srcRows []srcRow
	for rows.Next() {
		var sr srcRow
		if err := rows.Scan(&sr.name, &sr.value, &sr.isSecret, &sr.secretValue, &sr.sortOrder, &sr.description); err != nil {
			rows.Close()
			return model.Environment{}, fmt.Errorf("repos/variables: scan source variable: %w", err)
		}
		srcRows = append(srcRows, sr)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return model.Environment{}, fmt.Errorf("repos/variables: source variable rows: %w", err)
	}
	rows.Close()

	for _, sr := range srcRows {
		if _, err := tx.Exec(
			`INSERT INTO api_variables (id, collection_id, environment_id, name, value, is_secret, secret_value, sort_order, description, created_at, updated_at)
			 VALUES (?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), newEnv.ID, sr.name, sr.value, sr.isSecret, sr.secretValue, sr.sortOrder, sr.description, now, now,
		); err != nil {
			return model.Environment{}, fmt.Errorf("repos/variables: insert duplicated variable: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return model.Environment{}, fmt.Errorf("repos/variables: commit: %w", err)
	}
	return newEnv, nil
}

// DeleteEnvironment cascades its variables and their history (ON DELETE CASCADE, D4). Deleting the
// active environment simply leaves none active (D3) — there is nothing further to reassign.
func (r *VariablesRepo) DeleteEnvironment(id string) error {
	if id == "" {
		return fmt.Errorf("repos/variables: id is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("repos/variables: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`DELETE FROM api_environments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("repos/variables: delete environment %s: %w", id, err)
	}
	if err := sqlitex.RequireOneRow(res, "environment "+id); err != nil {
		return err
	}
	if err := reindexEnvironments(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos/variables: commit: %w", err)
	}
	return nil
}

// SetActiveEnvironment enforces D3's invariant — at most one active row — in one transaction.
// id == "" selects "No environment": the first statement alone.
func (r *VariablesRepo) SetActiveEnvironment(id string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("repos/variables: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE api_environments SET is_active = 0`); err != nil {
		return fmt.Errorf("repos/variables: clear active environment: %w", err)
	}
	if id != "" {
		res, err := tx.Exec(`UPDATE api_environments SET is_active = 1, updated_at = ? WHERE id = ?`, kiratime.NowISO(), id)
		if err != nil {
			return fmt.Errorf("repos/variables: set active environment %s: %w", id, err)
		}
		if err := sqlitex.RequireOneRow(res, "environment "+id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos/variables: commit: %w", err)
	}
	return nil
}

func (r *VariablesRepo) ReorderEnvironments(ids []string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("repos/variables: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := sqlitex.ReorderSortOrder(tx,
		`SELECT id FROM api_environments ORDER BY sort_order, created_at, id`,
		`UPDATE api_environments SET sort_order = ? WHERE id = ?`,
		ids,
	); err != nil {
		return fmt.Errorf("repos/variables: reorder environments: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos/variables: commit: %w", err)
	}
	return nil
}

// reindexEnvironments rewrites every environment's sort_order dense, 0..n-1, in the order the rows
// already have — CollectionsRepo.reindexSiblings' own discipline (P4 D2), applied to a table with
// no parent to scope by.
func reindexEnvironments(tx *sql.Tx) error {
	return sqlitex.ReindexSortOrder(tx,
		`SELECT id FROM api_environments ORDER BY sort_order, created_at, id`,
		`UPDATE api_environments SET sort_order = ? WHERE id = ?`,
	)
}

// ---- variables (D4/D5/D12) ----

func scopeColumn(scope model.VariableScope) (string, error) {
	switch scope {
	case model.VariableScopeCollection:
		return "collection_id", nil
	case model.VariableScopeEnvironment:
		return "environment_id", nil
	}
	return "", fmt.Errorf("repos/variables: unrecognised scope %q", scope)
}

// List returns one scope's variables, ordered dense by sort_order. Never selects secret_value —
// D4/D5's whole security property is a fact about this SQL projection, not a Go branch. A
// collection's own list is promoted first (D15/F5), one-shot, if it has not been already.
func (r *VariablesRepo) List(scope model.VariableScope, ownerID string) ([]model.Variable, error) {
	column, err := scopeColumn(scope)
	if err != nil {
		return nil, err
	}
	if ownerID == "" {
		return nil, fmt.Errorf("repos/variables: ownerId is required")
	}
	if scope == model.VariableScopeCollection {
		if err := r.promoteIfNeeded(ownerID); err != nil {
			// F5/D15: a promotion failure must not make the list itself unreadable — it is
			// retried on the next List, and the collection's own variables (if any were already
			// promoted) still read correctly.
			slog.Warn("promoting a pre-P5 collection's variable[] failed", "scope", "storage/variables", "collectionId", ownerID, "err", err)
		}
	}

	rows, err := r.db.Query(
		`SELECT id, name, value, is_secret, sort_order, description FROM api_variables
		  WHERE `+column+` = ? ORDER BY sort_order, name`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("repos/variables: query %s: %w", scope, err)
	}
	defer rows.Close()

	out := []model.Variable{}
	for rows.Next() {
		v := model.Variable{Scope: scope, OwnerID: ownerID}
		var isSecret int
		if err := rows.Scan(&v.ID, &v.Name, &v.Value, &isSecret, &v.SortOrder, &v.Description); err != nil {
			return nil, fmt.Errorf("repos/variables: scan variable: %w", err)
		}
		v.IsSecret = isSecret != 0
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos/variables: variable rows: %w", err)
	}
	return out, nil
}

// Upsert creates (id == "") or updates one variable. value is a three-state pointer — nil = leave
// the stored value untouched, matching connections.Input.Password's own "nil = unchanged" contract
// (connections/input.go). F2 (P108 Part 3): a secret's list projection is always "" (D4/D5), so a
// caller that reads a row, edits only its name or description, and writes it back without ever
// revealing the secret has no real plaintext to send — a plain string parameter forced every such
// edit to send "" and Upsert unconditionally re-encrypted it, silently wiping the secret. nil closes
// that: only a genuine edit (the frontend's own "value touched" flag) sends a non-nil value, and
// "" is still a real, explicit clear when a caller does send it. Required for a create (id == "").
// When non-nil, it is always the plaintext — a secret's plaintext crosses the bridge here
// deliberately, in the one direction D5 never restricts: the user just typed it into a revealed,
// editable field, the same as ConnectionDialog's password field. D13: an update that actually
// changes the stored value records the value it replaced, inside the same transaction, before
// trimming to variableHistoryLimit.
func (r *VariablesRepo) Upsert(scope model.VariableScope, ownerID, id, name string, value *string, isSecret bool, description string) (model.Variable, error) {
	if _, err := scopeColumn(scope); err != nil {
		return model.Variable{}, err
	}
	if err := (model.Variable{Name: name}).Validate(); err != nil {
		return model.Variable{}, err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return model.Variable{}, fmt.Errorf("repos/variables: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := kiratime.NowISO()
	if id == "" {
		if value == nil {
			return model.Variable{}, fmt.Errorf("repos/variables: value is required to create a variable")
		}
		storedValue, storedSecret, err := r.encryptFor(*value, isSecret)
		if err != nil {
			return model.Variable{}, err
		}
		return r.insertVariable(tx, scope, ownerID, name, *value, isSecret, description, storedValue, storedSecret, now)
	}

	existing, err := r.readVariableForUpdate(tx, id)
	if err != nil {
		return model.Variable{}, err
	}

	if value == nil {
		return r.updateVariableUntouched(tx, id, name, description, now, existing)
	}
	return r.updateVariableValue(tx, id, name, *value, isSecret, description, now, existing)
}

// existingVariableRow is Upsert's own pre-image of the row an update reads before writing —
// shared by both of its own branches below (F2, P108 Part 3's own split to stay under gocognit's
// cap).
type existingVariableRow struct {
	scope          model.VariableScope
	ownerID        string
	oldValue       string
	oldSecret      bool
	oldSecretValue sql.NullString
	sortOrder      int
}

func (r *VariablesRepo) readVariableForUpdate(tx *sql.Tx, id string) (existingVariableRow, error) {
	var (
		collectionID, environmentID sql.NullString
		row                         existingVariableRow
		oldSecretInt                int
	)
	err := tx.QueryRow(
		`SELECT collection_id, environment_id, value, is_secret, secret_value, sort_order FROM api_variables WHERE id = ?`, id,
	).Scan(&collectionID, &environmentID, &row.oldValue, &oldSecretInt, &row.oldSecretValue, &row.sortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return existingVariableRow{}, fmt.Errorf("repos/variables: no variable %s", id)
	}
	if err != nil {
		return existingVariableRow{}, fmt.Errorf("repos/variables: read variable %s: %w", id, err)
	}
	row.oldSecret = oldSecretInt != 0
	if collectionID.Valid {
		row.scope, row.ownerID = model.VariableScopeCollection, collectionID.String
	} else {
		row.scope, row.ownerID = model.VariableScopeEnvironment, environmentID.String
	}
	return row, nil
}

// updateVariableUntouched is Upsert's value == nil branch — nothing about the
// value transitioned, so value/secret_value/is_secret stay exactly as stored (an isSecret flip
// with no real value to move into/out of secret_value has no meaning here; the frontend never
// sends one) and no history is recorded, only name/description.
func (r *VariablesRepo) updateVariableUntouched(tx *sql.Tx, id, name, description, now string, existing existingVariableRow) (model.Variable, error) {
	if _, err := tx.Exec(
		`UPDATE api_variables SET name = ?, description = ?, updated_at = ? WHERE id = ?`,
		name, description, now, id,
	); err != nil {
		return model.Variable{}, fmt.Errorf("repos/variables: update variable %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return model.Variable{}, fmt.Errorf("repos/variables: commit: %w", err)
	}
	out := model.Variable{ID: id, Scope: existing.scope, OwnerID: existing.ownerID, Name: name, Value: existing.oldValue, IsSecret: existing.oldSecret, SortOrder: existing.sortOrder, Description: description}
	if out.IsSecret {
		out.Value = ""
	}
	return out, nil
}

// updateVariableValue is Upsert's value != nil branch — the ordinary path: encrypt if secret,
// record history for a real change, write the row, purge pre-secret history on a plain-to-secret
// flip (Finding 1, P21 round 3).
func (r *VariablesRepo) updateVariableValue(tx *sql.Tx, id, name, value string, isSecret bool, description, now string, existing existingVariableRow) (model.Variable, error) {
	storedValue, storedSecret, err := r.encryptFor(value, isSecret)
	if err != nil {
		return model.Variable{}, err
	}

	if changed, oldPlain, oldPlainOK := r.valueChanged(existing.oldValue, existing.oldSecret, existing.oldSecretValue, value); changed && oldPlainOK {
		if err := r.recordHistory(tx, id, oldPlain, existing.oldSecret, now); err != nil {
			return model.Variable{}, err
		}
	}

	if _, err := tx.Exec(
		`UPDATE api_variables SET name = ?, value = ?, is_secret = ?, secret_value = ?, description = ?, updated_at = ? WHERE id = ?`,
		name, storedValue, boolToInt(isSecret), storedSecret, description, now, id,
	); err != nil {
		return model.Variable{}, fmt.Errorf("repos/variables: update variable %s: %w", id, err)
	}

	// A plain-to-secret transition purges pre-secret history — see purgePlaintextHistory's own
	// comment (Finding 1, P21 round 3) for why.
	if isSecret && !existing.oldSecret {
		if err := purgePlaintextHistory(tx, id); err != nil {
			return model.Variable{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return model.Variable{}, fmt.Errorf("repos/variables: commit: %w", err)
	}

	out := model.Variable{ID: id, Scope: existing.scope, OwnerID: existing.ownerID, Name: name, Value: value, IsSecret: isSecret, SortOrder: existing.sortOrder, Description: description}
	if out.IsSecret {
		out.Value = ""
	}
	return out, nil
}

func mustScopeColumn(scope model.VariableScope) string {
	c, _ := scopeColumn(scope)
	return c
}

// insertVariable is Upsert's own id == "" branch: assign the next sort_order, insert the row,
// commit, and return it (value blanked for a secret, matching List's own convention).
func (r *VariablesRepo) insertVariable(tx *sql.Tx, scope model.VariableScope, ownerID, name, value string, isSecret bool, description string, storedValue string, storedSecret *string, now string) (model.Variable, error) {
	if ownerID == "" {
		return model.Variable{}, fmt.Errorf("repos/variables: ownerId is required")
	}
	order, err := sqlitex.NextSortOrder(tx, "api_variables", mustScopeColumn(scope)+" = ?", ownerID)
	if err != nil {
		return model.Variable{}, fmt.Errorf("repos/variables: next variable order: %w", err)
	}
	v := model.Variable{ID: uuid.NewString(), Scope: scope, OwnerID: ownerID, Name: name, Value: value, IsSecret: isSecret, SortOrder: order, Description: description}
	var collectionID, environmentID *string
	if scope == model.VariableScopeCollection {
		collectionID = &ownerID
	} else {
		environmentID = &ownerID
	}
	if _, err := tx.Exec(
		`INSERT INTO api_variables (id, collection_id, environment_id, name, value, is_secret, secret_value, sort_order, description, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, collectionID, environmentID, v.Name, storedValue, boolToInt(v.IsSecret), storedSecret, v.SortOrder, v.Description, now, now,
	); err != nil {
		return model.Variable{}, fmt.Errorf("repos/variables: insert variable: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.Variable{}, fmt.Errorf("repos/variables: commit: %w", err)
	}
	// value in the returned struct follows List's own convention: '' for a secret.
	if v.IsSecret {
		v.Value = ""
	}
	return v, nil
}

// purgePlaintextHistory: a variable flipped
// from plain to secret must not leave its pre-secret plaintext sitting in api_variable_history.
// recordHistory stamps a row's is_secret from the flag *at the time that row was written*, so
// every history row recorded while this variable was still plain (is_secret = 0) — including,
// potentially, the one just inserted above for this very transition — carries the old value in
// cleartext in its `value` column, with no reveal gate. Once the variable itself becomes a
// secret, those rows are exactly as sensitive as its current value and the app has no story for
// "the old value of a secret that was never a secret" (there is nothing meaningful to re-encrypt
// into — the value was typed in the clear), so the honest fix is to drop them rather than pretend
// they were always protected.
func purgePlaintextHistory(tx *sql.Tx, id string) error {
	if _, err := tx.Exec(`DELETE FROM api_variable_history WHERE variable_id = ? AND is_secret = 0`, id); err != nil {
		return fmt.Errorf("repos/variables: purge pre-secret history %s: %w", id, err)
	}
	return nil
}

// encryptFor turns a plaintext into the pair of columns api_variables actually stores (D4's
// CHECK: exactly one of value/secret_value is populated).
func (r *VariablesRepo) encryptFor(value string, isSecret bool) (storedValue string, storedSecret *string, err error) {
	if !isSecret {
		return value, nil, nil
	}
	encrypted, err := r.cipher.Encrypt(secrets.ScopeVariable, value)
	if err != nil {
		return "", nil, fmt.Errorf("repos/variables: encrypt: %w", err)
	}
	return "", &encrypted, nil
}

// valueChanged decrypts the stored old value (if secret) once and compares plaintext:
// comparing ciphertext directly is meaningless, since GCM nonces differ per encryption. A decrypt
// failure (a keychain reset, a database copied from another machine) is reported and treated as
// "cannot tell", so no history entry is recorded for it — recording garbage would only compound the
// original problem, and the edit itself must not be blocked by an old value nobody can read any
// more.
func (r *VariablesRepo) valueChanged(oldValue string, oldSecret bool, oldSecretValue sql.NullString, newValue string) (changed bool, oldPlain string, ok bool) {
	if !oldSecret {
		return oldValue != newValue, oldValue, true
	}
	if !oldSecretValue.Valid {
		return false, "", false
	}
	plain, err := r.cipher.Decrypt(secrets.ScopeVariable, oldSecretValue.String)
	if err != nil {
		slog.Warn("could not decrypt a variable's prior value while checking for a change", "scope", "storage/variables", "err", err)
		return false, "", false
	}
	return plain != newValue, plain, true
}

// recordHistory writes the value being replaced, then trims to variableHistoryLimit — the same
// "insert, then DELETE … WHERE id NOT IN (SELECT … ORDER BY … LIMIT ?)" shape
// filter_history.go's Record already uses (D13).
func (r *VariablesRepo) recordHistory(tx *sql.Tx, variableID, oldPlain string, oldSecret bool, now string) error {
	value := oldPlain
	var secretValue *string
	if oldSecret {
		value = ""
		// api_variable_history.secret_value is its own scope, so the value being replaced is
		// re-sealed under ScopeVariableHistory rather than copied across from api_variables — a
		// verbatim copy would be a ciphertext sealed for one column sitting in another, which is
		// exactly what the AAD refuses. Every caller reaches here only through
		// valueChanged's ok return, so oldPlain is a real, already-decrypted plaintext and this
		// Encrypt cannot fail for an unavailable cipher.
		enc, err := r.cipher.Encrypt(secrets.ScopeVariableHistory, oldPlain)
		if err != nil {
			return fmt.Errorf("repos/variables: encrypt history value: %w", err)
		}
		secretValue = &enc
	}
	if _, err := tx.Exec(
		`INSERT INTO api_variable_history (id, variable_id, value, is_secret, secret_value, recorded_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), variableID, value, boolToInt(oldSecret), secretValue, now,
	); err != nil {
		return fmt.Errorf("repos/variables: insert history: %w", err)
	}
	if _, err := tx.Exec(`
		DELETE FROM api_variable_history
		 WHERE variable_id = ?
		   AND id NOT IN (
		     SELECT id FROM api_variable_history
		      WHERE variable_id = ?
		      ORDER BY recorded_at DESC, rowid DESC
		      LIMIT ?
		   )
	`, variableID, variableID, variableHistoryLimit); err != nil {
		return fmt.Errorf("repos/variables: trim history: %w", err)
	}
	return nil
}

// Delete removes one variable (and its history, cascaded) and re-indexes its surviving siblings
// dense — CollectionsRepo.Delete's own discipline. It returns the deleted variable's scope
// and owner id so the bridge can emit an api-data-changed event without a second query —
// which would also race the delete.
func (r *VariablesRepo) Delete(id string) (model.VariableScope, string, error) {
	if id == "" {
		return "", "", fmt.Errorf("repos/variables: id is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return "", "", fmt.Errorf("repos/variables: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var collectionID, environmentID sql.NullString
	err = tx.QueryRow(`SELECT collection_id, environment_id FROM api_variables WHERE id = ?`, id).Scan(&collectionID, &environmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("repos/variables: no variable %s", id)
	}
	if err != nil {
		return "", "", fmt.Errorf("repos/variables: read variable %s: %w", id, err)
	}

	res, err := tx.Exec(`DELETE FROM api_variables WHERE id = ?`, id)
	if err != nil {
		return "", "", fmt.Errorf("repos/variables: delete variable %s: %w", id, err)
	}
	if err := sqlitex.RequireOneRow(res, "variable "+id); err != nil {
		return "", "", err
	}

	scope, ownerID := model.VariableScopeCollection, collectionID.String
	if !collectionID.Valid {
		scope, ownerID = model.VariableScopeEnvironment, environmentID.String
	}
	if err := reindexVariables(tx, scope, ownerID); err != nil {
		return "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", fmt.Errorf("repos/variables: commit: %w", err)
	}
	return scope, ownerID, nil
}

func reindexVariables(tx *sql.Tx, scope model.VariableScope, ownerID string) error {
	column, err := scopeColumn(scope)
	if err != nil {
		return err
	}
	return sqlitex.ReindexSortOrder(tx,
		`SELECT id FROM api_variables WHERE `+column+` = ? ORDER BY sort_order, created_at, id`,
		`UPDATE api_variables SET sort_order = ? WHERE id = ?`,
		ownerID,
	)
}

// Reorder rewrites one scope's sort_order dense, in the order ids names — ConnectionsService.
// Reorder's own "here is the new order, in full" shape (D14).
func (r *VariablesRepo) Reorder(scope model.VariableScope, ownerID string, ids []string) error {
	column, err := scopeColumn(scope)
	if err != nil {
		return err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("repos/variables: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := sqlitex.ReorderSortOrder(tx,
		`SELECT id FROM api_variables WHERE `+column+` = ? ORDER BY sort_order, created_at, id`,
		`UPDATE api_variables SET sort_order = ? WHERE id = ?`,
		ids, ownerID,
	); err != nil {
		return fmt.Errorf("repos/variables: reorder: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos/variables: commit: %w", err)
	}
	return nil
}

// ApplyBulk is the transaction: one scope's whole variable set replaced by a parsed
// `.env` entry list in a single atomic step — match by name (positionally for a duplicate key,
// mirroring dotenv.ts#reconcileEnv exactly, the guard that the two reconciles agree), update/
// insert/delete per the five rules, record history through the existing recordHistory helper for
// every value that changed, then re-index sort_order dense in the entries' own order.
//
// Why one transaction and not N Upsert + M Delete + one Reorder from the renderer: a fifteen-line
// paste would be sixteen IPC round trips with no atomicity — a failure halfway would leave a set
// that is neither the old one nor the new one, and the user's own pasted text is the only record
// of what they meant.
func (r *VariablesRepo) ApplyBulk(scope model.VariableScope, ownerID string, entries []model.VariableBulkEntry) (model.VariableBulkResult, error) {
	column, err := scopeColumn(scope)
	if err != nil {
		return model.VariableBulkResult{}, err
	}
	if ownerID == "" {
		return model.VariableBulkResult{}, fmt.Errorf("repos/variables: ownerId is required")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return model.VariableBulkResult{}, fmt.Errorf("repos/variables: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Phase 1: the pre-image every later phase diffs the pasted entries against.
	existing, err := r.loadBulkExisting(tx, column, ownerID)
	if err != nil {
		return model.VariableBulkResult{}, err
	}

	pools := map[string][]bulkExistingRow{}
	for _, er := range existing {
		pools[er.name] = append(pools[er.name], er)
	}
	nextIndex := map[string]int{}
	matched := map[string]bool{}

	now := kiratime.NowISO()
	var result model.VariableBulkResult
	finalOrder := make([]string, 0, len(entries))

	// Phase 2: match/update/insert one row per pasted entry (D22 rules 1-3).
	for _, entry := range entries {
		id, err := r.applyBulkEntry(tx, scope, ownerID, entry, pools, nextIndex, matched, now, &result)
		if err != nil {
			return model.VariableBulkResult{}, err
		}
		finalOrder = append(finalOrder, id)
	}

	// Phase 3: D22 rule 4 — an existing row whose name appears in no line is deleted, cascading
	// its history.
	priorSurviving, err := r.deleteBulkUnmatched(tx, existing, matched, &result)
	if err != nil {
		return model.VariableBulkResult{}, err
	}

	// Phase 4: D22 rule 5's other half — a reorder-only edit (no add/update/remove) still counts
	// as a change, detected by comparing the surviving rows' own relative order against before.
	result.Reordered = bulkOrderChanged(existing, finalOrder, priorSurviving)

	// Phase 5: D22 rule 5 — line order becomes sort_order, dense, in the entries' own final order.
	if err := renumberBulkSortOrder(tx, finalOrder); err != nil {
		return model.VariableBulkResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return model.VariableBulkResult{}, fmt.Errorf("repos/variables: commit: %w", err)
	}
	return result, nil
}

// bulkExistingRow is ApplyBulk's own pre-image row shape — one scope's whole variable set, as it
// stood before this reconcile.
type bulkExistingRow struct {
	id, name, value, description string
	isSecret                     int
	secretValue                  sql.NullString
}

// loadBulkExisting is ApplyBulk phase 1: every current row for ownerID, in sort_order.
func (r *VariablesRepo) loadBulkExisting(tx *sql.Tx, column, ownerID string) ([]bulkExistingRow, error) {
	rows, err := tx.Query(
		`SELECT id, name, value, is_secret, secret_value, description FROM api_variables
		  WHERE `+column+` = ? ORDER BY sort_order`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("repos/variables: read existing: %w", err)
	}
	defer rows.Close()

	var existing []bulkExistingRow
	for rows.Next() {
		var er bulkExistingRow
		if err := rows.Scan(&er.id, &er.name, &er.value, &er.isSecret, &er.secretValue, &er.description); err != nil {
			return nil, fmt.Errorf("repos/variables: scan existing: %w", err)
		}
		existing = append(existing, er)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos/variables: existing rows: %w", err)
	}
	return existing, nil
}

// applyBulkEntry is ApplyBulk's per-entry body: match entry against the next unmatched
// existing row of the same name (positional, mirroring dotenv.ts#reconcileEnv exactly — the
// guard that the two reconciles agree), update it, or insert a new row when unmatched. Counts the
// outcome into result and always returns the id that becomes this entry's place in finalOrder,
// whichever row it ended up being.
func (r *VariablesRepo) applyBulkEntry(tx *sql.Tx, scope model.VariableScope, ownerID string, entry model.VariableBulkEntry, pools map[string][]bulkExistingRow, nextIndex map[string]int, matched map[string]bool, now string, result *model.VariableBulkResult) (string, error) {
	pool := pools[entry.Name]
	idx := nextIndex[entry.Name]
	if idx >= len(pool) {
		return r.insertBulkEntry(tx, scope, ownerID, entry, now, matched, result)
	}

	er := pool[idx]
	nextIndex[entry.Name] = idx + 1
	matched[er.id] = true
	if err := r.applyBulkMatch(tx, entry, er, now, result); err != nil {
		return "", err
	}
	return er.id, nil
}

// applyBulkMatch is applyBulkEntry's own matched-row branch: a secret row and a plain row update
// under different rules (D22 rules 1/3), so each gets its own helper below.
func (r *VariablesRepo) applyBulkMatch(tx *sql.Tx, entry model.VariableBulkEntry, er bulkExistingRow, now string, result *model.VariableBulkResult) error {
	if er.isSecret != 0 {
		return r.applyBulkMatchSecret(tx, entry, er, now, result)
	}
	return r.applyBulkMatchPlain(tx, entry, er, now, result)
}

func (r *VariablesRepo) applyBulkMatchSecret(tx *sql.Tx, entry model.VariableBulkEntry, er bulkExistingRow, now string, result *model.VariableBulkResult) error {
	switch {
	case entry.HasValue:
		// D22 rule 3: a typed plaintext replaces the secret value; history records the value it
		// replaced through the existing helper.
		if changed, oldPlain, ok := r.valueChanged(er.value, true, er.secretValue, entry.Value); changed && ok {
			if err := r.recordHistory(tx, er.id, oldPlain, true, now); err != nil {
				return err
			}
		}
		storedValue, storedSecret, err := r.encryptFor(entry.Value, true)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(
			`UPDATE api_variables SET value = ?, secret_value = ?, description = ?, updated_at = ? WHERE id = ?`,
			storedValue, storedSecret, entry.Description, now, er.id,
		); err != nil {
			return fmt.Errorf("repos/variables: bulk update secret %s: %w", er.id, err)
		}
		result.Updated++
	case entry.Description != er.description:
		if _, err := tx.Exec(
			`UPDATE api_variables SET description = ?, updated_at = ? WHERE id = ?`,
			entry.Description, now, er.id,
		); err != nil {
			return fmt.Errorf("repos/variables: bulk update description %s: %w", er.id, err)
		}
		result.Updated++
	}
	// entry.HasValue == false and the description is unchanged: nothing touched at all — the
	// property that makes it safe to open this editor on a set full of secrets and press Apply
	// without thinking.
	return nil
}

func (r *VariablesRepo) applyBulkMatchPlain(tx *sql.Tx, entry model.VariableBulkEntry, er bulkExistingRow, now string, result *model.VariableBulkResult) error {
	valueChanged := entry.Value != er.value
	descriptionChanged := entry.Description != er.description
	if !valueChanged && !descriptionChanged {
		return nil
	}
	if valueChanged {
		if changed, oldPlain, ok := r.valueChanged(er.value, false, er.secretValue, entry.Value); changed && ok {
			if err := r.recordHistory(tx, er.id, oldPlain, false, now); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(
		`UPDATE api_variables SET value = ?, description = ?, updated_at = ? WHERE id = ?`,
		entry.Value, entry.Description, now, er.id,
	); err != nil {
		return fmt.Errorf("repos/variables: bulk update %s: %w", er.id, err)
	}
	result.Updated++
	return nil
}

// insertBulkEntry is applyBulkEntry's own unmatched branch, D22 rule 2: an unmatched line creates
// a new, non-secret row — there is no `.env` syntax for the secret flag, so bulk edit cannot
// create a secret (OQ-6).
func (r *VariablesRepo) insertBulkEntry(tx *sql.Tx, scope model.VariableScope, ownerID string, entry model.VariableBulkEntry, now string, matched map[string]bool, result *model.VariableBulkResult) (string, error) {
	newID := uuid.NewString()
	var collectionID, environmentID *string
	if scope == model.VariableScopeCollection {
		collectionID = &ownerID
	} else {
		environmentID = &ownerID
	}
	if _, err := tx.Exec(
		`INSERT INTO api_variables (id, collection_id, environment_id, name, value, is_secret, secret_value, sort_order, description, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 0, NULL, 0, ?, ?, ?)`,
		newID, collectionID, environmentID, entry.Name, entry.Value, entry.Description, now, now,
	); err != nil {
		return "", fmt.Errorf("repos/variables: bulk insert %s: %w", entry.Name, err)
	}
	matched[newID] = true
	result.Added++
	return newID, nil
}

// deleteBulkUnmatched is ApplyBulk phase 3, D22 rule 4: delete every existing row no entry
// matched, cascading its history. Returns the surviving rows' ids in their pre-existing relative
// order, for phase 4's reorder-only detection.
func (r *VariablesRepo) deleteBulkUnmatched(tx *sql.Tx, existing []bulkExistingRow, matched map[string]bool, result *model.VariableBulkResult) ([]string, error) {
	priorSurviving := make([]string, 0, len(existing))
	for _, er := range existing {
		if matched[er.id] {
			priorSurviving = append(priorSurviving, er.id)
			continue
		}
		if _, err := tx.Exec(`DELETE FROM api_variables WHERE id = ?`, er.id); err != nil {
			return nil, fmt.Errorf("repos/variables: bulk delete %s: %w", er.id, err)
		}
		result.Removed++
	}
	return priorSurviving, nil
}

// bulkOrderChanged is ApplyBulk phase 4, D22 rule 5's reorder-only detection: true when the rows
// that survived phases 2-3 come out in a different relative order than they went in.
func bulkOrderChanged(existing []bulkExistingRow, finalOrder, priorSurviving []string) bool {
	existingIDs := make(map[string]bool, len(existing))
	for _, er := range existing {
		existingIDs[er.id] = true
	}
	survivingFinalOrder := make([]string, 0, len(priorSurviving))
	for _, id := range finalOrder {
		if existingIDs[id] {
			survivingFinalOrder = append(survivingFinalOrder, id)
		}
	}
	for i := range survivingFinalOrder {
		if survivingFinalOrder[i] != priorSurviving[i] {
			return true
		}
	}
	return false
}

// renumberBulkSortOrder is ApplyBulk phase 5, D22 rule 5: line order becomes sort_order, dense,
// in the entries' own final order.
func renumberBulkSortOrder(tx *sql.Tx, finalOrder []string) error {
	for i, id := range finalOrder {
		if _, err := tx.Exec(`UPDATE api_variables SET sort_order = ? WHERE id = ?`, i, id); err != nil {
			return fmt.Errorf("repos/variables: bulk reindex %s: %w", id, err)
		}
	}
	return nil
}

// History returns one variable's prior values, newest first — never a secret's plaintext or
// ciphertext (the same list-projection discipline as List above). Reveal is the gated path to one.
func (r *VariablesRepo) History(variableID string) ([]model.VariableHistoryEntry, error) {
	rows, err := r.db.Query(
		`SELECT id, variable_id, value, is_secret, recorded_at FROM api_variable_history
		  WHERE variable_id = ? ORDER BY recorded_at DESC, rowid DESC`,
		variableID,
	)
	if err != nil {
		return nil, fmt.Errorf("repos/variables: query history: %w", err)
	}
	defer rows.Close()

	out := []model.VariableHistoryEntry{}
	for rows.Next() {
		var (
			e        model.VariableHistoryEntry
			isSecret int
		)
		if err := rows.Scan(&e.ID, &e.VariableID, &e.Value, &isSecret, &e.RecordedAt); err != nil {
			return nil, fmt.Errorf("repos/variables: scan history entry: %w", err)
		}
		e.IsSecret = isSecret != 0
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos/variables: history rows: %w", err)
	}
	return out, nil
}

// ---- the gated reveal's own accessors (D8) — called only after apivars.Service has already
// authorized the reveal; neither method gates anything itself. ----

// revealSecret is RevealValue's and RevealHistoryValue's own shared shape: read
// is_secret/secret_value off table by id, refuse a missing row or a non-secret one, decrypt under
// scope. noun names the row in every error string ("variable" / "history entry") — RevealValue's
// own decrypt error previously omitted it ("decrypt %s: %w") where RevealHistoryValue's included
// it; unified to always include it here (a log-string-only difference no caller or test ever
// asserted on, confirmed via a repo-wide search).
func (r *VariablesRepo) revealSecret(table string, scope secrets.Scope, noun, id string) (string, error) {
	var (
		isSecretInt int
		secretValue sql.NullString
	)
	err := r.db.QueryRow(`SELECT is_secret, secret_value FROM `+table+` WHERE id = ?`, id).Scan(&isSecretInt, &secretValue)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("repos/variables: no %s %s", noun, id)
	}
	if err != nil {
		return "", fmt.Errorf("repos/variables: read %s %s: %w", noun, id, err)
	}
	if isSecretInt == 0 || !secretValue.Valid {
		return "", fmt.Errorf("repos/variables: %s %s is not a secret", noun, id)
	}
	plain, err := r.cipher.Decrypt(scope, secretValue.String)
	if err != nil {
		return "", fmt.Errorf("repos/variables: decrypt %s %s: %w", noun, id, err)
	}
	return plain, nil
}

// RevealValue decrypts one variable's stored secret. Returns an error for a variable that either
// does not exist or is not a secret — apivars.Service.Reveal turns that into its own
// never-throws RevealResult, exactly as connections.Service.Reveal already does for a decrypt
// failure.
func (r *VariablesRepo) RevealValue(variableID string) (string, error) {
	return r.revealSecret("api_variables", secrets.ScopeVariable, "variable", variableID)
}

// RevealHistoryValue is RevealValue's sibling over api_variable_history — a secret's old value is
// exactly as sensitive as its current one (D13).
func (r *VariablesRepo) RevealHistoryValue(historyID string) (string, error) {
	return r.revealSecret("api_variable_history", secrets.ScopeVariableHistory, "history entry", historyID)
}

// SecretsFor decrypts every secret variable reachable from a send — the precedence, environment
// over collection, applied here as "insert the collection's secrets, then let the environment's
// overwrite same-named ones", so the returned map already reflects who wins. Either id may be ""
// (a scratch tab has no collection; no environment may be selected). A single entry's decrypt
// failure is logged (naming the variable, never the value) and absent from the result —
// the reference then resolves to nothing and the reference stays literal, rather than failing
// the whole send over one bad row.
func (r *VariablesRepo) SecretsFor(collectionID, environmentID string) (map[string]string, error) {
	out := map[string]string{}
	if collectionID != "" {
		if err := r.mergeSecrets(out, "collection_id", collectionID); err != nil {
			return nil, err
		}
	}
	if environmentID != "" {
		if err := r.mergeSecrets(out, "environment_id", environmentID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// mergeSecrets is one scope's own resolution pass (collection, then environment — SecretsFor's own
// call order). A duplicate name within *this* scope resolves first-wins by sort_order —
// ORDER BY makes that the query's own row order, and seen skips a later row sharing an
// already-decrypted name rather than overwriting it. seen is local to this call, never shared with
// the caller's out map, so a later call for a higher-precedence scope (environment) still
// overrides an already-set name from a prior, lower-precedence call — first-wins applies within a
// scope, never across scopes.
func (r *VariablesRepo) mergeSecrets(out map[string]string, column, ownerID string) error {
	rows, err := r.db.Query(`SELECT name, secret_value FROM api_variables WHERE `+column+` = ? AND is_secret = 1 ORDER BY sort_order`, ownerID)
	if err != nil {
		return fmt.Errorf("repos/variables: query secrets: %w", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var name string
		var secretValue sql.NullString
		if err := rows.Scan(&name, &secretValue); err != nil {
			return fmt.Errorf("repos/variables: scan secret: %w", err)
		}
		if seen[name] {
			continue
		}
		// Mark seen before the validity/decrypt checks below, so a NULL or undecryptable row
		// still shadows — both a later same-named row further down this scope's sort_order
		// (first-wins), and a same-named value SecretsFor's own earlier, lower-precedence call
		// (collection, before environment) already wrote into out. Without this, the next row
		// with the same name won, silently substituting a different scope's value: Send would use
		// the collection's secret while findSecretVariableId (curl.ts) already treats the broken
		// env row as unresolved, so Copy-as-curl and Send disagreed. delete(out, name) makes this
		// row's failure win too, leaving the {{reference}} unresolved (literal) rather than wrong.
		seen[name] = true
		if !secretValue.Valid {
			delete(out, name)
			continue
		}
		plain, err := r.cipher.Decrypt(secrets.ScopeVariable, secretValue.String)
		if err != nil {
			slog.Warn("a secret variable could not be decrypted while resolving a request", "scope", "storage/variables", "name", name, "err", err)
			delete(out, name)
			continue
		}
		out[name] = plain
	}
	return rows.Err()
}

// ---- import promotion (P4 D9's hand-off, closed by D15/F5) ----

// promoteIfNeeded is the one-shot: a collection still carries its top-level `variable[]` inside
// origin_json (variables_promoted = 0). This moves it into rows,
// sheds the member from origin_json so the exporter never re-emits it from there, and stamps the
// flag — all inside one transaction, idempotent (a second call is a no-op because the flag is
// already 1).
func (r *VariablesRepo) promoteIfNeeded(collectionID string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("repos/variables: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		originJSON string
		promoted   bool
	)
	err = tx.QueryRow(`SELECT origin_json, variables_promoted FROM api_collections WHERE id = ?`, collectionID).Scan(&originJSON, &promoted)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("repos/variables: no collection %s", collectionID)
	}
	if err != nil {
		return fmt.Errorf("repos/variables: read collection %s: %w", collectionID, err)
	}
	if promoted {
		return nil
	}

	origin := map[string]json.RawMessage{}
	if originJSON != "" {
		if err := json.Unmarshal([]byte(originJSON), &origin); err != nil {
			slog.Warn("collection origin is not a JSON object; nothing to promote", "scope", "storage/variables", "collectionId", collectionID)
			origin = map[string]json.RawMessage{}
		}
	}

	var entries []postman.Variable
	if raw, ok := origin["variable"]; ok {
		decoded, isArray := postman.DecodeVariables(raw)
		if isArray {
			entries = decoded
			delete(origin, "variable")
		} else {
			slog.Warn("collection variable[] is not an array; left in origin", "scope", "storage/variables", "collectionId", collectionID)
		}
	}

	if len(entries) > 0 {
		order, err := sqlitex.NextSortOrder(tx, "api_variables", "collection_id = ?", collectionID)
		if err != nil {
			return fmt.Errorf("repos/variables: next variable order: %w", err)
		}
		now := kiratime.NowISO()
		for _, entry := range entries {
			storedValue, storedSecret, err := r.encryptFor(entry.Value, entry.Secret)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(
				`INSERT INTO api_variables (id, collection_id, environment_id, name, value, is_secret, secret_value, sort_order, created_at, updated_at)
				 VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?, ?)`,
				uuid.NewString(), collectionID, entry.Name, storedValue, boolToInt(entry.Secret), storedSecret, order, now, now,
			); err != nil {
				return fmt.Errorf("repos/variables: insert promoted variable: %w", err)
			}
			order++
		}
	}

	encodedOrigin, err := encodeJSON(origin)
	if err != nil {
		return fmt.Errorf("repos/variables: encode origin: %w", err)
	}
	if _, err := tx.Exec(
		`UPDATE api_collections SET origin_json = ?, variables_promoted = 1, updated_at = ? WHERE id = ?`,
		string(encodedOrigin), kiratime.NowISO(), collectionID,
	); err != nil {
		return fmt.Errorf("repos/variables: stamp promoted: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos/variables: commit: %w", err)
	}
	return nil
}

// PromoteImported is promoteIfNeeded's exported entry point (used directly by
// repos/collections.go's ImportTree/LoadTree wiring in the postman commit, and available for a
// caller that wants to force promotion outside of List's own lazy trigger).
func (r *VariablesRepo) PromoteImported(collectionID string) error {
	return r.promoteIfNeeded(collectionID)
}

// ImportVariables writes a freshly-imported collection's own promoted `variable[]` as rows,
// stamping variables_promoted = 1 immediately — these rows already *are* the promoted set, so
// there is nothing left for promoteIfNeeded to do for this collection.
//
// Called by CollectionsService.Import right after CollectionsRepo.ImportTree creates the
// collection row — deliberately a second call, not folded into that one transaction: encrypting a
// secret value needs this repo's own Cipher, which CollectionsRepo does not have and should not be
// given (the module boundary keeps secret_value's only writers in this one repo).
func (r *VariablesRepo) ImportVariables(collectionID string, vars []postman.Variable) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("repos/variables: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := kiratime.NowISO()
	order := 0
	for _, v := range vars {
		if v.Name == "" {
			continue
		}
		storedValue, storedSecret, err := r.encryptFor(v.Value, v.Secret)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO api_variables (id, collection_id, environment_id, name, value, is_secret, secret_value, sort_order, description, created_at, updated_at)
			 VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), collectionID, v.Name, storedValue, boolToInt(v.Secret), storedSecret, order, v.Description, now, now,
		); err != nil {
			return fmt.Errorf("repos/variables: insert imported variable: %w", err)
		}
		order++
	}
	if _, err := tx.Exec(`UPDATE api_collections SET variables_promoted = 1, updated_at = ? WHERE id = ?`, now, collectionID); err != nil {
		return fmt.Errorf("repos/variables: stamp promoted: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos/variables: commit: %w", err)
	}
	return nil
}

// ImportEnvironment writes a Postman environment as one environment row plus its variables, in
// one transaction: a secret that cannot be encrypted leaves nothing behind.
func (r *VariablesRepo) ImportEnvironment(name string, vars []postman.Variable) (model.Environment, error) {
	if name == "" {
		return model.Environment{}, fmt.Errorf("repos/variables: name is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return model.Environment{}, fmt.Errorf("repos/variables: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	order, err := sqlitex.NextSortOrder(tx, "api_environments", "")
	if err != nil {
		return model.Environment{}, fmt.Errorf("repos/variables: next environment order: %w", err)
	}
	now := kiratime.NowISO()
	env := model.Environment{ID: uuid.NewString(), Name: name, SortOrder: order, Color: "none"}
	if _, err := tx.Exec(
		`INSERT INTO api_environments (id, name, sort_order, is_active, description, color, created_at, updated_at)
		 VALUES (?, ?, ?, 0, '', ?, ?, ?)`,
		env.ID, env.Name, env.SortOrder, env.Color, now, now,
	); err != nil {
		return model.Environment{}, fmt.Errorf("repos/variables: insert environment: %w", err)
	}
	position := 0
	for _, v := range vars {
		if v.Name == "" {
			continue
		}
		storedValue, storedSecret, err := r.encryptFor(v.Value, v.Secret)
		if err != nil {
			return model.Environment{}, err
		}
		if _, err := tx.Exec(
			`INSERT INTO api_variables (id, collection_id, environment_id, name, value, is_secret, secret_value, sort_order, description, created_at, updated_at)
			 VALUES (?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), env.ID, v.Name, storedValue, boolToInt(v.Secret), storedSecret, position, v.Description, now, now,
		); err != nil {
			return model.Environment{}, fmt.Errorf("repos/variables: insert imported variable: %w", err)
		}
		position++
	}
	if err := tx.Commit(); err != nil {
		return model.Environment{}, fmt.Errorf("repos/variables: commit: %w", err)
	}
	return env, nil
}
