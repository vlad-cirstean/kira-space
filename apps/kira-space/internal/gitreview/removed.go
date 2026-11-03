package gitreview

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// RemovedSource identifies the review record (one review event) a removed block came from.
type RemovedSource struct {
	Origin        string // OriginCommit | OriginWorking
	Path          string
	ReviewedAt    time.Time
	ReviewedAtSHA string
	SnapshotKey   string // commit: blob oid; working: content hash
	ContentKind   ContentKind
}

// Removed is one stored block of reviewed lines that left the current content. Range is nil for a
// non-text whole-file row.
type Removed struct {
	ID            int64
	Origin        string
	Path          string
	ReviewedAtSHA string
	ReviewedAt    time.Time
	RemovedAt     time.Time
	ContentKind   ContentKind
	Range         *LineRange
	Text          []byte
}

// SnapshotLines returns lines r.Start..r.End (1-based) of snapshot, each with its stored trailing
// newline. A final line without a newline counts as a line.
func SnapshotLines(snapshot []byte, r LineRange) []byte {
	var out []byte
	line := 1
	rest := snapshot
	for len(rest) > 0 && line <= r.End {
		n := bytes.IndexByte(rest, '\n')
		var cur []byte
		if n < 0 {
			cur, rest = rest, nil
		} else {
			cur, rest = rest[:n+1], rest[n+1:]
		}
		if line >= r.Start {
			out = append(out, cur...)
		}
		line++
	}
	return out
}

func sourceWhere() string {
	return `session_id = ? AND origin = ? AND path = ? AND reviewed_at = ? AND snapshot_key = ?`
}

// CaptureRemoved stores the blocks of ranges (snapshot coordinates) not already stored for src,
// dismissed ones included, so a dismissed block never returns. Idempotent. It returns the number
// of rows inserted; no session means nothing to store.
func (s *Store) CaptureRemoved(ctx context.Context, repoID, branch string, src RemovedSource, ranges []LineRange, snapshot []byte) (int, error) {
	return s.captureRemoved(ctx, repoID, branch, src, func(existing []LineRange) ([]removedRow, error) {
		fresh := Subtract(Normalize(ranges), existing)
		rows := make([]removedRow, 0, len(fresh))
		for _, r := range fresh {
			text := SnapshotLines(snapshot, r)
			compressed, err := Compress(text)
			if err != nil {
				return nil, err
			}
			rows = append(rows, removedRow{r: r, bytes: len(text), content: compressed})
		}
		return rows, nil
	})
}

// CaptureRemovedFile stores one whole-file row without text, unless a row for src exists.
func (s *Store) CaptureRemovedFile(ctx context.Context, repoID, branch string, src RemovedSource) (int, error) {
	return s.captureRemoved(ctx, repoID, branch, src, func(existing []LineRange) ([]removedRow, error) {
		if len(existing) > 0 {
			return nil, nil
		}
		return []removedRow{{}}, nil
	})
}

type removedRow struct {
	r       LineRange
	bytes   int
	content []byte
}

func (s *Store) captureRemoved(ctx context.Context, repoID, branch string, src RemovedSource, plan func(existing []LineRange) ([]removedRow, error)) (int, error) {
	db, err := s.conn()
	if err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("gitreview: begin CaptureRemoved: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var id int64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM review_session WHERE repo_id = ? AND branch = ?`, repoID, branch).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("gitreview: read review_session: %w", err)
	}

	args := []any{id, src.Origin, src.Path, src.ReviewedAt.UnixMilli(), src.SnapshotKey}
	rows, err := tx.QueryContext(ctx, `SELECT start_line, end_line FROM review_removed WHERE `+sourceWhere(), args...)
	if err != nil {
		return 0, fmt.Errorf("gitreview: query review_removed: %w", err)
	}
	var existing []LineRange
	for rows.Next() {
		var r LineRange
		if err := rows.Scan(&r.Start, &r.End); err != nil {
			rows.Close()
			return 0, fmt.Errorf("gitreview: scan review_removed: %w", err)
		}
		existing = append(existing, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("gitreview: iterate review_removed: %w", err)
	}
	rows.Close()

	fresh, err := plan(existing)
	if err != nil {
		return 0, err
	}
	now := time.Now().UnixMilli()
	for _, row := range fresh {
		var content any
		if row.content != nil {
			content = row.content
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO review_removed
				(session_id, origin, path, reviewed_at, reviewed_at_sha, snapshot_key, content_kind,
				 start_line, end_line, content_bytes, content, removed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, src.Origin, src.Path, src.ReviewedAt.UnixMilli(), src.ReviewedAtSHA, src.SnapshotKey,
			string(src.ContentKind), row.r.Start, row.r.End, row.bytes, content, now); err != nil {
			return 0, fmt.Errorf("gitreview: insert review_removed: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(fresh), nil
}

// Removed lists the undismissed removed blocks of (repoID, branch), ordered by path, origin and
// start line. Empty, never nil, when there is no session.
func (s *Store) Removed(ctx context.Context, repoID, branch string) ([]Removed, error) {
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	id, ok, err := sessionID(ctx, db, repoID, branch)
	if err != nil {
		return nil, err
	}
	out := []Removed{}
	if !ok {
		return out, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, origin, path, reviewed_at, reviewed_at_sha, content_kind, start_line, end_line,
		       content_bytes, content, removed_at
		FROM review_removed WHERE session_id = ? AND dismissed_at IS NULL
		ORDER BY path, origin, start_line, id`, id)
	if err != nil {
		return nil, fmt.Errorf("gitreview: query review_removed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r Removed
		var reviewedAt, removedAt int64
		var kind string
		var start, end, n int
		var compressed []byte
		if err := rows.Scan(&r.ID, &r.Origin, &r.Path, &reviewedAt, &r.ReviewedAtSHA, &kind, &start, &end,
			&n, &compressed, &removedAt); err != nil {
			return nil, fmt.Errorf("gitreview: scan review_removed: %w", err)
		}
		r.ReviewedAt = time.UnixMilli(reviewedAt)
		r.RemovedAt = time.UnixMilli(removedAt)
		r.ContentKind = ContentKind(kind)
		if r.ContentKind == ContentText || start > 0 {
			r.Range = &LineRange{Start: start, End: end}
		}
		if compressed != nil {
			if r.Text, err = Decompress(compressed, n); err != nil {
				return nil, err
			}
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gitreview: iterate review_removed: %w", err)
	}
	return out, nil
}

// DismissRemoved hides ids of this session for good and drops their text. Ids of other sessions
// match nothing. Returns the rows dismissed.
func (s *Store) DismissRemoved(ctx context.Context, repoID, branch string, ids []int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	db, err := s.conn()
	if err != nil {
		return 0, err
	}
	id, ok, err := sessionID(ctx, db, repoID, branch)
	if err != nil || !ok {
		return 0, err
	}
	args := []any{time.Now().UnixMilli(), id}
	for _, v := range ids {
		args = append(args, v)
	}
	res, err := db.ExecContext(ctx,
		`UPDATE review_removed SET dismissed_at = ?, content = NULL, content_bytes = 0
		 WHERE session_id = ? AND dismissed_at IS NULL AND id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`)`,
		args...)
	if err != nil {
		return 0, fmt.Errorf("gitreview: dismiss review_removed: %w", err)
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// SetRemovalChecked records the current-side identity last checked for removed lines. The
// reviewed_at guard makes it a no-op after a concurrent re-mark.
func (s *Store) SetRemovalChecked(ctx context.Context, repoID, branch, origin, path string, reviewedAt time.Time, key string) error {
	db, err := s.conn()
	if err != nil {
		return err
	}
	id, ok, err := sessionID(ctx, db, repoID, branch)
	if err != nil || !ok {
		return err
	}
	table := "review_file"
	if origin == OriginWorking {
		table = "review_working_file"
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE `+table+` SET removal_checked = ? WHERE session_id = ? AND path = ? AND reviewed_at = ?`,
		key, id, path, reviewedAt.UnixMilli()); err != nil {
		return fmt.Errorf("gitreview: set removal_checked: %w", err)
	}
	return nil
}
