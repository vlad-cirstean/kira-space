package repos

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/huandu/go-sqlbuilder"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// Structural limits of a proof session; payload limits live in internal/proof.
const (
	ProofMaxSteps        = 100
	ProofMaxSessionBytes = 32 << 20
	ProofMaxDrafts       = 5
	ProofKeepClosed      = 30
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

func sessionByKey(tx *sql.Tx, d sqlb.Dialect, nameKey string) (ProofSessionRow, bool, error) {
	c := kiraschema.ProofSessions
	sb := d.Select(proofSessionCols...).From(c.Table)
	sb.Where(sb.Equal(c.NameKey, nameKey))
	s, err := scanProofSession(sqlb.QueryRow(tx, sb))
	if errors.Is(err, sql.ErrNoRows) {
		return ProofSessionRow{}, false, nil
	}
	return s, err == nil, err
}

func draftCount(tx *sql.Tx, d sqlb.Dialect) (int, error) {
	var n int
	c := kiraschema.ProofRevisions
	sb := d.Select("COUNT(*)").From(c.Table)
	sb.Where(sb.Equal(c.State, "draft"))
	err := sqlb.QueryRow(tx, sb).Scan(&n)
	return n, err
}

// PurgeStaleDrafts drops drafts created before cutoff: never-closed sessions and unfinished edit
// revisions. Returns how many it dropped.
func (r *ProofRepo) PurgeStaleDrafts(cutoff time.Time) (int, error) {
	c := cutoff.UnixMilli()
	n := 0
	err := r.tx(func(tx *sql.Tx) error {
		ps := kiraschema.ProofSessions
		del := r.Dialect.DeleteFrom(ps.Table)
		del.Where(del.Equal(ps.State, "open"), del.LessThan(ps.CreatedAt, c))
		res, err := sqlb.Exec(tx, del)
		if err != nil {
			return err
		}
		a, _ := res.RowsAffected()
		n += int(a)
		pr := kiraschema.ProofRevisions
		del = r.Dialect.DeleteFrom(pr.Table)
		del.Where(del.Equal(pr.State, "draft"), del.LessThan(pr.CreatedAt, c))
		res, err = sqlb.Exec(tx, del)
		if err != nil {
			return err
		}
		a, _ = res.RowsAffected()
		n += int(a)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("repos/proof: purge drafts: %w", err)
	}
	return n, nil
}

// CreateSessionDraft opens a new, unclosed session with revision 1 as its draft.
func (r *ProofRepo) CreateSessionDraft(id, name, nameKey, summary, draftID string, now time.Time) error {
	ts := now.UnixMilli()
	return r.tx(func(tx *sql.Tx) error {
		if n, err := draftCount(tx, r.Dialect); err != nil {
			return err
		} else if n >= ProofMaxDrafts {
			return proofErr(ProofLimit, "%d proof drafts are already open; close one with proof_close_session first", n)
		}
		if ex, ok, err := sessionByKey(tx, r.Dialect, nameKey); err != nil {
			return err
		} else if ok {
			if ex.State == "open" {
				return proofErr(ProofNameTaken, "a proof session named %q is still open; pick another name, or close it first", ex.Name)
			}
			return proofErr(ProofNameTaken, "a proof session named %q already exists (closed %s); pick another name, or call proof_edit_session to change it", ex.Name, time.UnixMilli(*ex.ClosedAt).UTC().Format(time.RFC3339))
		}
		ps := kiraschema.ProofSessions
		ins := r.Dialect.InsertInto(ps.Table).Cols(ps.ID, ps.Name, ps.NameKey, ps.State, ps.Summary, ps.CreatedAt, ps.UpdatedAt).
			Values(id, name, nameKey, "open", summary, ts, ts)
		if _, err := sqlb.Exec(tx, ins); err != nil {
			return err
		}
		pr := kiraschema.ProofRevisions
		rev := r.Dialect.InsertInto(pr.Table).Cols(pr.SessionID, pr.Rev, pr.DraftID, pr.State, pr.CreatedAt).
			Values(id, 1, draftID, "draft", ts)
		_, err := sqlb.Exec(tx, rev)
		return err
	})
}

// CreateEditDraft opens a draft revision on a closed session and returns the session with its
// current steps (no payload).
func (r *ProofRepo) CreateEditDraft(nameKey, draftID, reason string, now time.Time) (ProofSessionRow, []ProofStepRow, error) {
	ts := now.UnixMilli()
	var sess ProofSessionRow
	var steps []ProofStepRow
	err := r.tx(func(tx *sql.Tx) error {
		s, ok, err := sessionByKey(tx, r.Dialect, nameKey)
		if err != nil {
			return err
		}
		switch {
		case !ok:
			return proofErr(ProofNotFound, "no proof session with that name; call proof_list_sessions for the existing names")
		case s.State != "closed" || s.CurrentRev == nil:
			return proofErr(ProofBadState, "proof session %q is still open; close it before editing", s.Name)
		case s.PendingRev != nil:
			return proofErr(ProofBadState, "proof session %q already has an edit waiting for the user's review", s.Name)
		}
		var open int
		pr := kiraschema.ProofRevisions
		oq := r.Dialect.Select("COUNT(*)").From(pr.Table)
		oq.Where(oq.Equal(pr.SessionID, s.ID), oq.Equal(pr.State, "draft"))
		if err := sqlb.QueryRow(tx, oq).Scan(&open); err != nil {
			return err
		}
		if open > 0 {
			return proofErr(ProofBadState, "proof session %q already has an edit draft open", s.Name)
		}
		if n, err := draftCount(tx, r.Dialect); err != nil {
			return err
		} else if n >= ProofMaxDrafts {
			return proofErr(ProofLimit, "%d proof drafts are already open; close one with proof_close_session first", n)
		}
		var rev int
		rq := r.Dialect.Select("COALESCE(MAX(" + pr.Rev + "), 0) + 1").From(pr.Table)
		rq.Where(rq.Equal(pr.SessionID, s.ID))
		if err := sqlb.QueryRow(tx, rq).Scan(&rev); err != nil {
			return err
		}
		ins := r.Dialect.InsertInto(pr.Table).Cols(pr.SessionID, pr.Rev, pr.DraftID, pr.Reason, pr.State, pr.CreatedAt).
			Values(s.ID, rev, draftID, reason, "draft", ts)
		if _, err := sqlb.Exec(tx, ins); err != nil {
			return err
		}
		pst := kiraschema.ProofSteps
		sq := r.Dialect.Select(pst.Ordinal, pst.Description, pst.View).From(pst.Table)
		sq.Where(sq.Equal(pst.SessionID, s.ID), sq.Equal(pst.Rev, *s.CurrentRev)).OrderBy(pst.Ordinal)
		rows, err := sqlb.Query(tx, sq)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			st := ProofStepRow{SessionID: s.ID, Rev: *s.CurrentRev}
			if err := rows.Scan(&st.Ordinal, &st.Description, &st.View); err != nil {
				return err
			}
			steps = append(steps, st)
		}
		sess = s
		return rows.Err()
	})
	return sess, steps, err
}

// ProofNewStep is what AddStep stores. For a kept step only Keep is set and every other content
// field comes from the old step.
type ProofNewStep struct {
	Description, Query, Language, Source, View string
	Payload                                    []byte
	Keep, Replaces                             *int
}

// applyRef resolves keepOrdinal/replacesOrdinal against the current revision's steps and fills step.
func applyRef(tx *sql.Tx, d sqlb.Dialect, sid string, rev int, curRev int64, in ProofNewStep, step *ProofStepRow) error {
	ref := in.Keep
	if in.Replaces != nil {
		ref = in.Replaces
	}
	if ref == nil {
		return nil
	}
	c := kiraschema.ProofSteps
	refq := d.Select("1").From(c.Table)
	refq.Where(refq.Equal(c.SessionID, sid), refq.Equal(c.Rev, rev), refq.Equal(c.OriginOrdinal, *ref))
	if err := sqlb.QueryRow(tx, refq).Scan(new(int)); err == nil {
		return proofErr(ProofBadRef, "old step %d is already referenced by another step of this draft", *ref)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var old ProofStepRow
	oldq := d.Select(c.Description, c.Query, c.Language, c.Source, c.View, c.Payload).From(c.Table)
	oldq.Where(oldq.Equal(c.SessionID, sid), oldq.Equal(c.Rev, curRev), oldq.Equal(c.Ordinal, *ref))
	err := sqlb.QueryRow(tx, oldq).Scan(&old.Description, &old.Query, &old.Language, &old.Source, &old.View, &old.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return proofErr(ProofBadRef, "the session has no step %d to keep or replace", *ref)
	}
	if err != nil {
		return err
	}
	step.OriginOrdinal = ref
	if in.Keep != nil {
		step.Description, step.Query, step.Language, step.Source, step.View, step.Payload = old.Description, old.Query, old.Language, old.Source, old.View, old.Payload
		step.Diff = "unchanged"
	} else {
		step.Diff = "changed"
	}
	return nil
}

// AddStep appends a step to an open draft and returns its ordinal.
func (r *ProofRepo) AddStep(draftID string, in ProofNewStep) (int, error) {
	var ordinal int
	err := r.tx(func(tx *sql.Tx) error {
		var sid string
		var rev int
		var cur sql.NullInt64
		err := sqlb.QueryRow(tx, r.draftSessionSelect(draftID, "r."+kiraschema.ProofRevisions.SessionID, "r."+kiraschema.ProofRevisions.Rev, "s."+kiraschema.ProofSessions.CurrentRev)).
			Scan(&sid, &rev, &cur)
		if errors.Is(err, sql.ErrNoRows) {
			return proofErr(ProofNotFound, "unknown or already closed draftId")
		}
		if err != nil {
			return err
		}
		edit := cur.Valid
		if (in.Keep != nil || in.Replaces != nil) && !edit {
			return proofErr(ProofBadRef, "keepOrdinal and replacesOrdinal only apply to a draft from proof_edit_session")
		}
		var count int
		pst := kiraschema.ProofSteps
		cq := r.Dialect.Select("COUNT(*)").From(pst.Table)
		cq.Where(cq.Equal(pst.SessionID, sid), cq.Equal(pst.Rev, rev))
		if err := sqlb.QueryRow(tx, cq).Scan(&count); err != nil {
			return err
		}
		if count >= ProofMaxSteps {
			return proofErr(ProofLimit, "a proof session holds at most %d steps", ProofMaxSteps)
		}
		step := ProofStepRow{SessionID: sid, Rev: rev, Description: in.Description, Query: in.Query, Language: in.Language,
			Source: in.Source, View: in.View, Payload: in.Payload, Diff: "added"}
		if err := applyRef(tx, r.Dialect, sid, rev, cur.Int64, in, &step); err != nil {
			return err
		}
		step.PayloadBytes = len(step.Payload)
		var used int
		uq := r.Dialect.Select("COALESCE(SUM(" + pst.PayloadBytes + "), 0)").From(pst.Table)
		uq.Where(uq.Equal(pst.SessionID, sid))
		if err := sqlb.QueryRow(tx, uq).Scan(&used); err != nil {
			return err
		}
		if used+step.PayloadBytes > ProofMaxSessionBytes {
			return proofErr(ProofLimit, "the session would exceed its %d MiB data limit", ProofMaxSessionBytes>>20)
		}
		oq := r.Dialect.Select("COALESCE(MAX(" + pst.Ordinal + "), 0) + 1").From(pst.Table)
		oq.Where(oq.Equal(pst.SessionID, sid), oq.Equal(pst.Rev, rev))
		if err := sqlb.QueryRow(tx, oq).Scan(&ordinal); err != nil {
			return err
		}
		ins := r.Dialect.InsertInto(pst.Table).
			Cols(pst.SessionID, pst.Rev, pst.Ordinal, pst.Description, pst.Query, pst.Language, pst.Source, pst.View, pst.Payload, pst.PayloadBytes, pst.Diff, pst.OriginOrdinal).
			Values(sid, rev, ordinal, step.Description, step.Query, step.Language, step.Source, step.View, step.Payload, step.PayloadBytes, step.Diff, step.OriginOrdinal)
		_, err = sqlb.Exec(tx, ins)
		return err
	})
	return ordinal, err
}

// ProofClosed is CloseDraft's result.
type ProofClosed struct {
	SessionID, Name string
	Rev, Steps      int
	Edit            bool
	Pruned          []string
}

// CloseDraft finishes a draft. A new session becomes closed with rev 1 current and the closed
// sessions beyond the newest ProofKeepClosed are pruned; an edit becomes the pending revision.
func (r *ProofRepo) CloseDraft(draftID string, now time.Time) (ProofClosed, error) {
	var out ProofClosed
	err := r.tx(func(tx *sql.Tx) error {
		var cur sql.NullInt64
		err := sqlb.QueryRow(tx, r.draftSessionSelect(draftID, "r."+kiraschema.ProofRevisions.SessionID, "r."+kiraschema.ProofRevisions.Rev,
			"s."+kiraschema.ProofSessions.Name, "s."+kiraschema.ProofSessions.CurrentRev)).Scan(&out.SessionID, &out.Rev, &out.Name, &cur)
		if errors.Is(err, sql.ErrNoRows) {
			return proofErr(ProofNotFound, "unknown or already closed draftId")
		}
		if err != nil {
			return err
		}
		out.Edit = cur.Valid
		pst := kiraschema.ProofSteps
		cq := r.Dialect.Select("COUNT(*)").From(pst.Table)
		cq.Where(cq.Equal(pst.SessionID, out.SessionID), cq.Equal(pst.Rev, out.Rev))
		if err := sqlb.QueryRow(tx, cq).Scan(&out.Steps); err != nil {
			return err
		}
		if out.Steps == 0 {
			return proofErr(ProofBadState, "the draft has no steps; call proof_add_step first")
		}
		closedAt, err := monotonicClose(tx, r.Dialect, now)
		if err != nil {
			return err
		}
		pr, ps := kiraschema.ProofRevisions, kiraschema.ProofSessions
		revState := "current"
		if out.Edit {
			revState = "pending"
		}
		rev := r.Dialect.Update(pr.Table)
		rev.Set(rev.Assign(pr.State, revState), pr.DraftID+" = NULL", rev.Assign(pr.ClosedAt, closedAt)).
			Where(rev.Equal(pr.SessionID, out.SessionID), rev.Equal(pr.Rev, out.Rev))
		if _, err := sqlb.Exec(tx, rev); err != nil {
			return err
		}
		sess := r.Dialect.Update(ps.Table)
		if out.Edit {
			sess.Set(sess.Assign(ps.PendingRev, out.Rev), ps.SeenAt+" = NULL", sess.Assign(ps.UpdatedAt, closedAt))
		} else {
			sess.Set(sess.Assign(ps.State, "closed"), sess.Assign(ps.CurrentRev, out.Rev), sess.Assign(ps.ClosedAt, closedAt),
				sess.Assign(ps.UpdatedAt, closedAt))
		}
		sess.Where(sess.Equal(ps.ID, out.SessionID))
		if _, err := sqlb.Exec(tx, sess); err != nil {
			return err
		}
		if out.Edit {
			return nil
		}
		out.Pruned, err = pruneClosed(tx, r.Dialect)
		return err
	})
	return out, err
}

// pruneClosed deletes the unattached closed sessions beyond the newest ProofKeepClosed and returns
// their ids. A session linked to a task is never pruned.
func pruneClosed(tx *sql.Tx, d sqlb.Dialect) ([]string, error) {
	ps, tl := kiraschema.ProofSessions, kiraschema.ProofTaskLinks
	linked := d.Select(tl.SessionID).From(tl.Table)
	sb := d.Select(ps.ID).From(ps.Table)
	sb.Where(sb.Equal(ps.State, "closed"), sb.NotIn(ps.ID, linked)).OrderBy(ps.ClosedAt+" DESC", sqlb.RowID+" DESC")
	sqlb.OffsetOnly(d, sb, ProofKeepClosed)
	rows, err := sqlb.Query(tx, sb)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		del := d.DeleteFrom(ps.Table)
		del.Where(del.Equal(ps.ID, id))
		if _, err := sqlb.Exec(tx, del); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// monotonicClose returns now (unix ms) strictly after every closed_at, so "newest closed" is a
// total order even when two closes land in one millisecond.
func monotonicClose(tx *sql.Tx, d sqlb.Dialect, now time.Time) (int64, error) {
	var latest sql.NullInt64
	ps := kiraschema.ProofSessions
	sb := d.Select("MAX(" + ps.ClosedAt + ")").From(ps.Table)
	sb.Where(sb.Equal(ps.State, "closed"))
	if err := sqlb.QueryRow(tx, sb).Scan(&latest); err != nil {
		return 0, err
	}
	ts := now.UnixMilli()
	if latest.Valid && ts <= latest.Int64 {
		ts = latest.Int64 + 1
	}
	return ts, nil
}

// ProofSummary is one closed session in the history list.
type ProofSummary struct {
	ID, Name      string
	ClosedAt      int64
	Steps         int
	Pending, Seen bool
}

// Sessions lists closed sessions, newest first.
func (r *ProofRepo) Sessions() ([]ProofSummary, error) {
	ps := kiraschema.ProofSessions
	sb := r.proofSummarySelect()
	sb.Where(sb.Equal("s."+ps.State, "closed")).OrderBy("s."+ps.ClosedAt+" DESC", "s."+sqlb.RowID+" DESC")
	rows, err := sqlb.Query(r.DB, sb)
	return queryAll(rows, err, func(rows *sql.Rows) (ProofSummary, error) {
		var p ProofSummary
		return p, rows.Scan(&p.ID, &p.Name, &p.ClosedAt, &p.Pending, &p.Seen, &p.Steps)
	})
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
	sb := r.Dialect.Select(c.Ordinal, c.Description, c.Query, c.Language, c.Source, c.View, c.PayloadBytes, c.Diff, c.OriginOrdinal).From(c.Table)
	sb.Where(sb.Equal(c.SessionID, id), sb.Equal(c.Rev, rev)).OrderBy(c.Ordinal)
	rows, err := sqlb.Query(r.DB, sb)
	return queryAll(rows, err, func(rows *sql.Rows) (ProofStepRow, error) {
		s := ProofStepRow{SessionID: id, Rev: rev}
		var origin sql.NullInt64
		err := rows.Scan(&s.Ordinal, &s.Description, &s.Query, &s.Language, &s.Source, &s.View, &s.PayloadBytes, &s.Diff, &origin)
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
		"st."+c.PayloadBytes, "st."+c.Diff, "st."+c.OriginOrdinal)
	err := sqlb.QueryRow(r.DB, sb).
		Scan(&s.Description, &s.Query, &s.Language, &s.Source, &s.View, &s.Payload, &s.PayloadBytes, &s.Diff, &origin)
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
	return r.tx(func(tx *sql.Tx) error {
		cur, pend, err := revs(tx, r.Dialect, id)
		if err != nil {
			return err
		}
		ts := now.UnixMilli()
		vs, st, pr, ps := kiraschema.ProofViewState, kiraschema.ProofSteps, kiraschema.ProofRevisions, kiraschema.ProofSessions
		sel := r.Dialect.Select("n."+st.SessionID, "n."+st.Rev, "n."+st.Ordinal, "v."+vs.State, "v."+vs.UpdatedAt).From(st.Table + " n")
		sel.Join(vs.Table+" v", "v."+vs.SessionID+" = n."+st.SessionID, "v."+vs.Rev+" = "+sel.Var(cur), "v."+vs.Ordinal+" = n."+st.OriginOrdinal)
		sel.Where(sel.Equal("n."+st.SessionID, id), sel.Equal("n."+st.Rev, pend), sel.Equal("n."+st.Diff, "unchanged"))
		copyView := r.Dialect.InsertInto(vs.Table).Cols(vs.SessionID, vs.Rev, vs.Ordinal, vs.State, vs.UpdatedAt)
		copyView.SQL(copyView.Var(sel))
		if _, err := sqlb.Exec(tx, copyView); err != nil {
			return err
		}
		dropOld := r.Dialect.DeleteFrom(pr.Table)
		dropOld.Where(dropOld.Equal(pr.SessionID, id), dropOld.Equal(pr.Rev, cur))
		if _, err := sqlb.Exec(tx, dropOld); err != nil {
			return err
		}
		promote := r.Dialect.Update(pr.Table)
		promote.Set(promote.Assign(pr.State, "current")).Where(promote.Equal(pr.SessionID, id), promote.Equal(pr.Rev, pend))
		if _, err := sqlb.Exec(tx, promote); err != nil {
			return err
		}
		sess := r.Dialect.Update(ps.Table)
		sess.Set(sess.Assign(ps.CurrentRev, pend), ps.PendingRev+" = NULL", sess.Assign(ps.UpdatedAt, ts)).Where(sess.Equal(ps.ID, id))
		_, err = sqlb.Exec(tx, sess)
		return err
	})
}

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

func revs(tx *sql.Tx, d sqlb.Dialect, id string) (cur, pend int, err error) {
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
}

// ProofTaskSession is a closed session attached to a task, with its current revision's steps
// (no payload).
type ProofTaskSession struct {
	Session  ProofSessionRow
	Link     ProofTaskLink
	LinkedAt int64
	Steps    []ProofStepRow
}

// LinkTask attaches a closed session, found by id or (byKey) by name key, to a task. Linking to the
// same task again changes nothing; a session belongs to one task.
func (r *ProofRepo) LinkTask(idOrKey string, byKey bool, l ProofTaskLink, now time.Time) (ProofSessionRow, error) {
	var out ProofSessionRow
	err := r.tx(func(tx *sql.Tx) error {
		ps := kiraschema.ProofSessions
		col := ps.ID
		if byKey {
			col = ps.NameKey
		}
		sb := r.Dialect.Select(proofSessionCols...).From(ps.Table)
		sb.Where(sb.Equal(col, idOrKey))
		s, err := scanProofSession(sqlb.QueryRow(tx, sb))
		if errors.Is(err, sql.ErrNoRows) {
			return proofErr(ProofNotFound, "no proof session with that name; call proof_list_sessions")
		}
		if err != nil {
			return err
		}
		if s.State != "closed" {
			return proofErr(ProofBadState, "close it with proof_close_session first")
		}
		out = s
		var taskID string
		tl := kiraschema.ProofTaskLinks
		lq := r.Dialect.Select(tl.TaskID).From(tl.Table)
		lq.Where(lq.Equal(tl.SessionID, s.ID))
		err = sqlb.QueryRow(tx, lq).Scan(&taskID)
		switch {
		case err == nil && taskID == l.TaskID:
			return nil
		case err == nil:
			return proofErr(ProofBadState, "proof session %q belongs to another task", s.Name)
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		ins := r.Dialect.InsertInto(tl.Table).Cols(tl.SessionID, tl.TaskID, tl.Source, tl.KiraSessionID, tl.StepID, tl.StepName, tl.LinkedAt).
			Values(s.ID, l.TaskID, l.Source, l.KiraSessionID, l.StepID, l.StepName, now.UnixMilli())
		_, err = sqlb.Exec(tx, ins)
		return err
	})
	return out, err
}

// UnlinkTask detaches a session from a task.
func (r *ProofRepo) UnlinkTask(id, taskID string) error {
	tl := kiraschema.ProofTaskLinks
	del := r.Dialect.DeleteFrom(tl.Table)
	del.Where(del.Equal(tl.SessionID, id), del.Equal(tl.TaskID, taskID))
	res, err := sqlb.Exec(r.DB, del)
	if err != nil {
		return fmt.Errorf("repos/proof: unlink: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return proofErr(ProofNotFound, "no such proof session on this task")
	}
	return nil
}

// TaskSessions lists the sessions attached to a task, newest closed first, each with its current
// revision's steps. Two queries, grouped in Go.
func (r *ProofRepo) TaskSessions(taskID string) ([]ProofTaskSession, error) {
	ps, tl, st := kiraschema.ProofSessions, kiraschema.ProofTaskLinks, kiraschema.ProofSteps
	sb := r.Dialect.Select(append(proofAlias("s", proofSessionCols...),
		proofAlias("l", tl.Source, tl.KiraSessionID, tl.StepID, tl.StepName, tl.LinkedAt)...)...).From(tl.Table + " l")
	sb.Join(ps.Table+" s", "s."+ps.ID+" = l."+tl.SessionID)
	sb.Where(sb.Equal("l."+tl.TaskID, taskID), sb.Equal("s."+ps.State, "closed")).
		OrderBy("s."+ps.ClosedAt+" DESC", "s."+sqlb.RowID+" DESC")
	rows, err := sqlb.Query(r.DB, sb)
	list, err := queryAll(rows, err, func(rows *sql.Rows) (ProofTaskSession, error) {
		var t ProofTaskSession
		var cur, pend, closed, seen sql.NullInt64
		s := &t.Session
		err := rows.Scan(&s.ID, &s.Name, &s.NameKey, &s.State, &cur, &pend, &s.Summary, &s.CreatedAt, &closed, &seen, &s.UpdatedAt,
			&t.Link.Source, &t.Link.KiraSessionID, &t.Link.StepID, &t.Link.StepName, &t.LinkedAt)
		s.CurrentRev, s.PendingRev, s.ClosedAt, s.SeenAt = nullInt(cur), nullInt(pend), nullInt64(closed), nullInt64(seen)
		t.Link.TaskID = taskID
		return t, err
	})
	if err != nil || len(list) == 0 {
		return list, err
	}
	sq := r.Dialect.Select(proofAlias("st", st.SessionID, st.Rev, st.Ordinal, st.Description, st.Query, st.Language, st.Source,
		st.View, st.PayloadBytes, st.Diff, st.OriginOrdinal)...).From(tl.Table + " l")
	sq.Join(ps.Table+" s", "s."+ps.ID+" = l."+tl.SessionID)
	sq.Join(st.Table+" st", "st."+st.SessionID+" = s."+ps.ID, "st."+st.Rev+" = s."+ps.CurrentRev)
	sq.Where(sq.Equal("l."+tl.TaskID, taskID), sq.Equal("s."+ps.State, "closed")).OrderBy("st."+st.SessionID, "st."+st.Ordinal)
	srows, err := sqlb.Query(r.DB, sq)
	steps, err := queryAll(srows, err, func(rows *sql.Rows) (ProofStepRow, error) {
		var st ProofStepRow
		var origin sql.NullInt64
		err := rows.Scan(&st.SessionID, &st.Rev, &st.Ordinal, &st.Description, &st.Query, &st.Language, &st.Source, &st.View, &st.PayloadBytes, &st.Diff, &origin)
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

// UnattachedSessions lists closed sessions with no task link, newest first.
func (r *ProofRepo) UnattachedSessions() ([]ProofSummary, error) {
	ps, tl := kiraschema.ProofSessions, kiraschema.ProofTaskLinks
	sb := r.proofSummarySelect()
	linked := r.Dialect.Select(tl.SessionID).From(tl.Table)
	sb.Where(sb.Equal("s."+ps.State, "closed"), sb.NotIn("s."+ps.ID, linked)).
		OrderBy("s."+ps.ClosedAt+" DESC", "s."+sqlb.RowID+" DESC")
	rows, err := sqlb.Query(r.DB, sb)
	return queryAll(rows, err, func(rows *sql.Rows) (ProofSummary, error) {
		var p ProofSummary
		return p, rows.Scan(&p.ID, &p.Name, &p.ClosedAt, &p.Pending, &p.Seen, &p.Steps)
	})
}

// draftSessionSelect selects cols (aliases r = proof_revisions, s = proof_sessions) of the open draft draftID.
func (r *ProofRepo) draftSessionSelect(draftID string, cols ...string) *sqlbuilder.SelectBuilder {
	pr, ps := kiraschema.ProofRevisions, kiraschema.ProofSessions
	sb := r.Dialect.Select(cols...).From(pr.Table + " r")
	sb.Join(ps.Table+" s", "s."+ps.ID+" = r."+pr.SessionID)
	sb.Where(sb.Equal("r."+pr.DraftID, draftID), sb.Equal("r."+pr.State, "draft"))
	return sb
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
