package repos

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/huandu/go-sqlbuilder"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// Structural limits of a proof session; payload limits live in internal/proof.
const (
	ProofMaxSteps        = 100
	ProofMaxSessionBytes = 32 << 20
)

// ProofCode classifies a ProofError.
type ProofCode string

const (
	ProofNotFound  ProofCode = "notFound"
	ProofNameTaken ProofCode = "nameTaken"
	ProofBadState  ProofCode = "badState"
	ProofLimit     ProofCode = "limit"
	ProofBadRef    ProofCode = "badRef"
)

// ProofError is a caller-correctable failure; Msg is agent-facing English.
type ProofError struct {
	Code ProofCode
	Msg  string
}

func (e *ProofError) Error() string { return e.Msg }

func proofErr(code ProofCode, format string, args ...any) error {
	return &ProofError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// ProofRepo reads and writes the proof_* tables (P420). Every multi-row change is one transaction;
// the pool holds one connection with foreign_keys ON (internal/sqlitex), so cascades fire and no
// query runs while a transaction is open.
type ProofRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// ProofSessionRow is one proof_sessions row.
type ProofSessionRow struct {
	ID, Name, NameKey, State, Summary string
	CurrentRev, PendingRev            *int
	CreatedAt, UpdatedAt              int64
	ClosedAt, SeenAt                  *int64
}

// ProofRevisionRow is one proof_revisions row.
type ProofRevisionRow struct {
	Rev           int
	DraftID       *string
	Reason, State string
	CreatedAt     int64
	ClosedAt      *int64
}

// ProofStepRow is one proof_steps row. Payload is nil in listings.
type ProofStepRow struct {
	SessionID                                  string
	Rev, Ordinal                               int
	StepKey                                    string
	Description, Query, Language, Source, View string
	Payload                                    []byte
	PayloadBytes                               int
	Diff                                       string
	OriginOrdinal                              *int
}

// ProofViewStateRow is one proof_view_state row.
type ProofViewStateRow struct {
	Ordinal int
	State   string
}

// ProofDraft describes an open draft.
type ProofDraft struct {
	SessionID, Name string
	Rev             int
	Edit            bool
	Steps           int
}

var proofSessionCols = []string{
	kiraschema.ProofSessions.ID, kiraschema.ProofSessions.Name, kiraschema.ProofSessions.NameKey, kiraschema.ProofSessions.State,
	kiraschema.ProofSessions.CurrentRev, kiraschema.ProofSessions.PendingRev, kiraschema.ProofSessions.Summary,
	kiraschema.ProofSessions.CreatedAt, kiraschema.ProofSessions.ClosedAt, kiraschema.ProofSessions.SeenAt,
	kiraschema.ProofSessions.UpdatedAt,
}

// proofAlias qualifies cols with a table alias.
func proofAlias(alias string, cols ...string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = alias + "." + c
	}
	return out
}

// proofSummarySelect is the history-list select shared by Sessions, Unreviewed and
// UnattachedSessions; the caller adds the filter and order.
func (r *ProofRepo) proofSummarySelect() *sqlbuilder.SelectBuilder {
	s, st := kiraschema.ProofSessions, kiraschema.ProofSteps
	return r.Dialect.Select(
		"s."+s.ID, "s."+s.Name, "s."+s.ClosedAt, "s."+s.PendingRev+" IS NOT NULL", "s."+s.SeenAt+" IS NOT NULL",
		"(SELECT COUNT(*) FROM "+st.Table+" st WHERE st."+st.SessionID+" = s."+s.ID+" AND st."+st.Rev+" = s."+s.CurrentRev+")",
	).From(s.Table + " s")
}

func scanProofSession(row rowScanner) (ProofSessionRow, error) {
	var s ProofSessionRow
	var cur, pend sql.NullInt64
	var closed, seen sql.NullInt64
	if err := row.Scan(&s.ID, &s.Name, &s.NameKey, &s.State, &cur, &pend, &s.Summary, &s.CreatedAt, &closed, &seen, &s.UpdatedAt); err != nil {
		return ProofSessionRow{}, err
	}
	s.CurrentRev, s.PendingRev = nullInt(cur), nullInt(pend)
	s.ClosedAt, s.SeenAt = nullInt64(closed), nullInt64(seen)
	return s, nil
}

func nullInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func nullInt64(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}

func (r *ProofRepo) tx(fn func(tx *sql.Tx) error) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos/proof: begin: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos/proof: commit: %w", err)
	}
	return nil
}

// ProofSummary is one closed session in the history list.
type ProofSummary struct {
	ID, Name      string
	ClosedAt      int64
	Steps         int
	Pending, Seen bool
	// TaskTitle is the title of the task holding the session; "" when none does.
	TaskTitle string
}

// Session returns one session; ok is false when absent or still open.
func (r *ProofRepo) Session(id string) (ProofSessionRow, bool, error) {
	ps := kiraschema.ProofSessions
	sb := r.Dialect.Select(proofSessionCols...).From(ps.Table)
	sb.Where(sb.Equal(ps.ID, id), sb.Equal(ps.State, "closed"))
	s, err := scanProofSession(sqlb.QueryRow(r.DB, sb))
	if errors.Is(err, sql.ErrNoRows) {
		return ProofSessionRow{}, false, nil
	}
	return s, err == nil, err
}

// Revision returns the revision row; ok is false when absent.
func (r *ProofRepo) Revision(id string, rev int) (ProofRevisionRow, bool, error) {
	var v ProofRevisionRow
	var draft sql.NullString
	var closed sql.NullInt64
	pr := kiraschema.ProofRevisions
	sb := r.Dialect.Select(pr.Rev, pr.DraftID, pr.Reason, pr.State, pr.CreatedAt, pr.ClosedAt).From(pr.Table)
	sb.Where(sb.Equal(pr.SessionID, id), sb.Equal(pr.Rev, rev), sb.NotEqual(pr.State, "draft"))
	err := sqlb.QueryRow(r.DB, sb).
		Scan(&v.Rev, &draft, &v.Reason, &v.State, &v.CreatedAt, &closed)
	if errors.Is(err, sql.ErrNoRows) {
		return ProofRevisionRow{}, false, nil
	}
	v.ClosedAt = nullInt64(closed)
	if draft.Valid {
		v.DraftID = &draft.String
	}
	return v, err == nil, err
}

// Steps lists one revision's steps in ordinal order, without payload.
func (r *ProofRepo) Steps(id string, rev int) ([]ProofStepRow, error) {
	c := kiraschema.ProofSteps
	sb := r.Dialect.Select(c.Ordinal, c.Description, c.Query, c.Language, c.Source, c.View, c.PayloadBytes, c.Diff, c.OriginOrdinal, c.StepKey).From(c.Table)
	sb.Where(sb.Equal(c.SessionID, id), sb.Equal(c.Rev, rev)).OrderBy(c.Ordinal)
	rows, err := sqlb.Query(r.DB, sb)
	return queryAll(rows, err, func(rows *sql.Rows) (ProofStepRow, error) {
		s := ProofStepRow{SessionID: id, Rev: rev}
		var origin sql.NullInt64
		err := rows.Scan(&s.Ordinal, &s.Description, &s.Query, &s.Language, &s.Source, &s.View, &s.PayloadBytes, &s.Diff, &origin, &s.StepKey)
		s.OriginOrdinal = nullInt(origin)
		return s, err
	})
}

// Step returns one step with its payload; ok is false when absent or its revision is a draft.
func (r *ProofRepo) Step(id string, rev, ordinal int) (ProofStepRow, bool, error) {
	s := ProofStepRow{SessionID: id, Rev: rev, Ordinal: ordinal}
	var origin sql.NullInt64
	c := kiraschema.ProofSteps
	sb := r.stepOfFinishedRevision(id, rev, ordinal,
		"st."+c.Description, "st."+c.Query, "st."+c.Language, "st."+c.Source, "st."+c.View, "st."+c.Payload,
		"st."+c.PayloadBytes, "st."+c.Diff, "st."+c.OriginOrdinal, "st."+c.StepKey)
	err := sqlb.QueryRow(r.DB, sb).
		Scan(&s.Description, &s.Query, &s.Language, &s.Source, &s.View, &s.Payload, &s.PayloadBytes, &s.Diff, &origin, &s.StepKey)
	if errors.Is(err, sql.ErrNoRows) {
		return ProofStepRow{}, false, nil
	}
	s.OriginOrdinal = nullInt(origin)
	return s, err == nil, err
}

// ViewStates lists the saved view state of every step of one revision.
func (r *ProofRepo) ViewStates(id string, rev int) ([]ProofViewStateRow, error) {
	c := kiraschema.ProofViewState
	sb := r.Dialect.Select(c.Ordinal, c.State).From(c.Table)
	sb.Where(sb.Equal(c.SessionID, id), sb.Equal(c.Rev, rev)).OrderBy(c.Ordinal)
	rows, err := sqlb.Query(r.DB, sb)
	return queryAll(rows, err, func(rows *sql.Rows) (ProofViewStateRow, error) {
		var v ProofViewStateRow
		return v, rows.Scan(&v.Ordinal, &v.State)
	})
}

// SetViewState upserts one step's view state. The step must belong to a closed or pending revision.
func (r *ProofRepo) SetViewState(id string, rev, ordinal int, state string, now time.Time) error {
	return r.tx(func(tx *sql.Tx) error {
		var one int
		err := sqlb.QueryRow(tx, r.stepOfFinishedRevision(id, rev, ordinal, "1")).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return proofErr(ProofNotFound, "no such proof step")
		}
		if err != nil {
			return err
		}
		vs := kiraschema.ProofViewState
		ib := r.Dialect.InsertInto(vs.Table).Cols(vs.SessionID, vs.Rev, vs.Ordinal, vs.State, vs.UpdatedAt).
			Values(id, rev, ordinal, state, now.UnixMilli())
		sqlb.Upsert(ib, []string{vs.SessionID, vs.Rev, vs.Ordinal}, []string{vs.State, vs.UpdatedAt}, "")
		_, err = sqlb.Exec(tx, ib)
		return err
	})
}

// AcceptEdit makes the pending revision current, copies the view state of unchanged steps onto
// it and deletes the old revision.
func (r *ProofRepo) AcceptEdit(id string, now time.Time) error {
	return r.tx(func(tx *sql.Tx) error { return ProofCache{r.Dialect}.Accept(tx, id, now) })
}

// Accept is AcceptEdit inside the caller's transaction.
func (c ProofCache) Accept(tx sqlb.Querier, id string, now time.Time) error {
	cur, pend, err := revs(tx, c.Dialect, id)
	if err != nil {
		return err
	}
	ts := now.UnixMilli()
	vs, st, pr, ps := kiraschema.ProofViewState, kiraschema.ProofSteps, kiraschema.ProofRevisions, kiraschema.ProofSessions
	sel := c.Dialect.Select("n."+st.SessionID, "n."+st.Rev, "n."+st.Ordinal, "v."+vs.State, "v."+vs.UpdatedAt).From(st.Table + " n")
	sel.Join(vs.Table+" v", "v."+vs.SessionID+" = n."+st.SessionID, "v."+vs.Rev+" = "+sel.Var(cur), "v."+vs.Ordinal+" = n."+st.OriginOrdinal)
	sel.Where(sel.Equal("n."+st.SessionID, id), sel.Equal("n."+st.Rev, pend), sel.Equal("n."+st.Diff, "unchanged"))
	copyView := c.Dialect.InsertInto(vs.Table).Cols(vs.SessionID, vs.Rev, vs.Ordinal, vs.State, vs.UpdatedAt)
	copyView.SQL(copyView.Var(sel))
	if _, err := sqlb.Exec(tx, copyView); err != nil {
		return err
	}
	dropOld := c.Dialect.DeleteFrom(pr.Table)
	dropOld.Where(dropOld.Equal(pr.SessionID, id), dropOld.Equal(pr.Rev, cur))
	if _, err := sqlb.Exec(tx, dropOld); err != nil {
		return err
	}
	promote := c.Dialect.Update(pr.Table)
	promote.Set(promote.Assign(pr.State, "current")).Where(promote.Equal(pr.SessionID, id), promote.Equal(pr.Rev, pend))
	if _, err := sqlb.Exec(tx, promote); err != nil {
		return err
	}
	// A pending edit carries the proposed name and summary; empty means the edit kept them.
	var name, summary string
	rq := c.Dialect.Select(pr.Name, pr.Summary).From(pr.Table)
	rq.Where(rq.Equal(pr.SessionID, id), rq.Equal(pr.Rev, pend))
	if err := sqlb.QueryRow(tx, rq).Scan(&name, &summary); err != nil {
		return err
	}
	sess := c.Dialect.Update(ps.Table)
	sets := []string{sess.Assign(ps.CurrentRev, pend), ps.PendingRev + " = NULL", sess.Assign(ps.UpdatedAt, ts)}
	if name != "" {
		var taskID string
		tl := kiraschema.ProofTaskLinks
		lq := c.Dialect.Select(tl.TaskID).From(tl.Table)
		lq.Where(lq.Equal(tl.SessionID, id))
		if err := sqlb.QueryRow(tx, lq).Scan(&taskID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		sets = append(sets, sess.Assign(ps.Name, name), sess.Assign(ps.NameKey, ProofNameKey(taskID, name)), sess.Assign(ps.Summary, summary))
	}
	sess.Set(sets...).Where(sess.Equal(ps.ID, id))
	_, err = sqlb.Exec(tx, sess)
	return err
}

// ProofNameKey is the uniqueness key of a proof name: names are unique per task, case-insensitively.
func ProofNameKey(taskID, name string) string { return taskID + "\x00" + strings.ToLower(name) }

// RejectEdit drops the pending revision.
func (r *ProofRepo) RejectEdit(id string, now time.Time) error {
	return r.tx(func(tx *sql.Tx) error {
		_, pend, err := revs(tx, r.Dialect, id)
		if err != nil {
			return err
		}
		pr, ps := kiraschema.ProofRevisions, kiraschema.ProofSessions
		del := r.Dialect.DeleteFrom(pr.Table)
		del.Where(del.Equal(pr.SessionID, id), del.Equal(pr.Rev, pend))
		if _, err := sqlb.Exec(tx, del); err != nil {
			return err
		}
		sess := r.Dialect.Update(ps.Table)
		sess.Set(ps.PendingRev+" = NULL", sess.Assign(ps.UpdatedAt, now.UnixMilli())).Where(sess.Equal(ps.ID, id))
		_, err = sqlb.Exec(tx, sess)
		return err
	})
}

func revs(tx sqlb.Querier, d sqlb.Dialect, id string) (cur, pend int, err error) {
	var c, p sql.NullInt64
	ps := kiraschema.ProofSessions
	sb := d.Select(ps.CurrentRev, ps.PendingRev).From(ps.Table)
	sb.Where(sb.Equal(ps.ID, id), sb.Equal(ps.State, "closed"))
	err = sqlb.QueryRow(tx, sb).Scan(&c, &p)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, proofErr(ProofNotFound, "no such proof session")
	}
	if err != nil {
		return 0, 0, err
	}
	if !c.Valid || !p.Valid {
		return 0, 0, proofErr(ProofBadState, "the session has no pending edit")
	}
	return int(c.Int64), int(p.Int64), nil
}

// Delete removes a closed session with everything under it. ok is false when absent.
func (r *ProofRepo) Delete(id string) (bool, error) {
	ps := kiraschema.ProofSessions
	del := r.Dialect.DeleteFrom(ps.Table)
	del.Where(del.Equal(ps.ID, id), del.Equal(ps.State, "closed"))
	res, err := sqlb.Exec(r.DB, del)
	if err != nil {
		return false, fmt.Errorf("repos/proof: delete: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// MarkSeen stamps a closed session as seen by the user.
func (r *ProofRepo) MarkSeen(id string, now time.Time) error {
	ts := now.UnixMilli()
	ps := kiraschema.ProofSessions
	ub := r.Dialect.Update(ps.Table)
	ub.Set(ub.Assign(ps.SeenAt, ts)).Where(ub.Equal(ps.ID, id), ub.Equal(ps.State, "closed"))
	_, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("repos/proof: mark seen: %w", err)
	}
	return nil
}

// Unreviewed lists closed sessions never opened by the user or holding a pending edit, oldest
// first: the ones whose prompt a restart re-raises.
func (r *ProofRepo) Unreviewed() ([]ProofSummary, error) {
	ps := kiraschema.ProofSessions
	sb := r.proofSummarySelect()
	sb.Where(sb.Equal("s."+ps.State, "closed"), sb.Or(sb.IsNull("s."+ps.SeenAt), sb.IsNotNull("s."+ps.PendingRev))).
		OrderBy("s."+ps.ClosedAt, "s."+sqlb.RowID)
	rows, err := sqlb.Query(r.DB, sb)
	return queryAll(rows, err, func(rows *sql.Rows) (ProofSummary, error) {
		var p ProofSummary
		return p, rows.Scan(&p.ID, &p.Name, &p.ClosedAt, &p.Pending, &p.Seen, &p.Steps)
	})
}

func queryAll[T any](rows *sql.Rows, err error, scan func(*sql.Rows) (T, error)) ([]T, error) {
	if err != nil {
		return nil, fmt.Errorf("repos/proof: query: %w", err)
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("repos/proof: scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos/proof: rows: %w", err)
	}
	return out, nil
}

// ProofTaskLink is one proof_task_links row, minus the session it belongs to.
type ProofTaskLink struct {
	TaskID, Source, KiraSessionID, StepID, StepName string
	// Slug is the proof folder, proofs/<slug>/, of the session's files.
	Slug string
}

// ProofTaskSession is a closed session attached to a task, with its current revision's steps
// (no payload).
type ProofTaskSession struct {
	Session  ProofSessionRow
	Link     ProofTaskLink
	LinkedAt int64
	Steps    []ProofStepRow
}

// TaskSessions lists the sessions attached to a task, newest closed first, each with its current
// revision's steps. Two queries, grouped in Go.
func (r *ProofRepo) TaskSessions(taskID string) ([]ProofTaskSession, error) {
	ps, tl, st := kiraschema.ProofSessions, kiraschema.ProofTaskLinks, kiraschema.ProofSteps
	sb := r.Dialect.Select(append(proofAlias("s", proofSessionCols...),
		proofAlias("l", tl.Source, tl.KiraSessionID, tl.StepID, tl.StepName, tl.LinkedAt, tl.Slug)...)...).From(tl.Table + " l")
	sb.Join(ps.Table+" s", "s."+ps.ID+" = l."+tl.SessionID)
	sb.Where(sb.Equal("l."+tl.TaskID, taskID), sb.Equal("s."+ps.State, "closed")).
		OrderBy("s."+ps.ClosedAt+" DESC", "s."+sqlb.RowID+" DESC")
	rows, err := sqlb.Query(r.DB, sb)
	list, err := queryAll(rows, err, func(rows *sql.Rows) (ProofTaskSession, error) {
		var t ProofTaskSession
		var cur, pend, closed, seen sql.NullInt64
		s := &t.Session
		err := rows.Scan(&s.ID, &s.Name, &s.NameKey, &s.State, &cur, &pend, &s.Summary, &s.CreatedAt, &closed, &seen, &s.UpdatedAt,
			&t.Link.Source, &t.Link.KiraSessionID, &t.Link.StepID, &t.Link.StepName, &t.LinkedAt, &t.Link.Slug)
		s.CurrentRev, s.PendingRev, s.ClosedAt, s.SeenAt = nullInt(cur), nullInt(pend), nullInt64(closed), nullInt64(seen)
		t.Link.TaskID = taskID
		return t, err
	})
	if err != nil || len(list) == 0 {
		return list, err
	}
	sq := r.Dialect.Select(proofAlias("st", st.SessionID, st.Rev, st.Ordinal, st.Description, st.Query, st.Language, st.Source,
		st.View, st.PayloadBytes, st.Diff, st.OriginOrdinal, st.StepKey)...).From(tl.Table + " l")
	sq.Join(ps.Table+" s", "s."+ps.ID+" = l."+tl.SessionID)
	sq.Join(st.Table+" st", "st."+st.SessionID+" = s."+ps.ID, "st."+st.Rev+" = s."+ps.CurrentRev)
	sq.Where(sq.Equal("l."+tl.TaskID, taskID), sq.Equal("s."+ps.State, "closed")).OrderBy("st."+st.SessionID, "st."+st.Ordinal)
	srows, err := sqlb.Query(r.DB, sq)
	steps, err := queryAll(srows, err, func(rows *sql.Rows) (ProofStepRow, error) {
		var st ProofStepRow
		var origin sql.NullInt64
		err := rows.Scan(&st.SessionID, &st.Rev, &st.Ordinal, &st.Description, &st.Query, &st.Language, &st.Source, &st.View, &st.PayloadBytes, &st.Diff, &origin, &st.StepKey)
		st.OriginOrdinal = nullInt(origin)
		return st, err
	})
	if err != nil {
		return nil, err
	}
	at := make(map[string]int, len(list))
	for i := range list {
		at[list[i].Session.ID] = i
	}
	for _, st := range steps {
		i := at[st.SessionID]
		list[i].Steps = append(list[i].Steps, st)
	}
	return list, nil
}

// stepOfFinishedRevision selects cols (aliases st = proof_steps, rv = proof_revisions) of one step whose revision is not a draft.
func (r *ProofRepo) stepOfFinishedRevision(id string, rev, ordinal int, cols ...string) *sqlbuilder.SelectBuilder {
	st, pr := kiraschema.ProofSteps, kiraschema.ProofRevisions
	sb := r.Dialect.Select(cols...).From(st.Table + " st")
	sb.Join(pr.Table+" rv", "rv."+pr.SessionID+" = st."+st.SessionID, "rv."+pr.Rev+" = st."+st.Rev)
	sb.Where(sb.Equal("st."+st.SessionID, id), sb.Equal("st."+st.Rev, rev), sb.Equal("st."+st.Ordinal, ordinal),
		sb.NotEqual("rv."+pr.State, "draft"))
	return sb
}
