package repos

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// Log kinds: a run log is keyed by run id, a worktree-setup log by branch id.
const (
	AdeLogRun   = "run"
	AdeLogSetup = "setup"
)

const (
	// AdeLogTailBytes is the tail kept per log; older chunks drop and set truncated.
	AdeLogTailBytes = 2 << 20
	// AdeLogChunkBytes caps one chunk; a longer line is cut with an ellipsis.
	AdeLogChunkBytes = 8 << 10
	adeLogPageLimit  = 500
	// adeLogTrimScan is the first prefix read size when trimming: one flush appends a handful of
	// chunks, so a trim usually drops about as many.
	adeLogTrimScan = 64
)

// AdeLogChunk is one stored log chunk.
type AdeLogChunk struct {
	Seq    int
	At     int64
	Stream string
	Text   string
}

// AdeLogPage is one page of a log plus its counters.
type AdeLogPage struct {
	Chunks    []AdeLogChunk
	NextSeq   int
	Truncated bool
}

// AdeLogsRepo stores run and worktree-setup logs: the newest AdeLogTailBytes per log.
type AdeLogsRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// Reset empties a log (creating it) and zeroes its counters.
func (r *AdeLogsRepo) Reset(kind, id, taskID string) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin reset ade log: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	del := r.Dialect.DeleteFrom(kiraschema.AdeLogChunks.Table)
	del.Where(del.Equal(kiraschema.AdeLogChunks.Kind, kind), del.Equal(kiraschema.AdeLogChunks.ID, id))
	if _, err := sqlb.Exec(tx, del); err != nil {
		return fmt.Errorf("repos: reset ade log chunks %s/%s: %w", kind, id, err)
	}
	l := kiraschema.AdeLogs
	ib := r.Dialect.InsertInto(l.Table).Cols(l.Kind, l.ID, l.TaskID, l.NextSeq, l.Bytes, l.Truncated).Values(kind, id, taskID, 1, 0, 0)
	sqlb.UpsertSet(ib, []string{l.Kind, l.ID}, []string{l.TaskID + " = excluded." + l.TaskID, l.NextSeq + " = 1", l.Bytes + " = 0", l.Truncated + " = 0"}, "")
	if _, err := sqlb.Exec(tx, ib); err != nil {
		return fmt.Errorf("repos: reset ade log %s/%s: %w", kind, id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit reset ade log: %w", err)
	}
	return nil
}

// Append stores chunks (Seq assigned here) in one transaction, dropping the oldest while the log
// exceeds AdeLogTailBytes, and returns the stored chunks with their seq. The log must exist (Reset).
func (r *AdeLogsRepo) Append(kind, id string, chunks []AdeLogChunk) ([]AdeLogChunk, error) {
	if len(chunks) == 0 {
		return nil, nil
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("repos: begin append ade log: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var next int
	var bytes int64
	var truncated int
	l, ch := kiraschema.AdeLogs, kiraschema.AdeLogChunks
	state := r.Dialect.Select(l.NextSeq, l.Bytes, l.Truncated).From(l.Table)
	state.Where(state.Equal(l.Kind, kind), state.Equal(l.ID, id))
	if err := sqlb.QueryRow(tx, state).Scan(&next, &bytes, &truncated); err != nil {
		return nil, fmt.Errorf("repos: read ade log %s/%s: %w", kind, id, err)
	}
	out := make([]AdeLogChunk, 0, len(chunks))
	for _, c := range chunks {
		c.Seq = next
		next++
		c.Text = clipChunk(c.Text)
		bytes += int64(len(c.Text))
		ib := r.Dialect.InsertInto(ch.Table).Cols(ch.Kind, ch.ID, ch.Seq, ch.At, ch.Stream, ch.Text).
			Values(kind, id, c.Seq, c.At, c.Stream, c.Text)
		if _, err := sqlb.Exec(tx, ib); err != nil {
			return nil, fmt.Errorf("repos: append ade log chunk %s/%s: %w", kind, id, err)
		}
		out = append(out, c)
	}
	if bytes > AdeLogTailBytes {
		dropped, err := r.trimLogTail(tx, kind, id, bytes-AdeLogTailBytes)
		if err != nil {
			return nil, err
		}
		bytes -= dropped
		truncated = 1
	}
	ub := r.Dialect.Update(l.Table)
	ub.Set(ub.Assign(l.NextSeq, next), ub.Assign(l.Bytes, bytes), ub.Assign(l.Truncated, truncated)).
		Where(ub.Equal(l.Kind, kind), ub.Equal(l.ID, id))
	if _, err := sqlb.Exec(tx, ub); err != nil {
		return nil, fmt.Errorf("repos: update ade log %s/%s: %w", kind, id, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("repos: commit append ade log: %w", err)
	}
	return out, nil
}

// trimLogTail deletes the shortest oldest prefix of a log's chunks whose bytes reach excess and
// returns the bytes dropped. Only that prefix is read (the first n chunks by seq, n grown until
// the prefix fits), so the cost scales with chunks dropped, not with the stored tail. Every chunk
// goes when none reaches excess.
func (r *AdeLogsRepo) trimLogTail(tx *sql.Tx, kind, id string, excess int64) (int64, error) {
	ch := kiraschema.AdeLogChunks
	size := "length(CAST(" + ch.Text + " AS BLOB))"
	for n := int64(adeLogTrimScan); ; n *= 4 {
		q := r.Dialect.Select(ch.Seq, size).From(ch.Table)
		q.Where(q.Equal(ch.Kind, kind), q.Equal(ch.ID, id))
		q.OrderBy(ch.Seq).Limit(int(n))
		cutoff, dropped, rows, found, err := scanPrefix(tx, q, excess)
		if err != nil {
			return 0, fmt.Errorf("repos: find ade log cutoff %s/%s: %w", kind, id, err)
		}
		if !found && rows == n {
			continue // prefix not reached within n chunks; rescan wider
		}
		if !found {
			cutoff = math.MaxInt64 // fewer than n chunks and none reached excess: drop all
		}
		drop := r.Dialect.DeleteFrom(ch.Table)
		drop.Where(drop.Equal(ch.Kind, kind), drop.Equal(ch.ID, id), drop.LessEqualThan(ch.Seq, cutoff))
		if _, err := sqlb.Exec(tx, drop); err != nil {
			return 0, fmt.Errorf("repos: drop ade log chunks %s/%s: %w", kind, id, err)
		}
		return dropped, nil
	}
}

// scanPrefix sums chunk sizes in seq order. It returns the seq at which the sum first reaches
// excess with the bytes up to there (found), or all bytes read when it never does, plus the row
// count read.
func scanPrefix(tx *sql.Tx, q sqlb.Builder, excess int64) (cutoff, dropped, rows int64, found bool, err error) {
	rs, err := sqlb.Query(tx, q)
	if err != nil {
		return 0, 0, 0, false, err
	}
	defer rs.Close() //nolint:errcheck
	for rs.Next() {
		var seq, n int64
		if err := rs.Scan(&seq, &n); err != nil {
			return 0, 0, 0, false, err
		}
		rows++
		dropped += n
		if dropped >= excess {
			return seq, dropped, rows, true, rs.Err()
		}
	}
	return 0, dropped, rows, false, rs.Err()
}

// clipChunk cuts text over AdeLogChunkBytes at a rune boundary and marks the cut.
func clipChunk(text string) string {
	if len(text) <= AdeLogChunkBytes {
		return text
	}
	cut := AdeLogChunkBytes - len("…")
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

// Page returns up to 500 chunks with seq > afterSeq. An unknown log reads as empty.
func (r *AdeLogsRepo) Page(kind, id string, afterSeq int) (AdeLogPage, error) {
	page := AdeLogPage{Chunks: []AdeLogChunk{}, NextSeq: afterSeq}
	var truncated int
	l, ch := kiraschema.AdeLogs, kiraschema.AdeLogChunks
	head := r.Dialect.Select(l.Truncated).From(l.Table)
	head.Where(head.Equal(l.Kind, kind), head.Equal(l.ID, id))
	err := sqlb.QueryRow(r.DB, head).Scan(&truncated)
	if errors.Is(err, sql.ErrNoRows) {
		return page, nil
	}
	if err != nil {
		return page, fmt.Errorf("repos: read ade log %s/%s: %w", kind, id, err)
	}
	page.Truncated = truncated != 0
	sb := r.Dialect.Select(ch.Seq, ch.At, ch.Stream, ch.Text).From(ch.Table)
	sb.Where(sb.Equal(ch.Kind, kind), sb.Equal(ch.ID, id), sb.GreaterThan(ch.Seq, afterSeq)).OrderBy(ch.Seq).Limit(adeLogPageLimit)
	rows, err := sqlb.Query(r.DB, sb)
	chunks, err := sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (AdeLogChunk, bool, error) {
		var c AdeLogChunk
		err := rows.Scan(&c.Seq, &c.At, &c.Stream, &c.Text)
		return c, true, err
	})
	if err != nil {
		return page, fmt.Errorf("repos: page ade log %s/%s: %w", kind, id, err)
	}
	for _, c := range chunks {
		page.Chunks = append(page.Chunks, c)
		page.NextSeq = c.Seq
	}
	return page, nil
}
