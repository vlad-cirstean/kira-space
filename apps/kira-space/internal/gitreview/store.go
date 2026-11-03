package gitreview

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Store is review.db's whole surface. Construction is free: the file is neither created
// nor opened until the first call that needs it (ensureOpen, db.go) — a Kira Space instance that
// never serves a review request (including a second instance that lost the app.lock flock)
// never creates the file and never starts the reaper.
type Store struct {
	path string

	openMu sync.Mutex
	sqlDB  *sql.DB

	reapStop chan struct{}
	reapDone chan struct{}
	closed   bool

	closeOnce sync.Once
	closeErr  error

	locks *keyedMutex
}

// SetPinned pins or unpins (repoID, branch): a pinned session survives the idle sweep and Purge.
// Pinning creates the session row so the pin outlives the first mark.
func (s *Store) SetPinned(ctx context.Context, repoID, branch string, pinned bool) error {
	db, err := s.conn()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("gitreview: begin SetPinned: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := upsertSession(ctx, tx, repoID, branch, time.Now().UnixMilli()); err != nil {
		return err
	}
	v := 0
	if pinned {
		v = 1
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE review_session SET pinned = ? WHERE repo_id = ? AND branch = ?`, v, repoID, branch,
	); err != nil {
		return fmt.Errorf("gitreview: set pinned: %w", err)
	}
	return tx.Commit()
}

// NewStore constructs a Store over path. Nothing is opened yet.
func NewStore(path string) *Store {
	return &Store{path: path, locks: newKeyedMutex()}
}

// Close stops the reaper, joins its goroutine, and closes the underlying *sql.DB. Idempotent and
// safe to call concurrently (later callers wait for the first and share its result), and safe when
// the store was never opened. Once closed, the store never reopens: conn returns ErrStoreClosed.
func (s *Store) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.close() })
	return s.closeErr
}

func (s *Store) close() error {
	s.openMu.Lock()
	s.closed = true
	stop, done := s.reapStop, s.reapDone
	s.reapStop, s.reapDone = nil, nil
	s.openMu.Unlock()
	if stop != nil {
		close(stop)
		<-done
	}
	s.openMu.Lock()
	db := s.sqlDB
	s.sqlDB = nil
	s.openMu.Unlock()
	if db == nil {
		return nil
	}
	return db.Close()
}

// Lock takes the per-(repoID, branch, path) mutex D12 requires review.mark to hold for the whole
// of its read-diff-write — the returned func releases it. Concurrent marks on different files
// never contend; the map entry is refcounted so it does not grow without bound.
func (s *Store) Lock(repoID, branch, path string) func() {
	return s.locks.lock(repoID + "\x00" + branch + "\x00" + path)
}

// FileRecord is one (session, path)'s whole stored state — gitsession's own view of a
// review_file row plus its review_range rows.
type FileRecord struct {
	Path          string
	State         string // "full" | "partial"
	ReviewedAtSHA string
	ReviewedAt    time.Time
	BlobOID       string
	ContentKind   ContentKind
	ContentBytes  int         // UNCOMPRESSED length
	LineCount     int         // text only; 0 otherwise
	Ranges        []LineRange // snapshot coordinates (D10's invariant); empty when State == "full"
	// RemovalChecked is the current-side identity (tip blob oid) last checked for removed reviewed
	// lines; Records only.
	RemovalChecked string
}

// sessionID resolves (repoID, branch) to its review_session row id — false, not an error, when no
// session has ever been created for it (the common case: a session row is created only by the
// first review.mark, D11).
func sessionID(ctx context.Context, db *sql.DB, repoID, branch string) (int64, bool, error) {
	var id int64
	err := db.QueryRowContext(ctx,
		`SELECT id FROM review_session WHERE repo_id = ? AND branch = ?`, repoID, branch,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("gitreview: read review_session: %w", err)
	}
	return id, true, nil
}

// rangesForSession returns every review_range row for id, grouped by path — one query for every
// file in the session rather than one round trip per file.
func rangesForSession(ctx context.Context, db *sql.DB, id int64) (map[string][]LineRange, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT path, start_line, end_line FROM review_range WHERE session_id = ? ORDER BY path, start_line`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("gitreview: query review_range: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]LineRange)
	for rows.Next() {
		var path string
		var r LineRange
		if err := rows.Scan(&path, &r.Start, &r.End); err != nil {
			return nil, fmt.Errorf("gitreview: scan review_range: %w", err)
		}
		out[path] = append(out[path], r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gitreview: iterate review_range: %w", err)
	}
	return out, nil
}

func pathRanges(ctx context.Context, db *sql.DB, id int64, path string) ([]LineRange, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT start_line, end_line FROM review_range WHERE session_id = ? AND path = ? ORDER BY start_line`,
		id, path,
	)
	if err != nil {
		return nil, fmt.Errorf("gitreview: query review_range: %w", err)
	}
	defer rows.Close()

	var out []LineRange
	for rows.Next() {
		var r LineRange
		if err := rows.Scan(&r.Start, &r.End); err != nil {
			return nil, fmt.Errorf("gitreview: scan review_range: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gitreview: iterate review_range: %w", err)
	}
	return out, nil
}

// Records returns every record for (repoID, branch), keyed by path, WITHOUT reading the content
// column — the list path (review.files) never needs a snapshot's bytes, and a query that selected
// the BLOB would read megabytes to answer a question about oids. An empty map (never nil) when the
// session has no records, or does not exist at all.
func (s *Store) Records(ctx context.Context, repoID, branch string) (map[string]FileRecord, error) {
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	id, ok, err := sessionID(ctx, db, repoID, branch)
	if err != nil {
		return nil, err
	}
	if !ok {
		return map[string]FileRecord{}, nil
	}

	rows, err := db.QueryContext(ctx, `
		SELECT path, state, reviewed_at_sha, reviewed_at, blob_oid, content_kind, content_bytes, line_count, removal_checked
		FROM review_file WHERE session_id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("gitreview: query review_file: %w", err)
	}
	out := make(map[string]FileRecord)
	for rows.Next() {
		var rec FileRecord
		var reviewedAtMillis int64
		var kind string
		if err := rows.Scan(&rec.Path, &rec.State, &rec.ReviewedAtSHA, &reviewedAtMillis,
			&rec.BlobOID, &kind, &rec.ContentBytes, &rec.LineCount, &rec.RemovalChecked); err != nil {
			rows.Close()
			return nil, fmt.Errorf("gitreview: scan review_file: %w", err)
		}
		rec.ReviewedAt = time.UnixMilli(reviewedAtMillis)
		rec.ContentKind = ContentKind(kind)
		out[rec.Path] = rec
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("gitreview: iterate review_file: %w", err)
	}
	rows.Close()

	ranges, err := rangesForSession(ctx, db, id)
	if err != nil {
		return nil, err
	}
	for path, rec := range out {
		rec.Ranges = ranges[path]
		out[path] = rec
	}
	return out, nil
}

// Record returns one path's record, and its decompressed content when ContentKind is "text".
// found is false when there is no session, or the session has no record for path.
func (s *Store) Record(ctx context.Context, repoID, branch, path string) (rec FileRecord, content []byte, found bool, err error) {
	db, err := s.conn()
	if err != nil {
		return FileRecord{}, nil, false, err
	}
	id, ok, err := sessionID(ctx, db, repoID, branch)
	if err != nil {
		return FileRecord{}, nil, false, err
	}
	if !ok {
		return FileRecord{}, nil, false, nil
	}

	var reviewedAtMillis int64
	var kind string
	var compressed []byte
	row := db.QueryRowContext(ctx, `
		SELECT path, state, reviewed_at_sha, reviewed_at, blob_oid, content_kind, content_bytes, line_count, content
		FROM review_file WHERE session_id = ? AND path = ?`, id, path)
	if scanErr := row.Scan(&rec.Path, &rec.State, &rec.ReviewedAtSHA, &reviewedAtMillis,
		&rec.BlobOID, &kind, &rec.ContentBytes, &rec.LineCount, &compressed); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return FileRecord{}, nil, false, nil
		}
		return FileRecord{}, nil, false, fmt.Errorf("gitreview: query review_file: %w", scanErr)
	}
	rec.ReviewedAt = time.UnixMilli(reviewedAtMillis)
	rec.ContentKind = ContentKind(kind)

	rec.Ranges, err = pathRanges(ctx, db, id, path)
	if err != nil {
		return FileRecord{}, nil, false, err
	}

	if rec.ContentKind == ContentText && compressed != nil {
		content, err = Decompress(compressed, rec.ContentBytes)
		if err != nil {
			return FileRecord{}, nil, false, err
		}
	}
	return rec, content, true, nil
}

// upsertSession creates (repoID, branch)'s review_session row if this is its first record, or
// bumps its last_used_at either way.
func upsertSession(ctx context.Context, tx *sql.Tx, repoID, branch string, nowMillis int64) (int64, error) {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO review_session (repo_id, branch, created_at, last_used_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (repo_id, branch) DO UPDATE SET last_used_at = excluded.last_used_at
	`, repoID, branch, nowMillis, nowMillis); err != nil {
		return 0, fmt.Errorf("gitreview: upsert review_session: %w", err)
	}
	// SQLite's LastInsertId is only reliable for a real INSERT, not the ON CONFLICT UPDATE arm, so
	// the id is read back explicitly regardless of which arm ran.
	var id int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM review_session WHERE repo_id = ? AND branch = ?`, repoID, branch,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("gitreview: read review_session id: %w", err)
	}
	return id, nil
}

// Put replaces one path's record and its ranges in one transaction, creating the session row if
// this is its first record, and touching last_used_at either way. Ranges are normalized before
// storing (the invariant is "in the snapshot's coordinates", never "however the caller happened to
// list them").
func (s *Store) Put(ctx context.Context, repoID, branch string, rec FileRecord, content []byte) error {
	db, err := s.conn()
	if err != nil {
		return err
	}

	var compressed []byte
	if rec.ContentKind == ContentText {
		compressed, err = Compress(content)
		if err != nil {
			return err
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("gitreview: begin Put: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit has succeeded

	id, err := upsertSession(ctx, tx, repoID, branch, time.Now().UnixMilli())
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO review_file
			(session_id, path, state, reviewed_at_sha, reviewed_at, blob_oid, content_kind, content_bytes, line_count, content)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (session_id, path) DO UPDATE SET
			state = excluded.state, reviewed_at_sha = excluded.reviewed_at_sha, reviewed_at = excluded.reviewed_at,
			blob_oid = excluded.blob_oid, content_kind = excluded.content_kind,
			content_bytes = excluded.content_bytes, line_count = excluded.line_count, content = excluded.content,
			removal_checked = ''
	`, id, rec.Path, rec.State, rec.ReviewedAtSHA, rec.ReviewedAt.UnixMilli(), rec.BlobOID,
		string(rec.ContentKind), rec.ContentBytes, rec.LineCount, compressed); err != nil {
		return fmt.Errorf("gitreview: upsert review_file: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM review_range WHERE session_id = ? AND path = ?`, id, rec.Path,
	); err != nil {
		return fmt.Errorf("gitreview: clear review_range: %w", err)
	}
	for _, r := range Normalize(rec.Ranges) {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO review_range (session_id, path, start_line, end_line) VALUES (?, ?, ?, ?)`,
			id, rec.Path, r.Start, r.End,
		); err != nil {
			return fmt.Errorf("gitreview: insert review_range: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// Delete removes one path's record (and its ranges, by cascade). The session row is left alone —
// an empty session is the reaper's business, not this call's.
func (s *Store) Delete(ctx context.Context, repoID, branch, path string) error {
	db, err := s.conn()
	if err != nil {
		return err
	}
	id, ok, err := sessionID(ctx, db, repoID, branch)
	if err != nil || !ok {
		return err
	}
	if _, err := db.ExecContext(ctx,
		`DELETE FROM review_file WHERE session_id = ? AND path = ?`, id, path,
	); err != nil {
		return fmt.Errorf("gitreview: delete review_file: %w", err)
	}
	return nil
}

// Touch bumps last_used_at for an EXISTING session only (D11) — never creates one. A no-op when
// no session exists for (repoID, branch).
func (s *Store) Touch(ctx context.Context, repoID, branch string) error {
	db, err := s.conn()
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE review_session SET last_used_at = ? WHERE repo_id = ? AND branch = ?`,
		time.Now().UnixMilli(), repoID, branch,
	); err != nil {
		return fmt.Errorf("gitreview: touch review_session: %w", err)
	}
	return nil
}

// keyedMutex is D12's own concurrency answer for review.mark: a mutex per key, refcounted so the
// map does not grow without bound once every in-flight mark on a key has released it.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*refCountedMutex
}

type refCountedMutex struct {
	mu   sync.Mutex
	refs int
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{locks: make(map[string]*refCountedMutex)}
}

// lock blocks until key's mutex is held and returns a func that releases it. Concurrent lock
// calls for different keys never contend with each other.
func (k *keyedMutex) lock(key string) func() {
	k.mu.Lock()
	entry, ok := k.locks[key]
	if !ok {
		entry = &refCountedMutex{}
		k.locks[key] = entry
	}
	entry.refs++
	k.mu.Unlock()

	entry.mu.Lock()

	var once sync.Once
	return func() {
		once.Do(func() {
			entry.mu.Unlock()
			k.mu.Lock()
			entry.refs--
			if entry.refs == 0 {
				delete(k.locks, key)
			}
			k.mu.Unlock()
		})
	}
}
