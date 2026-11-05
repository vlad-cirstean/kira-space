package gitreview

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Comment origins (P418). A working comment was written against uncommitted worktree content.
const (
	OriginCommit  = "commit"
	OriginWorking = "working"
)

// ContentHash is the sha256 hex of raw — the identity of uncommitted content, which is never a git
// object.
func ContentHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// WorkingRecord is one (session, path)'s uncommitted-review state: a review_working_file row plus
// its review_working_range rows. Ranges are in the coordinates of the stored content snapshot.
type WorkingRecord struct {
	Path         string
	State        string // "full" | "partial"
	HeadSHA      string
	ReviewedAt   time.Time
	ContentHash  string
	ContentKind  ContentKind
	ContentBytes int // UNCOMPRESSED length
	LineCount    int
	Ranges       []LineRange
	// RemovalChecked is the worktree content identity last checked for removed reviewed lines;
	// WorkingRecords only.
	RemovalChecked string
}

// WorkingRecords returns every working record for (repoID, branch) keyed by path, without content.
func (s *Store) WorkingRecords(ctx context.Context, repoID, branch string) (map[string]WorkingRecord, error) {
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	id, ok, err := sessionID(ctx, db, repoID, branch)
	if err != nil {
		return nil, err
	}
	if !ok {
		return map[string]WorkingRecord{}, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT path, state, head_sha, reviewed_at, content_hash, content_kind, content_bytes, line_count, removal_checked
		FROM review_working_file WHERE session_id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("gitreview: query review_working_file: %w", err)
	}
	out := make(map[string]WorkingRecord)
	for rows.Next() {
		var rec WorkingRecord
		var at int64
		var kind string
		if err := rows.Scan(&rec.Path, &rec.State, &rec.HeadSHA, &at, &rec.ContentHash, &kind,
			&rec.ContentBytes, &rec.LineCount, &rec.RemovalChecked); err != nil {
			rows.Close()
			return nil, fmt.Errorf("gitreview: scan review_working_file: %w", err)
		}
		rec.ReviewedAt = time.UnixMilli(at)
		rec.ContentKind = ContentKind(kind)
		out[rec.Path] = rec
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("gitreview: iterate review_working_file: %w", err)
	}
	rows.Close()

	rr, err := db.QueryContext(ctx,
		`SELECT path, start_line, end_line FROM review_working_range WHERE session_id = ? ORDER BY path, start_line`, id)
	if err != nil {
		return nil, fmt.Errorf("gitreview: query review_working_range: %w", err)
	}
	defer rr.Close()
	for rr.Next() {
		var path string
		var r LineRange
		if err := rr.Scan(&path, &r.Start, &r.End); err != nil {
			return nil, fmt.Errorf("gitreview: scan review_working_range: %w", err)
		}
		if rec, ok := out[path]; ok {
			rec.Ranges = append(rec.Ranges, r)
			out[path] = rec
		}
	}
	if err := rr.Err(); err != nil {
		return nil, fmt.Errorf("gitreview: iterate review_working_range: %w", err)
	}
	return out, nil
}

// WorkingRecord returns one path's working record and, for a text record, its decompressed
// snapshot. found is false when there is no session or no record.
func (s *Store) WorkingRecord(ctx context.Context, repoID, branch, path string) (rec WorkingRecord, content []byte, found bool, err error) {
	db, err := s.conn()
	if err != nil {
		return WorkingRecord{}, nil, false, err
	}
	id, ok, err := sessionID(ctx, db, repoID, branch)
	if err != nil || !ok {
		return WorkingRecord{}, nil, false, err
	}
	var at int64
	var kind string
	var compressed []byte
	scanErr := db.QueryRowContext(ctx, `
		SELECT path, state, head_sha, reviewed_at, content_hash, content_kind, content_bytes, line_count, content
		FROM review_working_file WHERE session_id = ? AND path = ?`, id, path,
	).Scan(&rec.Path, &rec.State, &rec.HeadSHA, &at, &rec.ContentHash, &kind, &rec.ContentBytes, &rec.LineCount, &compressed)
	if errors.Is(scanErr, sql.ErrNoRows) {
		return WorkingRecord{}, nil, false, nil
	}
	if scanErr != nil {
		return WorkingRecord{}, nil, false, fmt.Errorf("gitreview: query review_working_file: %w", scanErr)
	}
	rec.ReviewedAt = time.UnixMilli(at)
	rec.ContentKind = ContentKind(kind)
	rec.Ranges, err = workingPathRanges(ctx, db, id, path)
	if err != nil {
		return WorkingRecord{}, nil, false, err
	}
	if rec.ContentKind == ContentText && compressed != nil {
		content, err = Decompress(compressed, rec.ContentBytes)
		if err != nil {
			return WorkingRecord{}, nil, false, err
		}
	}
	return rec, content, true, nil
}

func workingPathRanges(ctx context.Context, db *sql.DB, id int64, path string) ([]LineRange, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT start_line, end_line FROM review_working_range WHERE session_id = ? AND path = ? ORDER BY start_line`, id, path)
	if err != nil {
		return nil, fmt.Errorf("gitreview: query review_working_range: %w", err)
	}
	defer rows.Close()
	var out []LineRange
	for rows.Next() {
		var r LineRange
		if err := rows.Scan(&r.Start, &r.End); err != nil {
			return nil, fmt.Errorf("gitreview: scan review_working_range: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PutWorking replaces one path's working record and ranges in one transaction, creating the
// session row if needed. content is stored (compressed) only for ContentText.
func (s *Store) PutWorking(ctx context.Context, repoID, branch string, rec WorkingRecord, content []byte) error {
	db, err := s.conn()
	if err != nil {
		return err
	}
	var compressed []byte
	if rec.ContentKind == ContentText {
		if compressed, err = Compress(content); err != nil {
			return err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("gitreview: begin PutWorking: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, err := upsertSession(ctx, tx, repoID, branch, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO review_working_file
			(session_id, path, state, head_sha, reviewed_at, content_hash, content_kind, content_bytes, line_count, content)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (session_id, path) DO UPDATE SET
			state = excluded.state, head_sha = excluded.head_sha, reviewed_at = excluded.reviewed_at,
			content_hash = excluded.content_hash, content_kind = excluded.content_kind,
			content_bytes = excluded.content_bytes, line_count = excluded.line_count, content = excluded.content,
			removal_checked = ''
	`, id, rec.Path, rec.State, rec.HeadSHA, rec.ReviewedAt.UnixMilli(), rec.ContentHash,
		string(rec.ContentKind), rec.ContentBytes, rec.LineCount, compressed); err != nil {
		return fmt.Errorf("gitreview: upsert review_working_file: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM review_working_range WHERE session_id = ? AND path = ?`, id, rec.Path); err != nil {
		return fmt.Errorf("gitreview: clear review_working_range: %w", err)
	}
	for _, r := range Normalize(rec.Ranges) {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO review_working_range (session_id, path, start_line, end_line) VALUES (?, ?, ?, ?)`,
			id, rec.Path, r.Start, r.End); err != nil {
			return fmt.Errorf("gitreview: insert review_working_range: %w", err)
		}
	}
	return tx.Commit()
}

// DeleteWorking removes one path's working record (ranges cascade). No-op when absent.
func (s *Store) DeleteWorking(ctx context.Context, repoID, branch, path string) error {
	db, err := s.conn()
	if err != nil {
		return err
	}
	id, ok, err := sessionID(ctx, db, repoID, branch)
	if err != nil || !ok {
		return err
	}
	if _, err := db.ExecContext(ctx,
		`DELETE FROM review_working_file WHERE session_id = ? AND path = ?`, id, path); err != nil {
		return fmt.Errorf("gitreview: delete review_working_file: %w", err)
	}
	return nil
}

// PromoteComment turns a working comment into a commit-origin one anchored at a commit: new
// range, anchor, and no stored content. false when the id matches nothing in this session.
func (s *Store) PromoteComment(ctx context.Context, repoID, branch string, id string, anchorSHA, blobOID string, rng LineRange) (bool, error) {
	return s.updateComment(ctx, repoID, branch,
		`origin = 'commit', anchor_sha = ?, anchor_blob_oid = ?, start_line = ?, end_line = ?, anchor_content = NULL`,
		[]any{anchorSHA, blobOID, rng.Start, rng.End}, id)
}
