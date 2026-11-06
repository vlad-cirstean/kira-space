package repos

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/sqlitex"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// VariablesRepo reads api_environments, api_variables and api_variable_history. The rows are a
// projection of the api config files, written only by apifiles.Store; the repo itself writes the
// active environment alone. Secret values live in secret_values, named by api_variables.secret_ref.
type VariablesRepo struct {
	db     *sql.DB
	cipher Cipher
}

// NewVariables takes the cipher that opens a history entry's sealed value.
func NewVariables(db *sql.DB, cipher Cipher) *VariablesRepo {
	return &VariablesRepo{db: db, cipher: cipher}
}

// ---- environments ----
func coerceEnvironmentColor(id, color string) string {
	if model.ValidPaletteColor(color) {
		return color
	}
	slog.Warn("repos/variables: environment has unrecognised colour, coercing to none", "scope", "storage/variables", "id", id, "color", color)
	return "none"
}

func (r *VariablesRepo) ListEnvironments() ([]model.Environment, error) {
	rows, err := r.db.Query(`SELECT id, name, sort_order, is_active, description, color, COALESCE(stem || '.environment.yaml', '') FROM api_environments ORDER BY sort_order, name`)
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
		if err := rows.Scan(&e.ID, &e.Name, &e.SortOrder, &isActive, &e.Description, &e.Color, &e.File); err != nil {
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

// ---- variables ----

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

// SecretRefOf names the secret a variable row points at. A row that is not a secret is an error.
func (r *VariablesRepo) SecretRefOf(variableID string) (string, error) {
	var isSecret int
	var ref sql.NullString
	err := r.db.QueryRow(`SELECT is_secret, secret_ref FROM api_variables WHERE id = ?`, variableID).Scan(&isSecret, &ref)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("repos/variables: no variable %s", variableID)
	}
	if err != nil {
		return "", fmt.Errorf("repos/variables: read variable %s: %w", variableID, err)
	}
	if isSecret == 0 || !ref.Valid || ref.String == "" {
		return "", fmt.Errorf("repos/variables: variable %s is not a secret", variableID)
	}
	return ref.String, nil
}

// RevealHistoryValue decrypts one history entry's sealed value; apivars.Service gates the call.
func (r *VariablesRepo) RevealHistoryValue(historyID string) (string, error) {
	var isSecret int
	var sealed sql.NullString
	err := r.db.QueryRow(`SELECT is_secret, secret_value FROM api_variable_history WHERE id = ?`, historyID).Scan(&isSecret, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("repos/variables: no history entry %s", historyID)
	}
	if err != nil {
		return "", fmt.Errorf("repos/variables: read history entry %s: %w", historyID, err)
	}
	if isSecret == 0 || !sealed.Valid {
		return "", fmt.Errorf("repos/variables: history entry %s is not a secret", historyID)
	}
	plain, err := r.cipher.Decrypt(secrets.ScopeVariableHistory, sealed.String)
	if err != nil {
		return "", fmt.Errorf("repos/variables: decrypt history entry %s: %w", historyID, err)
	}
	return plain, nil
}

// SecretRefsFor maps each secret variable name reachable from a send to its secret ref. The
// environment wins over the collection, and within one scope the first row by sort order wins.
func (r *VariablesRepo) SecretRefsFor(collectionID, environmentID string) (map[string]string, error) {
	out := map[string]string{}
	for _, owner := range []struct{ column, id string }{{"collection_id", collectionID}, {"environment_id", environmentID}} {
		if owner.id == "" {
			continue
		}
		if err := r.mergeSecretRefs(out, owner.column, owner.id); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *VariablesRepo) mergeSecretRefs(out map[string]string, column, ownerID string) error {
	rows, err := r.db.Query(`SELECT name, is_secret, secret_ref FROM api_variables WHERE `+column+` = ? ORDER BY sort_order`, ownerID)
	if err != nil {
		return fmt.Errorf("repos/variables: query secrets: %w", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var name string
		var isSecret int
		var ref sql.NullString
		if err := rows.Scan(&name, &isSecret, &ref); err != nil {
			return fmt.Errorf("repos/variables: scan secret: %w", err)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		if isSecret == 0 || !ref.Valid || ref.String == "" {
			delete(out, name)
			continue
		}
		out[name] = ref.String
	}
	return rows.Err()
}
