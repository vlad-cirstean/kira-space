package memory

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/memory/memfmt"
	"github.com/kirathecat/kira-studio/internal/memory/migrations"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

const (
	defaultSearchLimit = 40
	maxSearchLimit     = 200

	// DefaultSearchLimit and MaxSearchLimit bound a search; the MCP tool description states them.
	DefaultSearchLimit = defaultSearchLimit
	MaxSearchLimit     = maxSearchLimit
)

var errStale = errors.New("memory store changed concurrently; retry")

// Store is memory.db. Construction is free: the file opens on first use, so a host that never
// touches memory never creates it.
type Store struct {
	path string

	mu sync.Mutex
	db *sql.DB

	vecMu sync.Mutex
	vecs  *vecCache
}

func NewStore(path string) *Store { return &Store{path: path} }

// Path is the memory.db file location.
func (s *Store) Path() string { return s.path }

// OpenDefault returns a Store at DefaultPath.
func OpenDefault() *Store { return NewStore(DefaultPath()) }

func (s *Store) conn() (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return nil, fmt.Errorf("memory: create home: %w", err)
	}
	db, err := sqlitex.OpenImmediate(s.path)
	if err != nil {
		return nil, err
	}
	steps, err := migrations.All()
	if err == nil {
		err = sqlitex.Migrate(db, steps)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	s.db = db
	return db, nil
}

// DB returns the open memory.db handle, for internal/memory subpackages sharing the file.
func (s *Store) DB() (*sql.DB, error) { return s.conn() }

// Opened reports whether the file has been opened by this Store.
func (s *Store) Opened() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db != nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

type SearchArgs struct {
	Query          string
	IncludeHistory bool
	Limit          int // clamped to [1, 200]; 0 means 40

	// Scopes and Origins hard-filter results; empty means all.
	Scopes  []string
	Origins []string
	// Project is the rank boost target, not a filter. Empty means no boost.
	Project string

	// within limits matches to one scope and project. Reconcile only; not on the MCP surface.
	within *scopeKey
}

// scopeKey is a scope plus its project (empty for company and personal).
type scopeKey struct{ scope, project string }

// args are the nullable SQL parameters of the scope filter; a nil key matches everything.
func (k *scopeKey) args() (scope, project any) {
	if k == nil {
		return nil, nil
	}
	if k.project == "" {
		return k.scope, nil
	}
	return k.scope, k.project
}

const memoryColumns = `m.id, m.lineage_id, m.version, m.fact, m.kind, m.reason, m.keywords, m.origin, m.source_tag, m.scope,
	m.project, m.session_id, m.status, m.supersedes_id, m.superseded_by_id, m.created_at, m.superseded_at,
	(SELECT count(*) FROM memories x WHERE x.lineage_id = m.lineage_id) AS versions, m.seq`

type rowScanner interface{ Scan(dest ...any) error }

func scanMemory(r rowScanner, extra ...any) (Memory, error) {
	var m Memory
	var kw string
	dest := append([]any{&m.ID, &m.LineageID, &m.Version, &m.Fact, &m.Kind, &m.Reason, &kw, &m.Origin, &m.SourceTag, &m.Scope,
		&m.Project, &m.SessionID, &m.Status, &m.SupersedesID, &m.SupersededBy, &m.CreatedAt, &m.SupersededAt, &m.Versions, &m.seq}, extra...)
	if err := r.Scan(dest...); err != nil {
		return Memory{}, err
	}
	m.Keywords = splitKeywords(kw)
	m.Historical = m.Status == StatusSuperseded
	return m, nil
}

func collect(rows *sql.Rows) ([]Memory, error) {
	defer rows.Close()
	out := []Memory{}
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func clampLimit(n int) int {
	switch {
	case n <= 0:
		return defaultSearchLimit
	case n > maxSearchLimit:
		return maxSearchLimit
	}
	return n
}

// filter is the hard part of a search: which rows may match at all.
type filter struct {
	includeHistory bool
	scopes         []string
	origins        []string
	within         *scopeKey
}

func (a SearchArgs) filter() filter {
	return filter{includeHistory: a.IncludeHistory, scopes: a.Scopes, origins: a.Origins, within: a.within}
}

// where is an " AND ..." clause over alias m, with anonymous placeholders.
func (f filter) where() (string, []any) {
	var b strings.Builder
	var args []any
	if !f.includeHistory {
		b.WriteString(" AND m.status = 'current'")
	}
	in := func(col string, vals []string) {
		if len(vals) == 0 {
			return
		}
		b.WriteString(" AND " + col + " IN (?" + strings.Repeat(",?", len(vals)-1) + ")")
		for _, v := range vals {
			args = append(args, v)
		}
	}
	in("m.scope", f.scopes)
	in("m.origin", f.origins)
	if f.within != nil {
		scope, project := f.within.args()
		b.WriteString(" AND m.scope = ? AND m.project IS ?")
		args = append(args, scope, project)
	}
	return b.String(), args
}

// ftsRow is a keyword match with its bm25 relevance (higher is better).
type ftsRow struct {
	Memory
	bm25 float64
}

// searchFTSRows is the keyword half of search, recall-first (BuildMatch): bm25 orders results and
// nothing is cut by score. Column weights: fact 10, reason 3, keywords 6.
func (s *Store) searchFTSRows(ctx context.Context, match string, f filter, pool int) ([]ftsRow, error) {
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	where, args := f.where()
	rows, err := db.QueryContext(ctx, `SELECT `+memoryColumns+`, bm25(memories_fts, 10.0, 3.0, 6.0) AS rank
		FROM memories_fts JOIN memories m ON m.seq = memories_fts.rowid
		WHERE memories_fts MATCH ?`+where+`
		ORDER BY rank, m.seq DESC
		LIMIT ?`, append(append([]any{match}, args...), pool)...)
	if err != nil {
		return nil, fmt.Errorf("memory: search: %w", err)
	}
	defer rows.Close()
	out := []ftsRow{}
	for rows.Next() {
		var rank float64
		m, err := scanMemory(rows, &rank)
		if err != nil {
			return nil, err
		}
		out = append(out, ftsRow{Memory: m, bm25: -rank})
	}
	return out, rows.Err()
}

// searchFTS lists keyword matches only, best first; reconcile uses it.
func (s *Store) searchFTS(ctx context.Context, a SearchArgs) ([]Memory, error) {
	match, ok := BuildMatch(a.Query)
	if !ok {
		return []Memory{}, nil
	}
	rows, err := s.searchFTSRows(ctx, match, a.filter(), clampLimit(a.Limit))
	if err != nil {
		return nil, err
	}
	out := make([]Memory, len(rows))
	for i, r := range rows {
		out[i] = r.Memory
	}
	return out, nil
}

// Recent lists current memories, newest first.
func (s *Store) Recent(ctx context.Context, limit int) ([]Memory, error) {
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+memoryColumns+` FROM memories m WHERE m.status = 'current'
		ORDER BY m.created_at DESC, m.seq DESC LIMIT ?`, clampLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("memory: recent: %w", err)
	}
	return collect(rows)
}

// lineageOf resolves a fact id or a version id to its fact id.
func lineageOf(ctx context.Context, q queryer, id string) (string, error) {
	var lineage string
	err := q.QueryRowContext(ctx, `SELECT lineage_id FROM memories
		WHERE lineage_id = ?1 OR id = ?1
		LIMIT 1`, id).Scan(&lineage)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("memory: resolve id: %w", err)
	}
	return lineage, nil
}

// Lineage returns every version of a fact plus its events. id is the fact id or any version id.
func (s *Store) Lineage(ctx context.Context, id string) (History, error) {
	db, err := s.conn()
	if err != nil {
		return History{}, err
	}
	lineage, err := lineageOf(ctx, db, id)
	if err != nil {
		return History{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+memoryColumns+` FROM memories m
		WHERE m.lineage_id = ? ORDER BY m.version`, lineage)
	if err != nil {
		return History{}, fmt.Errorf("memory: lineage: %w", err)
	}
	mems, err := collect(rows)
	if err != nil {
		return History{}, err
	}
	if len(mems) == 0 {
		return History{}, ErrNotFound
	}
	erows, err := db.QueryContext(ctx, `SELECT e.seq, e.request_id, e.source, e.source_ref, COALESCE(f.rel_path, ''),
		e.action, e.lineage_id, e.memory_id, e.previous_id, e.origin, e.source_tag, e.session_id, e.rationale, e.created_at
		FROM memory_events e LEFT JOIN import_files f ON e.source = 'import' AND f.id = e.source_ref
		WHERE e.lineage_id = ? ORDER BY e.seq`, lineage)
	if err != nil {
		return History{}, fmt.Errorf("memory: events: %w", err)
	}
	defer erows.Close()
	h := History{Memories: mems, Events: []Event{}}
	for erows.Next() {
		var e Event
		if err := erows.Scan(&e.Seq, &e.RequestID, &e.Source, &e.SourceRef, &e.SourceLabel, &e.Action, &e.LineageID, &e.MemoryID,
			&e.PreviousID, &e.Origin, &e.SourceTag, &e.SessionID, &e.Rationale, &e.CreatedAt); err != nil {
			return History{}, err
		}
		h.Events = append(h.Events, e)
	}
	return h, erows.Err()
}

// Context returns the current version of a fact with, per group the fact belongs to (newest group
// first), the other current facts of that group, newest first, at most limit each.
func (s *Store) Context(ctx context.Context, id string, limit int) (Context, error) {
	db, err := s.conn()
	if err != nil {
		return Context{}, err
	}
	lineage, err := lineageOf(ctx, db, id)
	if err != nil {
		return Context{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+memoryColumns+` FROM memories m
		WHERE m.lineage_id = ? AND m.status = 'current'`, lineage)
	if err != nil {
		return Context{}, fmt.Errorf("memory: context: %w", err)
	}
	cur, err := collect(rows)
	if err != nil {
		return Context{}, err
	}
	if len(cur) == 0 {
		return Context{}, ErrNotFound
	}
	out := Context{Memory: cur[0], Groups: []GroupContext{}}
	grows, err := db.QueryContext(ctx, `SELECT g.id, g.kind, g.key, g.title,
		(SELECT count(*) FROM memory_group_members x WHERE x.group_id = g.id AND x.lineage_id <> ?1)
		FROM memory_group_members gm JOIN memory_groups g ON g.id = gm.group_id
		WHERE gm.lineage_id = ?1 ORDER BY gm.added_at DESC, g.rowid DESC`, lineage)
	if err != nil {
		return Context{}, fmt.Errorf("memory: context groups: %w", err)
	}
	for grows.Next() {
		var g GroupContext
		if err := grows.Scan(&g.ID, &g.Kind, &g.Key, &g.Title, &g.Total); err != nil {
			_ = grows.Close()
			return Context{}, err
		}
		out.Groups = append(out.Groups, g)
	}
	if err := grows.Err(); err != nil {
		_ = grows.Close()
		return Context{}, err
	}
	_ = grows.Close()
	for i := range out.Groups {
		frows, err := db.QueryContext(ctx, `SELECT `+memoryColumns+` FROM memory_group_members gm
			JOIN memories m ON m.lineage_id = gm.lineage_id AND m.status = 'current'
			WHERE gm.group_id = ? AND gm.lineage_id <> ?
			ORDER BY m.created_at DESC, m.seq DESC LIMIT ?`, out.Groups[i].ID, lineage, limit)
		if err != nil {
			return Context{}, fmt.Errorf("memory: context facts: %w", err)
		}
		if out.Groups[i].Facts, err = collect(frows); err != nil {
			return Context{}, err
		}
	}
	return out, nil
}

// Projects lists the distinct project keys of current facts, sorted.
func (s *Store) Projects(ctx context.Context) ([]string, error) {
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT project FROM memories
		WHERE status = 'current' AND project IS NOT NULL ORDER BY project`)
	if err != nil {
		return nil, fmt.Errorf("memory: projects: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ImportProvenance is what an import-mode store call forces onto every fact: the job's provenance and
// groups, plus the file's absolute and relative paths.
type ImportProvenance struct {
	Path, RelPath                              string
	Origin, SourceTag, Scope, Project, Session string
	Groups                                     []GroupRef
}

// ImportProvenance reads the import file ref and its job. ErrNotFound when the file row is gone.
func (s *Store) ImportProvenance(ctx context.Context, ref string) (ImportProvenance, error) {
	db, err := s.conn()
	if err != nil {
		return ImportProvenance{}, err
	}
	var p ImportProvenance
	var groups string
	err = db.QueryRowContext(ctx, `SELECT f.path, f.rel_path, j.origin, COALESCE(j.source_tag, ''), j.scope, COALESCE(j.project, ''),
		COALESCE(j.session_id, ''), j.groups_json FROM import_files f JOIN import_jobs j ON j.id = f.job_id WHERE f.id = ?`, ref).
		Scan(&p.Path, &p.RelPath, &p.Origin, &p.SourceTag, &p.Scope, &p.Project, &p.Session, &groups)
	if errors.Is(err, sql.ErrNoRows) {
		return ImportProvenance{}, ErrNotFound
	}
	if err != nil {
		return ImportProvenance{}, fmt.Errorf("memory: import provenance: %w", err)
	}
	if err := json.Unmarshal([]byte(groups), &p.Groups); err != nil {
		return ImportProvenance{}, fmt.Errorf("memory: import groups: %w", err)
	}
	return p, nil
}

// currentOf returns the current version of a fact id.
func (s *Store) currentOf(ctx context.Context, lineage string) (Memory, bool, error) {
	db, err := s.conn()
	if err != nil {
		return Memory{}, false, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+memoryColumns+` FROM memories m WHERE m.lineage_id = ? AND m.status = 'current'`, lineage)
	if err != nil {
		return Memory{}, false, fmt.Errorf("memory: current version: %w", err)
	}
	ms, err := collect(rows)
	if err != nil || len(ms) == 0 {
		return Memory{}, false, err
	}
	return ms[0], true, nil
}

// DocumentFacts lists the current versions of the facts the import file's events touched (add, update,
// noop), in the order of their first event.
func (s *Store) DocumentFacts(ctx context.Context, fileID string) ([]Memory, error) {
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+memoryColumns+` FROM memories m
		JOIN (SELECT lineage_id, min(seq) AS first FROM memory_events WHERE source = 'import' AND source_ref = ? GROUP BY lineage_id) e
			ON e.lineage_id = m.lineage_id
		WHERE m.status = 'current' ORDER BY e.first`, fileID)
	if err != nil {
		return nil, fmt.Errorf("memory: document facts: %w", err)
	}
	return collect(rows)
}

// orphans counts the facts of group id that listed does not name.
func (s *Store) orphans(ctx context.Context, groupID string, listed []memfmt.Fact) (int, error) {
	db, err := s.conn()
	if err != nil {
		return 0, err
	}
	rows, err := db.QueryContext(ctx, `SELECT m.lineage_id FROM memory_group_members gm
		JOIN memories m ON m.lineage_id = gm.lineage_id AND m.status = 'current' WHERE gm.group_id = ?`, groupID)
	if err != nil {
		return 0, fmt.Errorf("memory: orphans: %w", err)
	}
	defer rows.Close()
	named := make(map[string]struct{}, len(listed))
	for _, f := range listed {
		named[f.ID] = struct{}{}
	}
	n := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		if _, ok := named[id]; !ok {
			n++
		}
	}
	return n, rows.Err()
}

// ErrNotFound is returned when an id names no memory.
var ErrNotFound = errors.New("memory not found")

// currentByHash lists current memories whose normalized fact hashes to h, within one scope and project.
func (s *Store) currentByHash(ctx context.Context, h string, within *scopeKey) ([]Memory, error) {
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	scope, project := within.args()
	rows, err := db.QueryContext(ctx, `SELECT `+memoryColumns+` FROM memories m
		WHERE m.status = 'current' AND m.fact_hash = ?1 AND (?2 IS NULL OR (m.scope = ?2 AND m.project IS ?3))`, h, scope, project)
	if err != nil {
		return nil, fmt.Errorf("memory: hash lookup: %w", err)
	}
	return collect(rows)
}

// Revision is the insert counter reconcile decisions are checked against at commit.
func (s *Store) Revision(ctx context.Context) (int64, error) {
	db, err := s.conn()
	if err != nil {
		return 0, err
	}
	return readRevision(ctx, db)
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func readRevision(ctx context.Context, q queryer) (int64, error) {
	var v int64
	if err := q.QueryRowContext(ctx, `SELECT value FROM memory_revision WHERE id = 1`).Scan(&v); err != nil {
		return 0, fmt.Errorf("memory: revision: %w", err)
	}
	return v, nil
}

// DataVersion changes whenever another connection (the MCP subprocess) commits.
func (s *Store) DataVersion(ctx context.Context) (int64, error) {
	db, err := s.conn()
	if err != nil {
		return 0, err
	}
	var v int64
	if err := db.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&v); err != nil {
		return 0, fmt.Errorf("memory: data_version: %w", err)
	}
	return v, nil
}

type commitInput struct {
	Revision  int64
	Action    string
	TargetID  string // update/noop: version id of the current version
	FactID    string // add: the fact id to use (a doc's); empty generates one
	Fact      string
	Kind      string // empty means fact
	Reason    string
	Keywords  []string
	Origin    string
	SourceTag string // external only
	Scope     string
	Project   string // scope project only
	SessionID string // empty stores NULL
	Source    string
	SourceRef string // import file id; empty stores NULL
	RequestID string
	Why       string
	// Groups the fact joins, whatever the action. Group rows are created on first use.
	Groups []GroupRef
	// Vec, when set, is the L2-normalised embedding of the inserted fact, stored in the same
	// transaction under Model. Noop ignores it.
	Vec   []byte
	Model string
}

type commitResult struct {
	ID, LineageID string
	Version       int
	PreviousID    string
	// Revision is the store revision after this commit, so a request's next fact can proceed
	// without mistaking its own insert for a concurrent writer.
	Revision int64
}

// nullable maps "" to SQL NULL.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// commitFact applies one decision in one write transaction. errStale means the store changed
// since the decision was read; the caller reruns reconcile.
func (s *Store) commitFact(ctx context.Context, in commitInput) (commitResult, error) {
	db, err := s.conn()
	if err != nil {
		return commitResult{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return commitResult{}, fmt.Errorf("memory: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rev, err := readRevision(ctx, tx)
	if err != nil {
		return commitResult{}, err
	}
	if rev != in.Revision {
		return commitResult{}, errStale
	}
	now := kiratime.NowISO()
	var res commitResult
	switch in.Action {
	case ActionAdd:
		res, err = commitAdd(ctx, tx, in, now)
	case ActionUpdate:
		res, err = commitUpdate(ctx, tx, in, now)
	case ActionNoop:
		res, err = commitNoop(ctx, tx, in)
	default:
		err = fmt.Errorf("memory: unknown action %q", in.Action)
	}
	if err != nil {
		return commitResult{}, err
	}

	var prev, ref any
	if res.PreviousID != "" {
		prev = res.PreviousID
	}
	if in.SourceRef != "" {
		ref = in.SourceRef
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO memory_events
		(request_id, source, source_ref, action, lineage_id, memory_id, previous_id, origin, source_tag, session_id, rationale, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.RequestID, in.Source, ref, in.Action, res.LineageID, res.ID, prev, in.Origin, nullable(in.SourceTag),
		nullable(in.SessionID), in.Why, now); err != nil {
		return commitResult{}, fmt.Errorf("memory: event: %w", err)
	}
	for _, g := range in.Groups {
		if err := joinGroup(ctx, tx, g, res.LineageID, time.Now().UnixMilli()); err != nil {
			return commitResult{}, err
		}
	}
	if res.Revision, err = readRevision(ctx, tx); err != nil {
		return commitResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return commitResult{}, fmt.Errorf("memory: commit: %w", err)
	}
	return res, nil
}

// commitAdd inserts version 1 of a new fact. One retry on a fact id collision: 60 bits make a
// second one implausible.
func commitAdd(ctx context.Context, tx *sql.Tx, in commitInput, now string) (commitResult, error) {
	for attempt := 0; ; attempt++ {
		id := in.FactID
		if id == "" {
			id = ids.New(ids.Fact)
		}
		res := commitResult{ID: versionID(id, 1), LineageID: id, Version: 1}
		err := insertMemory(ctx, tx, res, in, "", now)
		if err == nil {
			return res, nil
		}
		if !isUnique(err) || attempt == 1 || in.FactID != "" {
			return commitResult{}, err
		}
	}
}

// commitUpdate supersedes the current version of in.TargetID's fact with a new one.
func commitUpdate(ctx context.Context, tx *sql.Tx, in commitInput, now string) (commitResult, error) {
	var lineage, status string
	var version int
	err := tx.QueryRowContext(ctx, `SELECT lineage_id, version, status FROM memories WHERE id = ?`, in.TargetID).
		Scan(&lineage, &version, &status)
	if err != nil || status != StatusCurrent {
		return commitResult{}, errStale
	}
	res := commitResult{ID: versionID(lineage, version+1), LineageID: lineage, Version: version + 1, PreviousID: in.TargetID}
	if _, err := tx.ExecContext(ctx, `UPDATE memories SET status = 'superseded', superseded_at = ? WHERE id = ?`,
		now, in.TargetID); err != nil {
		return commitResult{}, fmt.Errorf("memory: supersede: %w", err)
	}
	if err := insertMemory(ctx, tx, res, in, in.TargetID, now); err != nil {
		return commitResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE memories SET superseded_by_id = ? WHERE id = ?`,
		res.ID, in.TargetID); err != nil {
		return commitResult{}, fmt.Errorf("memory: link superseded: %w", err)
	}
	return res, nil
}

// commitNoop reports the existing version; the caller still records the event and group joins.
func commitNoop(ctx context.Context, tx *sql.Tx, in commitInput) (commitResult, error) {
	var lineage string
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT lineage_id, version FROM memories WHERE id = ?`, in.TargetID).
		Scan(&lineage, &version); err != nil {
		return commitResult{}, errStale
	}
	return commitResult{ID: in.TargetID, LineageID: lineage, Version: version}, nil
}

// joinGroup adds a fact to a group, creating the group on first use. Membership is by fact id, so it
// survives new versions. An existing group keeps its first title.
func joinGroup(ctx context.Context, tx *sql.Tx, g GroupRef, lineage string, ms int64) error {
	var id string
	var err error
	if g.ID != "" {
		err = tx.QueryRowContext(ctx, `SELECT id FROM memory_groups WHERE id = ?`, g.ID).Scan(&id)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT id FROM memory_groups WHERE kind = ? AND key = ?`, g.Kind, g.Key).Scan(&id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		if id = g.ID; id == "" {
			id = ids.New(ids.Group)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO memory_groups (id, kind, key, title, created_at) VALUES (?, ?, ?, ?, ?)`,
			id, g.Kind, g.Key, g.Title, ms)
	}
	if err != nil {
		return fmt.Errorf("memory: group: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO memory_group_members (group_id, lineage_id, added_at) VALUES (?, ?, ?)`,
		id, lineage, ms); err != nil {
		return fmt.Errorf("memory: group member: %w", err)
	}
	return nil
}

func insertMemory(ctx context.Context, tx *sql.Tx, r commitResult, in commitInput, supersedes, now string) error {
	res, err := tx.ExecContext(ctx, `INSERT INTO memories
		(id, lineage_id, version, fact, kind, reason, keywords, origin, source_tag, scope, project, session_id, status,
		 supersedes_id, fact_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'current', ?, ?, ?)`,
		r.ID, r.LineageID, r.Version, in.Fact, cmp.Or(in.Kind, KindFact), in.Reason, joinKeywords(in.Keywords), in.Origin, nullable(in.SourceTag),
		in.Scope, nullable(in.Project), nullable(in.SessionID), nullable(supersedes), factHash(in.Fact), now)
	if err != nil {
		return fmt.Errorf("memory: insert: %w", err)
	}
	if len(in.Vec) == 0 {
		return nil
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("memory: insert: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO memory_embeddings (seq, model, vec, created_at)
		VALUES (?, ?, ?, ?)`, seq, in.Model, in.Vec, now); err != nil {
		return fmt.Errorf("memory: insert embedding: %w", err)
	}
	return nil
}
