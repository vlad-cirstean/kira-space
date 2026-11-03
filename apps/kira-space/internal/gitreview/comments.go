package gitreview

import (
	"bytes"
	"compress/flate"
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"
)

// Comment is one review_comment row. Ranges are in AnchorSHA's own coordinates (D6's invariant)
// and are never rewritten — projection happens on read, in gitsession.
type Comment struct {
	ID            int64
	Path          string
	Range         LineRange
	Body          string
	AnchorSHA     string
	AnchorBlobOID string
	CreatedAt     time.Time
	Kind          string // CommentKindChange | CommentKindQuestion
	State         string // CommentStateOpen | CommentStateAddressed
	Reply         string
	RepliedAt     *time.Time
	SentAt        *time.Time
	Origin        string // OriginCommit | OriginWorking; "" reads as commit
	// AnchorContent is the worktree content a working comment was written against (raw, text only).
	AnchorContent []byte
}

// Comment kinds and states (P286).
const (
	CommentKindChange     = "change"
	CommentKindQuestion   = "question"
	CommentStateOpen      = "open"
	CommentStateAddressed = "addressed"
)

// MaxCommentBytes bounds one comment body or reply, after \r\n normalisation.
const MaxCommentBytes = 8 << 10

// AddComment inserts one comment, creating (repoID, branch)'s review_session row if this is the
// first thing ever recorded for it and bumping last_used_at either way. Returns the row with
// its assigned id — AUTOINCREMENT guarantees that id is never reused by a later insert even after
// this row is deleted, which is what makes it safe for a client to hold across a
// remove.
func (s *Store) AddComment(ctx context.Context, repoID, branch string, c Comment) (Comment, error) {
	db, err := s.conn()
	if err != nil {
		return Comment{}, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Comment{}, fmt.Errorf("gitreview: begin AddComment: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit has succeeded

	sessionID, err := upsertSession(ctx, tx, repoID, branch, time.Now().UnixMilli())
	if err != nil {
		return Comment{}, err
	}

	if c.Kind == "" {
		c.Kind = CommentKindChange
	}
	if c.Origin == "" {
		c.Origin = OriginCommit
	}
	var anchorContent []byte
	if c.Origin == OriginWorking && len(c.AnchorContent) > 0 {
		if anchorContent, err = Compress(c.AnchorContent); err != nil {
			return Comment{}, err
		}
	}
	c.State = CommentStateOpen
	res, err := tx.ExecContext(ctx, `
		INSERT INTO review_comment
			(session_id, path, start_line, end_line, body, anchor_sha, anchor_blob_oid, created_at, kind, origin, anchor_content)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, c.Path, c.Range.Start, c.Range.End, c.Body, c.AnchorSHA, c.AnchorBlobOID,
		c.CreatedAt.UnixMilli(), c.Kind, c.Origin, anchorContent)
	if err != nil {
		return Comment{}, fmt.Errorf("gitreview: insert review_comment: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Comment{}, fmt.Errorf("gitreview: read review_comment id: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Comment{}, fmt.Errorf("gitreview: commit AddComment: %w", err)
	}
	c.ID = id
	return c, nil
}

// Comments returns every comment for (repoID, branch) in stored-coordinate order (path, start,
// end, created_at, id) — a stable starting point only: the authoritative order is applied by
// SortAnchored after projection, since projection can reorder two comments inside one file. nil
// (never an error) when no session exists for (repoID, branch).
func (s *Store) Comments(ctx context.Context, repoID, branch string) ([]Comment, error) {
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	id, ok, err := sessionID(ctx, db, repoID, branch)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	return queryComments(ctx, db, id, false)
}

// OpenComments is Comments filtered to state open, in the same stored order.
func (s *Store) OpenComments(ctx context.Context, repoID, branch string) ([]Comment, error) {
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	id, ok, err := sessionID(ctx, db, repoID, branch)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return queryComments(ctx, db, id, true)
}

func queryComments(ctx context.Context, db *sql.DB, sessionID int64, openOnly bool) ([]Comment, error) {
	q := `
		SELECT id, path, start_line, end_line, body, anchor_sha, anchor_blob_oid, created_at,
			kind, state, reply, replied_at, sent_at, origin, anchor_content
		FROM review_comment WHERE session_id = ?`
	if openOnly {
		q += ` AND state = 'open'`
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY path, start_line, end_line, created_at, id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("gitreview: query review_comment: %w", err)
	}
	defer rows.Close()

	var out []Comment
	for rows.Next() {
		var c Comment
		var createdAtMillis int64
		var repliedAt, sentAt sql.NullInt64
		var anchorContent []byte
		if err := rows.Scan(&c.ID, &c.Path, &c.Range.Start, &c.Range.End, &c.Body,
			&c.AnchorSHA, &c.AnchorBlobOID, &createdAtMillis,
			&c.Kind, &c.State, &c.Reply, &repliedAt, &sentAt, &c.Origin, &anchorContent); err != nil {
			return nil, fmt.Errorf("gitreview: scan review_comment: %w", err)
		}
		c.CreatedAt = time.UnixMilli(createdAtMillis)
		if len(anchorContent) > 0 {
			if c.AnchorContent, err = decompressAnchor(anchorContent); err != nil {
				return nil, err
			}
		}
		if repliedAt.Valid {
			t := time.UnixMilli(repliedAt.Int64)
			c.RepliedAt = &t
		}
		if sentAt.Valid {
			t := time.UnixMilli(sentAt.Int64)
			c.SentAt = &t
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gitreview: iterate review_comment: %w", err)
	}
	return out, nil
}

// SetCommentState reopens or addresses one comment. false when the id matches nothing in this
// session.
func (s *Store) SetCommentState(ctx context.Context, repoID, branch string, id int64, state string) (bool, error) {
	return s.updateComment(ctx, repoID, branch, `state = ?`, []any{state}, id)
}

// EditComment replaces body and kind. Range, anchor, state and reply stay; sent_at clears, since the
// edited text was never sent.
func (s *Store) EditComment(ctx context.Context, repoID, branch string, id int64, body, kind string) (bool, error) {
	return s.updateComment(ctx, repoID, branch, `body = ?, kind = ?, sent_at = NULL`, []any{body, kind}, id)
}

// ReplyComment stores an agent reply; resolved moves the comment to addressed, otherwise it stays open.
func (s *Store) ReplyComment(ctx context.Context, repoID, branch string, id int64, reply string, resolved bool, at time.Time) (bool, error) {
	state := CommentStateOpen
	if resolved {
		state = CommentStateAddressed
	}
	return s.updateComment(ctx, repoID, branch, `reply = ?, replied_at = ?, state = ?`,
		[]any{reply, at.UnixMilli(), state}, id)
}

// MarkSent stamps sent_at on the given comments of this session with one update and one notify.
func (s *Store) MarkSent(ctx context.Context, repoID, branch string, ids []int64, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.updateComments(ctx, repoID, branch, `sent_at = ?`, []any{at.UnixMilli()}, ids)
	return err
}

func (s *Store) updateComment(ctx context.Context, repoID, branch, set string, args []any, id int64) (bool, error) {
	return s.updateComments(ctx, repoID, branch, set, args, []int64{id})
}

func (s *Store) updateComments(ctx context.Context, repoID, branch, set string, args []any, ids []int64) (bool, error) {
	db, err := s.conn()
	if err != nil {
		return false, err
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args = append([]any{}, args...)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, repoID, branch)
	res, err := db.ExecContext(ctx, `UPDATE review_comment SET `+set+` WHERE id IN (`+marks+`) AND session_id = (
		SELECT id FROM review_session WHERE repo_id = ? AND branch = ?
	)`, args...)
	if err != nil {
		return false, fmt.Errorf("gitreview: update review_comment: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("gitreview: read RowsAffected: %w", err)
	}
	return n > 0, nil
}

// RemoveComment deletes one comment, scoped to (repoID, branch)'s own session so an id belonging
// to another session matches nothing. false (not an error) when the row is already gone (D11) —
// two windows sharing one session is the designed state, not a client mistake.
func (s *Store) RemoveComment(ctx context.Context, repoID, branch string, id int64) (bool, error) {
	db, err := s.conn()
	if err != nil {
		return false, err
	}
	res, err := db.ExecContext(ctx, `
		DELETE FROM review_comment WHERE id = ? AND session_id = (
			SELECT id FROM review_session WHERE repo_id = ? AND branch = ?
		)
	`, id, repoID, branch)
	if err != nil {
		return false, fmt.Errorf("gitreview: delete review_comment: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("gitreview: read RowsAffected: %w", err)
	}
	return n > 0, nil
}

// RemoveComments deletes the given comments of this session with one statement. Returns how many
// it removed.
func (s *Store) RemoveComments(ctx context.Context, repoID, branch string, ids []int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	db, err := s.conn()
	if err != nil {
		return 0, err
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+2)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, repoID, branch)
	res, err := db.ExecContext(ctx, `DELETE FROM review_comment WHERE id IN (`+marks+`) AND session_id = (
		SELECT id FROM review_session WHERE repo_id = ? AND branch = ?
	)`, args...)
	if err != nil {
		return 0, fmt.Errorf("gitreview: delete review_comments: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("gitreview: read RowsAffected: %w", err)
	}
	return int(n), nil
}

// ClearComments removes every comment for one session and nothing else — never a review_file, a
// review_range, or the session row itself (D14). Runs no incremental_vacuum (the sweep's and
// Purge's own job, D14). Returns how many were removed.
func (s *Store) ClearComments(ctx context.Context, repoID, branch string) (int, error) {
	db, err := s.conn()
	if err != nil {
		return 0, err
	}
	res, err := db.ExecContext(ctx, `
		DELETE FROM review_comment WHERE session_id = (
			SELECT id FROM review_session WHERE repo_id = ? AND branch = ?
		)
	`, repoID, branch)
	if err != nil {
		return 0, fmt.Errorf("gitreview: clear review_comment: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("gitreview: read RowsAffected: %w", err)
	}
	return int(n), nil
}

// decompressAnchor inflates a stored anchor_content BLOB. Its length is bounded by MaxSnapshotBytes
// at write time; the decoder is capped one byte past it.
func decompressAnchor(compressed []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(compressed))
	defer r.Close()
	raw, err := io.ReadAll(io.LimitReader(r, MaxSnapshotBytes+1))
	if err != nil {
		return nil, fmt.Errorf("gitreview: flate decode anchor_content: %w", err)
	}
	if len(raw) > MaxSnapshotBytes {
		return nil, fmt.Errorf("gitreview: anchor_content exceeds %d bytes", MaxSnapshotBytes)
	}
	return raw, nil
}
