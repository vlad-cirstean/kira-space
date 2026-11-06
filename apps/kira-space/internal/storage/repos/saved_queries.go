package repos

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"log/slog"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// ErrSavedQueryNotFound is a read or patch of a saved-query id that has no row.
var ErrSavedQueryNotFound = errors.New("repos/saved_queries: not found")

// ErrSavedQueryNameTaken is a rename or save into a name another discover query of the connection holds.
var ErrSavedQueryNameTaken = errors.New("repos/saved_queries: name taken")

const savedQuerySelectColumns = `id, connection_id, path, name, kind, body, pinned, created_at, used_at`

type SavedQueriesRepo struct {
	DB *sql.DB
}

// scanSavedQueryRow scans one row and validates it the same way saved-queries.ts's
// parseSavedQueryRow does: an unparseable body, or one that doesn't match its own kind's shape,
// drops the row (nil, nil) rather than propagating it.
func scanSavedQueryRow(row rowScanner) (*model.SavedQuery, error) {
	var (
		q      model.SavedQuery
		pinned int
		usedAt sql.NullString
		body   string
	)
	if err := row.Scan(&q.ID, &q.ConnectionID, &q.Path, &q.Name, &q.Kind, &body, &pinned, &q.CreatedAt, &usedAt); err != nil {
		return nil, err
	}
	q.Pinned = pinned != 0
	if usedAt.Valid {
		q.UsedAt = &usedAt.String
	}

	if !json.Valid([]byte(body)) {
		slog.Warn("dropping saved query: body is not valid JSON", "scope", "storage/saved-queries", "id", q.ID)
		return nil, nil
	}
	switch q.Kind {
	case "filter":
		// decodeStrict's DisallowUnknownFields is what actually distinguishes a filter body from
		// a console body: FilterBody/ConsoleBody's fields don't overlap, and json.Unmarshal alone
		// silently ignores a console body's "text" key when decoding into FilterBody instead of
		// rejecting it.
		if _, err := decodeStrict[model.FilterBody](body); err != nil {
			slog.Warn("dropping saved query: body does not match filter shape", "scope", "storage/saved-queries", "id", q.ID)
			return nil, nil
		}
	case "console":
		if _, err := decodeStrict[model.ConsoleBody](body); err != nil {
			slog.Warn("dropping saved query: body does not match console shape", "scope", "storage/saved-queries", "id", q.ID)
			return nil, nil
		}
	case "discover":
		body, err := decodeStrict[model.DiscoverBody](body)
		if err == nil {
			err = body.Validate()
		}
		if err != nil {
			slog.Warn("dropping saved query: body does not match discover shape", "scope", "storage/saved-queries", "id", q.ID)
			return nil, nil
		}
	default:
		slog.Warn("dropping saved query: unrecognised kind", "scope", "storage/saved-queries", "id", q.ID, "kind", q.Kind)
		return nil, nil
	}
	q.Body = json.RawMessage(body)
	return &q, nil
}

// decodeStrict decodes body into T, rejecting unknown fields — the same check scanSavedQueryRow
// uses to tell a filter body from a console body (see comment above).
func decodeStrict[T any](body string) (T, error) {
	var v T
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	err := dec.Decode(&v)
	return v, err
}

func (r *SavedQueriesRepo) listByKind(connectionID, path, kind string) ([]model.SavedQuery, error) {
	return r.queryRows(
		`SELECT `+savedQuerySelectColumns+`
		   FROM saved_queries
		  WHERE connection_id = ? AND path = ? AND kind = ?
		  ORDER BY pinned DESC, used_at DESC, name`,
		connectionID, path, kind,
	)
}

// ListDiscover lists a connection's discover queries regardless of path.
func (r *SavedQueriesRepo) ListDiscover(connectionID string) ([]model.SavedQuery, error) {
	return r.queryRows(
		`SELECT `+savedQuerySelectColumns+`
		   FROM saved_queries
		  WHERE connection_id = ? AND kind = 'discover'
		  ORDER BY pinned DESC, used_at DESC, name`,
		connectionID,
	)
}

func (r *SavedQueriesRepo) queryRows(query string, args ...any) ([]model.SavedQuery, error) {
	rows, err := r.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("repos/saved_queries: query: %w", err)
	}
	defer rows.Close()

	out := []model.SavedQuery{}
	for rows.Next() {
		q, err := scanSavedQueryRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repos/saved_queries: scan: %w", err)
		}
		if q != nil {
			out = append(out, *q)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos/saved_queries: rows: %w", err)
	}
	return out, nil
}

func (r *SavedQueriesRepo) ListFilters(connectionID, path string) ([]model.SavedQuery, error) {
	return r.listByKind(connectionID, path, "filter")
}

func (r *SavedQueriesRepo) ListConsole(connectionID, path string) ([]model.SavedQuery, error) {
	return r.listByKind(connectionID, path, "console")
}

func (r *SavedQueriesRepo) insert(connectionID, path, name, kind string, body []byte) (model.SavedQuery, error) {
	if err := model.ValidSavedQueryName(name); err != nil {
		return model.SavedQuery{}, fmt.Errorf("repos/saved_queries: %w", err)
	}
	name = strings.TrimSpace(name)
	newID := ids.NewID()
	now := kiratime.NowISO()
	if _, err := r.DB.Exec(
		`INSERT INTO saved_queries (id, connection_id, path, name, kind, body, pinned, created_at, used_at)
		 VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		newID, connectionID, path, name, kind, string(body), now, now,
	); err != nil {
		if isUniqueViolation(err) {
			return model.SavedQuery{}, fmt.Errorf("%w: %s", ErrSavedQueryNameTaken, name)
		}
		return model.SavedQuery{}, fmt.Errorf("repos/saved_queries: insert: %w", err)
	}
	return r.mustGet(newID)
}

// Get returns one saved query, ErrSavedQueryNotFound when it does not exist.
func (r *SavedQueriesRepo) Get(queryID string) (model.SavedQuery, error) { return r.mustGet(queryID) }

func (r *SavedQueriesRepo) mustGet(queryID string) (model.SavedQuery, error) {
	row := r.DB.QueryRow(`SELECT `+savedQuerySelectColumns+` FROM saved_queries WHERE id = ?`, queryID)
	q, err := scanSavedQueryRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.SavedQuery{}, fmt.Errorf("%w: %s", ErrSavedQueryNotFound, queryID)
	}
	if err != nil {
		return model.SavedQuery{}, fmt.Errorf("repos/saved_queries: get %s: %w", queryID, err)
	}
	if q == nil {
		return model.SavedQuery{}, fmt.Errorf("repos/saved_queries: row %s not found or failed to parse", queryID)
	}
	return *q, nil
}

// save encodes body, inserts it under kind, and applies pinned — SaveFilter/SaveConsole's shared
// shape (P113 G6).
func (r *SavedQueriesRepo) save(connectionID, path, name, kind string, body any, pinned bool) (model.SavedQuery, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return model.SavedQuery{}, fmt.Errorf("repos/saved_queries: encode %s body: %w", kind, err)
	}
	q, err := r.insert(connectionID, path, name, kind, encoded)
	if err != nil {
		return model.SavedQuery{}, err
	}
	if pinned {
		if err := r.setPinned(q.ID, true); err != nil {
			return model.SavedQuery{}, err
		}
		return r.mustGet(q.ID)
	}
	return q, nil
}

func (r *SavedQueriesRepo) SaveFilter(connectionID, path, name string, body model.FilterBody, pinned bool) (model.SavedQuery, error) {
	return r.save(connectionID, path, name, "filter", body, pinned)
}

func (r *SavedQueriesRepo) SaveConsole(connectionID, path, name string, body model.ConsoleBody, pinned bool) (model.SavedQuery, error) {
	return r.save(connectionID, path, name, "console", body, pinned)
}

// SaveDiscover stores a Discover state under a name unique within the connection.
func (r *SavedQueriesRepo) SaveDiscover(connectionID, name string, body model.DiscoverBody, pinned bool) (model.SavedQuery, error) {
	if err := body.Validate(); err != nil {
		return model.SavedQuery{}, err
	}
	return r.save(connectionID, "", name, "discover", body.Normalized(), pinned)
}

// UpdateDiscover replaces the body of a discover query in place.
func (r *SavedQueriesRepo) UpdateDiscover(queryID string, body model.DiscoverBody) (model.SavedQuery, error) {
	if err := body.Validate(); err != nil {
		return model.SavedQuery{}, err
	}
	encoded, err := json.Marshal(body.Normalized())
	if err != nil {
		return model.SavedQuery{}, fmt.Errorf("repos/saved_queries: encode discover body: %w", err)
	}
	res, err := r.DB.Exec(`UPDATE saved_queries SET body = ?, used_at = ? WHERE id = ? AND kind = 'discover'`,
		string(encoded), kiratime.NowISO(), queryID)
	if err != nil {
		return model.SavedQuery{}, fmt.Errorf("repos/saved_queries: update body %s: %w", queryID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.SavedQuery{}, fmt.Errorf("%w: %s", ErrSavedQueryNotFound, queryID)
	}
	return r.mustGet(queryID)
}

func (r *SavedQueriesRepo) setPinned(queryID string, pinned bool) error {
	if _, err := r.DB.Exec(`UPDATE saved_queries SET pinned = ? WHERE id = ?`, boolToInt(pinned), queryID); err != nil {
		return fmt.Errorf("repos/saved_queries: set pinned %s: %w", queryID, err)
	}
	return nil
}

// Update is kind-agnostic (queries.ts's own updateSavedQuery): id alone identifies the row, and
// neither field touches body/kind.
func (r *SavedQueriesRepo) Update(queryID string, patch model.SavedQueryPatch) (model.SavedQuery, error) {
	if patch.Name != nil {
		if err := model.ValidSavedQueryName(*patch.Name); err != nil {
			return model.SavedQuery{}, fmt.Errorf("repos/saved_queries: %w", err)
		}
		if _, err := r.DB.Exec(`UPDATE saved_queries SET name = ? WHERE id = ?`, strings.TrimSpace(*patch.Name), queryID); err != nil {
			if isUniqueViolation(err) {
				return model.SavedQuery{}, fmt.Errorf("%w: %s", ErrSavedQueryNameTaken, strings.TrimSpace(*patch.Name))
			}
			return model.SavedQuery{}, fmt.Errorf("repos/saved_queries: update name %s: %w", queryID, err)
		}
	}
	if patch.Pinned != nil {
		if err := r.setPinned(queryID, *patch.Pinned); err != nil {
			return model.SavedQuery{}, err
		}
	}
	return r.mustGet(queryID)
}

func (r *SavedQueriesRepo) Delete(queryID string) error {
	if _, err := r.DB.Exec(`DELETE FROM saved_queries WHERE id = ?`, queryID); err != nil {
		return fmt.Errorf("repos/saved_queries: delete %s: %w", queryID, err)
	}
	return nil
}

func (r *SavedQueriesRepo) Touch(queryID string) error {
	if _, err := r.DB.Exec(`UPDATE saved_queries SET used_at = ? WHERE id = ?`, kiratime.NowISO(), queryID); err != nil {
		return fmt.Errorf("repos/saved_queries: touch %s: %w", queryID, err)
	}
	return nil
}
