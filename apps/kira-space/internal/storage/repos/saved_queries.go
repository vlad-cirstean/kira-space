package repos

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"log/slog"
	"strings"

	"github.com/huandu/go-sqlbuilder"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// ErrSavedQueryNotFound is a read or patch of a saved-query id that has no row.
var ErrSavedQueryNotFound = errors.New("repos/saved_queries: not found")

// ErrSavedQueryNameTaken is a rename or save into a name another discover query of the connection holds.
var ErrSavedQueryNameTaken = errors.New("repos/saved_queries: name taken")

var savedQuerySelectColumns = func() []string {
	c := kiraschema.ConnectionSavedQueries
	return []string{c.ID, c.ConnectionID, c.Path, c.Name, c.Kind, c.Body, c.Pinned, c.CreatedAt, c.UsedAt}
}()

type SavedQueriesRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// savedQueryList is the discover/filter/console list query: pinned first, then most recently used.
func (r *SavedQueriesRepo) savedQueryList(connectionID string, conds func(sb *sqlbuilder.SelectBuilder) []string) *sqlbuilder.SelectBuilder {
	c := kiraschema.ConnectionSavedQueries
	sb := r.Dialect.Select(savedQuerySelectColumns...).From(c.Table)
	sb.Where(append([]string{sb.Equal(c.ConnectionID, connectionID)}, conds(sb)...)...).
		OrderBy(c.Pinned+" DESC", c.UsedAt+" DESC", c.Name)
	return sb
}

// scanSavedQueryRow scans one row and validates it the same way saved-queries.ts's
// parseSavedQueryRow does: an unparseable body, or one that doesn't match its own kind's shape,
// drops the row (nil, nil) rather than propagating it.
func scanSavedQueryRow(row rowScanner) (*model.SavedQuery, error) {
	var (
		q         model.SavedQuery
		pinned    int
		usedAt    sql.NullInt64
		createdMs int64
		body      string
	)
	if err := row.Scan(&q.ID, &q.ConnectionID, &q.Path, &q.Name, &q.Kind, &body, &pinned, &createdMs, &usedAt); err != nil {
		return nil, err
	}
	q.CreatedAt = kiratime.MsToISO(createdMs)
	q.Pinned = pinned != 0
	if usedAt.Valid {
		used := kiratime.MsToISO(usedAt.Int64)
		q.UsedAt = &used
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
	return r.queryRows(r.listByKindQuery(connectionID, path, kind))
}

func (r *SavedQueriesRepo) listByKindQuery(connectionID, path, kind string) *sqlbuilder.SelectBuilder {
	c := kiraschema.ConnectionSavedQueries
	return r.savedQueryList(connectionID, func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.Equal(c.Path, path), sb.Equal(c.Kind, kind)}
	})
}

// ListDiscover lists a connection's discover queries regardless of path.
func (r *SavedQueriesRepo) ListDiscover(connectionID string) ([]model.SavedQuery, error) {
	c := kiraschema.ConnectionSavedQueries
	return r.queryRows(r.savedQueryList(connectionID, func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.Equal(c.Kind, "discover")}
	}))
}

func (r *SavedQueriesRepo) queryRows(sb *sqlbuilder.SelectBuilder) ([]model.SavedQuery, error) {
	rows, err := sqlb.Query(r.DB, sb)
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
	now := kiratime.NowMs()
	c := kiraschema.ConnectionSavedQueries
	ib := r.Dialect.InsertInto(c.Table).Cols(c.ID, c.ConnectionID, c.Path, c.Name, c.Kind, c.Body, c.Pinned, c.CreatedAt, c.UsedAt).
		Values(newID, connectionID, path, name, kind, string(body), 0, now, now)
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
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
	c := kiraschema.ConnectionSavedQueries
	sb := r.Dialect.Select(savedQuerySelectColumns...).From(c.Table)
	sb.Where(sb.Equal(c.ID, queryID))
	q, err := scanSavedQueryRow(sqlb.QueryRow(r.DB, sb))
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
	c := kiraschema.ConnectionSavedQueries
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.Body, string(encoded)), ub.Assign(c.UsedAt, kiratime.NowMs())).
		Where(ub.Equal(c.ID, queryID), ub.Equal(c.Kind, "discover"))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return model.SavedQuery{}, fmt.Errorf("repos/saved_queries: update body %s: %w", queryID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.SavedQuery{}, fmt.Errorf("%w: %s", ErrSavedQueryNotFound, queryID)
	}
	return r.mustGet(queryID)
}

func (r *SavedQueriesRepo) setPinned(queryID string, pinned bool) error {
	c := kiraschema.ConnectionSavedQueries
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.Pinned, boolToInt(pinned))).Where(ub.Equal(c.ID, queryID))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
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
		c := kiraschema.ConnectionSavedQueries
		ub := r.Dialect.Update(c.Table)
		ub.Set(ub.Assign(c.Name, strings.TrimSpace(*patch.Name))).Where(ub.Equal(c.ID, queryID))
		if _, err := sqlb.Exec(r.DB, ub); err != nil {
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
	c := kiraschema.ConnectionSavedQueries
	del := r.Dialect.DeleteFrom(c.Table)
	del.Where(del.Equal(c.ID, queryID))
	if _, err := sqlb.Exec(r.DB, del); err != nil {
		return fmt.Errorf("repos/saved_queries: delete %s: %w", queryID, err)
	}
	return nil
}

func (r *SavedQueriesRepo) Touch(queryID string) error {
	c := kiraschema.ConnectionSavedQueries
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.UsedAt, kiratime.NowMs())).Where(ub.Equal(c.ID, queryID))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos/saved_queries: touch %s: %w", queryID, err)
	}
	return nil
}
