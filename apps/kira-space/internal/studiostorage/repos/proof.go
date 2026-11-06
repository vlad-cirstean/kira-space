package repos

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
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
	DB *sql.DB
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

const proofSessionCols = `id, name, name_key, state, current_rev, pending_rev, summary, created_at, closed_at, seen_at, updated_at`

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

func sessionByKey(tx *sql.Tx, nameKey string) (ProofSessionRow, bool, error) {
	s, err := scanProofSession(tx.QueryRow(`SELECT `+proofSessionCols+` FROM proof_sessions WHERE name_key = ?`, nameKey))
	if errors.Is(err, sql.ErrNoRows) {
		return ProofSessionRow{}, false, nil
	}
	return s, err == nil, err
}

func draftCount(tx *sql.Tx) (int, error) {
	var n int
	err := tx.QueryRow(`SELECT COUNT(*) FROM proof_revisions WHERE state = 'draft'`).Scan(&n)
	return n, err
}

// PurgeStaleDrafts drops drafts created before cutoff: never-closed sessions and unfinished edit
// revisions. Returns how many it dropped.
func (r *ProofRepo) PurgeStaleDrafts(cutoff time.Time) (int, error) {
	c := cutoff.UnixMilli()
	n := 0
	err := r.tx(func(tx *sql.Tx) error {
		res, err := tx.Exec(`DELETE FROM proof_sessions WHERE state = 'open' AND created_at < ?`, c)
		if err != nil {
			return err
		}
		a, _ := res.RowsAffected()
		n += int(a)
		res, err = tx.Exec(`DELETE FROM proof_revisions WHERE state = 'draft' AND created_at < ?`, c)
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
		if n, err := draftCount(tx); err != nil {
			return err
		} else if n >= ProofMaxDrafts {
			return proofErr(ProofLimit, "%d proof drafts are already open; close one with proof_close_session first", n)
		}
		if ex, ok, err := sessionByKey(tx, nameKey); err != nil {
			return err
		} else if ok {
			if ex.State == "open" {
				return proofErr(ProofNameTaken, "a proof session named %q is still open; pick another name, or close it first", ex.Name)
			}
			return proofErr(ProofNameTaken, "a proof session named %q already exists (closed %s); pick another name, or call proof_edit_session to change it", ex.Name, time.UnixMilli(*ex.ClosedAt).UTC().Format(time.RFC3339))
		}
		if _, err := tx.Exec(`INSERT INTO proof_sessions (id, name, name_key, state, summary, created_at, updated_at) VALUES (?, ?, ?, 'open', ?, ?, ?)`,
			id, name, nameKey, summary, ts, ts); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO proof_revisions (session_id, rev, draft_id, state, created_at) VALUES (?, 1, ?, 'draft', ?)`, id, draftID, ts)
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
		s, ok, err := sessionByKey(tx, nameKey)
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
		if err := tx.QueryRow(`SELECT COUNT(*) FROM proof_revisions WHERE session_id = ? AND state = 'draft'`, s.ID).Scan(&open); err != nil {
			return err
		}
		if open > 0 {
			return proofErr(ProofBadState, "proof session %q already has an edit draft open", s.Name)
		}
		if n, err := draftCount(tx); err != nil {
			return err
		} else if n >= ProofMaxDrafts {
			return proofErr(ProofLimit, "%d proof drafts are already open; close one with proof_close_session first", n)
		}
		var rev int
		if err := tx.QueryRow(`SELECT COALESCE(MAX(rev), 0) + 1 FROM proof_revisions WHERE session_id = ?`, s.ID).Scan(&rev); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO proof_revisions (session_id, rev, draft_id, reason, state, created_at) VALUES (?, ?, ?, ?, 'draft', ?)`,
			s.ID, rev, draftID, reason, ts); err != nil {
			return err
		}
		rows, err := tx.Query(`SELECT ordinal, description, view FROM proof_steps WHERE session_id = ? AND rev = ? ORDER BY ordinal`, s.ID, *s.CurrentRev)
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
func applyRef(tx *sql.Tx, sid string, rev int, curRev int64, in ProofNewStep, step *ProofStepRow) error {
	ref := in.Keep
	if in.Replaces != nil {
		ref = in.Replaces
	}
	if ref == nil {
		return nil
	}
	if err := tx.QueryRow(`SELECT 1 FROM proof_steps WHERE session_id = ? AND rev = ? AND origin_ordinal = ?`, sid, rev, *ref).Scan(new(int)); err == nil {
		return proofErr(ProofBadRef, "old step %d is already referenced by another step of this draft", *ref)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var old ProofStepRow
	err := tx.QueryRow(`SELECT description, query, language, source, view, payload FROM proof_steps WHERE session_id = ? AND rev = ? AND ordinal = ?`,
		sid, curRev, *ref).Scan(&old.Description, &old.Query, &old.Language, &old.Source, &old.View, &old.Payload)
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
		err := tx.QueryRow(`SELECT r.session_id, r.rev, s.current_rev FROM proof_revisions r JOIN proof_sessions s ON s.id = r.session_id
			WHERE r.draft_id = ? AND r.state = 'draft'`, draftID).Scan(&sid, &rev, &cur)
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
		if err := tx.QueryRow(`SELECT COUNT(*) FROM proof_steps WHERE session_id = ? AND rev = ?`, sid, rev).Scan(&count); err != nil {
			return err
		}
		if count >= ProofMaxSteps {
			return proofErr(ProofLimit, "a proof session holds at most %d steps", ProofMaxSteps)
		}
		step := ProofStepRow{SessionID: sid, Rev: rev, Description: in.Description, Query: in.Query, Language: in.Language,
			Source: in.Source, View: in.View, Payload: in.Payload, Diff: "added"}
		if err := applyRef(tx, sid, rev, cur.Int64, in, &step); err != nil {
			return err
		}
		step.PayloadBytes = len(step.Payload)
		var used int
		if err := tx.QueryRow(`SELECT COALESCE(SUM(payload_bytes), 0) FROM proof_steps WHERE session_id = ?`, sid).Scan(&used); err != nil {
			return err
		}
		if used+step.PayloadBytes > ProofMaxSessionBytes {
			return proofErr(ProofLimit, "the session would exceed its %d MiB data limit", ProofMaxSessionBytes>>20)
		}
		if err := tx.QueryRow(`SELECT COALESCE(MAX(ordinal), 0) + 1 FROM proof_steps WHERE session_id = ? AND rev = ?`, sid, rev).Scan(&ordinal); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO proof_steps (session_id, rev, ordinal, description, query, language, source, view, payload, payload_bytes, diff, origin_ordinal)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sid, rev, ordinal, step.Description, step.Query, step.Language, step.Source, step.View, step.Payload, step.PayloadBytes, step.Diff, step.OriginOrdinal)
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
		err := tx.QueryRow(`SELECT r.session_id, r.rev, s.name, s.current_rev FROM proof_revisions r JOIN proof_sessions s ON s.id = r.session_id
			WHERE r.draft_id = ? AND r.state = 'draft'`, draftID).Scan(&out.SessionID, &out.Rev, &out.Name, &cur)
		if errors.Is(err, sql.ErrNoRows) {
			return proofErr(ProofNotFound, "unknown or already closed draftId")
		}
		if err != nil {
			return err
		}
		out.Edit = cur.Valid
		if err := tx.QueryRow(`SELECT COUNT(*) FROM proof_steps WHERE session_id = ? AND rev = ?`, out.SessionID, out.Rev).Scan(&out.Steps); err != nil {
			return err
		}
		if out.Steps == 0 {
			return proofErr(ProofBadState, "the draft has no steps; call proof_add_step first")
		}
		closedAt, err := monotonicClose(tx, now)
		if err != nil {
			return err
		}
		if out.Edit {
			if _, err := tx.Exec(`UPDATE proof_revisions SET state = 'pending', draft_id = NULL, closed_at = ? WHERE session_id = ? AND rev = ?`, closedAt, out.SessionID, out.Rev); err != nil {
				return err
			}
			_, err = tx.Exec(`UPDATE proof_sessions SET pending_rev = ?, seen_at = NULL, updated_at = ? WHERE id = ?`, out.Rev, closedAt, out.SessionID)
			return err
		}
		if _, err := tx.Exec(`UPDATE proof_revisions SET state = 'current', draft_id = NULL, closed_at = ? WHERE session_id = ? AND rev = ?`, closedAt, out.SessionID, out.Rev); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE proof_sessions SET state = 'closed', current_rev = ?, closed_at = ?, updated_at = ? WHERE id = ?`, out.Rev, closedAt, closedAt, out.SessionID); err != nil {
			return err
		}
		out.Pruned, err = pruneClosed(tx)
		return err
	})
	return out, err
}

// pruneClosed deletes the closed sessions beyond the newest ProofKeepClosed and returns their ids.
func pruneClosed(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query(`SELECT id FROM proof_sessions WHERE state = 'closed' ORDER BY closed_at DESC, rowid DESC LIMIT -1 OFFSET ?`, ProofKeepClosed)
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
		if _, err := tx.Exec(`DELETE FROM proof_sessions WHERE id = ?`, id); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// monotonicClose returns now (unix ms) strictly after every closed_at, so "newest closed" is a
// total order even when two closes land in one millisecond.
func monotonicClose(tx *sql.Tx, now time.Time) (int64, error) {
	var latest sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(closed_at) FROM proof_sessions WHERE state = 'closed'`).Scan(&latest); err != nil {
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
	rows, err := r.DB.Query(`SELECT s.id, s.name, s.closed_at, s.pending_rev IS NOT NULL, s.seen_at IS NOT NULL,
		(SELECT COUNT(*) FROM proof_steps st WHERE st.session_id = s.id AND st.rev = s.current_rev)
		FROM proof_sessions s WHERE s.state = 'closed' ORDER BY s.closed_at DESC, s.rowid DESC`)
	return queryAll(rows, err, func(rows *sql.Rows) (ProofSummary, error) {
		var p ProofSummary
		return p, rows.Scan(&p.ID, &p.Name, &p.ClosedAt, &p.Pending, &p.Seen, &p.Steps)
	})
}

// Session returns one session; ok is false when absent or still open.
func (r *ProofRepo) Session(id string) (ProofSessionRow, bool, error) {
	s, err := scanProofSession(r.DB.QueryRow(`SELECT `+proofSessionCols+` FROM proof_sessions WHERE id = ? AND state = 'closed'`, id))
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
	err := r.DB.QueryRow(`SELECT rev, draft_id, reason, state, created_at, closed_at FROM proof_revisions WHERE session_id = ? AND rev = ? AND state != 'draft'`, id, rev).
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
	rows, err := r.DB.Query(`SELECT ordinal, description, query, language, source, view, payload_bytes, diff, origin_ordinal
		FROM proof_steps WHERE session_id = ? AND rev = ? ORDER BY ordinal`, id, rev)
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
	err := r.DB.QueryRow(`SELECT st.description, st.query, st.language, st.source, st.view, st.payload, st.payload_bytes, st.diff, st.origin_ordinal
		FROM proof_steps st JOIN proof_revisions rv ON rv.session_id = st.session_id AND rv.rev = st.rev
		WHERE st.session_id = ? AND st.rev = ? AND st.ordinal = ? AND rv.state != 'draft'`, id, rev, ordinal).
		Scan(&s.Description, &s.Query, &s.Language, &s.Source, &s.View, &s.Payload, &s.PayloadBytes, &s.Diff, &origin)
	if errors.Is(err, sql.ErrNoRows) {
		return ProofStepRow{}, false, nil
	}
	s.OriginOrdinal = nullInt(origin)
	return s, err == nil, err
}

// ViewStates lists the saved view state of every step of one revision.
func (r *ProofRepo) ViewStates(id string, rev int) ([]ProofViewStateRow, error) {
	rows, err := r.DB.Query(`SELECT ordinal, state FROM proof_view_state WHERE session_id = ? AND rev = ? ORDER BY ordinal`, id, rev)
	return queryAll(rows, err, func(rows *sql.Rows) (ProofViewStateRow, error) {
		var v ProofViewStateRow
		return v, rows.Scan(&v.Ordinal, &v.State)
	})
}

// SetViewState upserts one step's view state. The step must belong to a closed or pending revision.
func (r *ProofRepo) SetViewState(id string, rev, ordinal int, state string, now time.Time) error {
	return r.tx(func(tx *sql.Tx) error {
		var one int
		err := tx.QueryRow(`SELECT 1 FROM proof_steps st JOIN proof_revisions rv ON rv.session_id = st.session_id AND rv.rev = st.rev
			WHERE st.session_id = ? AND st.rev = ? AND st.ordinal = ? AND rv.state != 'draft'`, id, rev, ordinal).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return proofErr(ProofNotFound, "no such proof step")
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO proof_view_state (session_id, rev, ordinal, state, updated_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (session_id, rev, ordinal) DO UPDATE SET state = excluded.state, updated_at = excluded.updated_at`,
			id, rev, ordinal, state, now.UnixMilli())
		return err
	})
}

// AcceptEdit makes the pending revision current, copies the view state of unchanged steps onto
// it and deletes the old revision.
func (r *ProofRepo) AcceptEdit(id string, now time.Time) error {
	return r.tx(func(tx *sql.Tx) error {
		cur, pend, err := revs(tx, id)
		if err != nil {
			return err
		}
		ts := now.UnixMilli()
		if _, err := tx.Exec(`INSERT INTO proof_view_state (session_id, rev, ordinal, state, updated_at)
			SELECT n.session_id, n.rev, n.ordinal, v.state, v.updated_at FROM proof_steps n
			JOIN proof_view_state v ON v.session_id = n.session_id AND v.rev = ? AND v.ordinal = n.origin_ordinal
			WHERE n.session_id = ? AND n.rev = ? AND n.diff = 'unchanged'`, cur, id, pend); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM proof_revisions WHERE session_id = ? AND rev = ?`, id, cur); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE proof_revisions SET state = 'current' WHERE session_id = ? AND rev = ?`, id, pend); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE proof_sessions SET current_rev = ?, pending_rev = NULL, updated_at = ? WHERE id = ?`, pend, ts, id)
		return err
	})
}

// RejectEdit drops the pending revision.
func (r *ProofRepo) RejectEdit(id string, now time.Time) error {
	return r.tx(func(tx *sql.Tx) error {
		_, pend, err := revs(tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM proof_revisions WHERE session_id = ? AND rev = ?`, id, pend); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE proof_sessions SET pending_rev = NULL, updated_at = ? WHERE id = ?`, now.UnixMilli(), id)
		return err
	})
}

func revs(tx *sql.Tx, id string) (cur, pend int, err error) {
	var c, p sql.NullInt64
	err = tx.QueryRow(`SELECT current_rev, pending_rev FROM proof_sessions WHERE id = ? AND state = 'closed'`, id).Scan(&c, &p)
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
	res, err := r.DB.Exec(`DELETE FROM proof_sessions WHERE id = ? AND state = 'closed'`, id)
	if err != nil {
		return false, fmt.Errorf("repos/proof: delete: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// MarkSeen stamps a closed session as seen by the user.
func (r *ProofRepo) MarkSeen(id string, now time.Time) error {
	ts := now.UnixMilli()
	_, err := r.DB.Exec(`UPDATE proof_sessions SET seen_at = ? WHERE id = ? AND state = 'closed'`, ts, id)
	if err != nil {
		return fmt.Errorf("repos/proof: mark seen: %w", err)
	}
	return nil
}

// Unreviewed lists closed sessions never opened by the user or holding a pending edit, oldest
// first: the ones whose prompt a restart re-raises.
func (r *ProofRepo) Unreviewed() ([]ProofSummary, error) {
	rows, err := r.DB.Query(`SELECT s.id, s.name, s.closed_at, s.pending_rev IS NOT NULL, s.seen_at IS NOT NULL,
		(SELECT COUNT(*) FROM proof_steps st WHERE st.session_id = s.id AND st.rev = s.current_rev)
		FROM proof_sessions s WHERE s.state = 'closed' AND (s.seen_at IS NULL OR s.pending_rev IS NOT NULL) ORDER BY s.closed_at, s.rowid`)
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
