package repos

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// ProofCache is the SQLite cache of proof folders (P643 Part 4): the proof unit writes it inside the
// transaction of its files, through the Querier the file index hands out. The proof_* rows are a cache
// of proofs/<slug>/; the files stay the truth.
type ProofCache struct{ Dialect sqlb.Dialect }

// ProofUnit is a proof session with its task link.
type ProofUnit struct {
	Session ProofSessionRow
	TaskID  string
	Slug    string
}

// ByTaskSlug returns the proof of a task folder; nil when none.
func (c ProofCache) ByTaskSlug(q sqlb.Querier, taskID, slug string) (*ProofUnit, error) {
	ps, tl := kiraschema.ProofSessions, kiraschema.ProofTaskLinks
	sb := c.Dialect.Select(proofAlias("s", proofSessionCols...)...).From(tl.Table + " l")
	sb.Join(ps.Table+" s", "s."+ps.ID+" = l."+tl.SessionID)
	sb.Where(sb.Equal("l."+tl.TaskID, taskID), sb.Equal("l."+tl.Slug, slug))
	s, err := scanProofSession(sqlb.QueryRow(q, sb))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ProofUnit{Session: s, TaskID: taskID, Slug: slug}, nil
}

// SlugsOfTask lists the slugs of a task's proofs.
func (c ProofCache) SlugsOfTask(q sqlb.Querier, taskID string) ([]string, error) {
	tl := kiraschema.ProofTaskLinks
	sb := c.Dialect.Select(tl.Slug).From(tl.Table)
	sb.Where(sb.Equal(tl.TaskID, taskID), sb.NotEqual(tl.Slug, "")).OrderBy(tl.Slug)
	rows, err := sqlb.Query(q, sb)
	return queryAll(rows, err, func(rows *sql.Rows) (string, error) {
		var s string
		return s, rows.Scan(&s)
	})
}

// Steps lists a revision's steps with payload, in ordinal order.
func (c ProofCache) Steps(q sqlb.Querier, sessionID string, rev int) ([]ProofStepRow, error) {
	st := kiraschema.ProofSteps
	sb := c.Dialect.Select(st.Ordinal, st.StepKey, st.Description, st.Query, st.Language, st.Source, st.View, st.Payload).From(st.Table)
	sb.Where(sb.Equal(st.SessionID, sessionID), sb.Equal(st.Rev, rev)).OrderBy(st.Ordinal)
	rows, err := sqlb.Query(q, sb)
	return queryAll(rows, err, func(rows *sql.Rows) (ProofStepRow, error) {
		s := ProofStepRow{SessionID: sessionID, Rev: rev}
		err := rows.Scan(&s.Ordinal, &s.StepKey, &s.Description, &s.Query, &s.Language, &s.Source, &s.View, &s.Payload)
		s.PayloadBytes = len(s.Payload)
		return s, err
	})
}

// NameTakenBy returns the id of the proof in a task holding the name, "" when free.
func (c ProofCache) NameTakenBy(q sqlb.Querier, taskID, name string) (string, error) {
	ps := kiraschema.ProofSessions
	sb := c.Dialect.Select(ps.ID).From(ps.Table)
	sb.Where(sb.Equal(ps.NameKey, ProofNameKey(taskID, name)))
	var id string
	err := sqlb.QueryRow(q, sb).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// ProofNew is a new proof: session, link and first revision in one write.
type ProofNew struct {
	ID, TaskID, Slug, Name, Summary string
	Steps                           []ProofStepRow
	Now                             int64
}

// Create writes a new closed proof with revision 1 as its current one.
func (c ProofCache) Create(q sqlb.Querier, p ProofNew) error {
	ps, pr, tl := kiraschema.ProofSessions, kiraschema.ProofRevisions, kiraschema.ProofTaskLinks
	ins := c.Dialect.InsertInto(ps.Table).Cols(ps.ID, ps.Name, ps.NameKey, ps.State, ps.CurrentRev, ps.Summary, ps.CreatedAt, ps.ClosedAt, ps.UpdatedAt).
		Values(p.ID, p.Name, ProofNameKey(p.TaskID, p.Name), "closed", 1, p.Summary, p.Now, p.Now, p.Now)
	if _, err := sqlb.Exec(q, ins); err != nil {
		return fmt.Errorf("repos/proof: create: %w", err)
	}
	rev := c.Dialect.InsertInto(pr.Table).Cols(pr.SessionID, pr.Rev, pr.State, pr.CreatedAt, pr.ClosedAt).Values(p.ID, 1, "current", p.Now, p.Now)
	if _, err := sqlb.Exec(q, rev); err != nil {
		return fmt.Errorf("repos/proof: create revision: %w", err)
	}
	if err := c.insertSteps(q, p.ID, 1, p.Steps, nil); err != nil {
		return err
	}
	lk := c.Dialect.InsertInto(tl.Table).Cols(tl.SessionID, tl.TaskID, tl.Source, tl.LinkedAt, tl.Slug).Values(p.ID, p.TaskID, "agent", p.Now, p.Slug)
	if _, err := sqlb.Exec(q, lk); err != nil {
		return fmt.Errorf("repos/proof: link: %w", err)
	}
	return nil
}

// ProofEdit proposes a new revision of an existing proof.
type ProofEdit struct {
	SessionID, Name, Summary, Reason string
	Steps                            []ProofStepRow
	// Origin maps a new step's index to the ordinal of the unchanged or changed step it descends from; nil when added.
	Origin map[int]int
	Now    int64
}

// Propose stores a pending revision, replacing any earlier pending one. Name and summary ride on the revision
// until AcceptEdit copies them to the session.
func (c ProofCache) Propose(q sqlb.Querier, e ProofEdit) error {
	if err := c.DropPending(q, e.SessionID, e.Now); err != nil {
		return err
	}
	ps, pr := kiraschema.ProofSessions, kiraschema.ProofRevisions
	var cur int
	sel := c.Dialect.Select(ps.CurrentRev).From(ps.Table)
	sel.Where(sel.Equal(ps.ID, e.SessionID))
	if err := sqlb.QueryRow(q, sel).Scan(&cur); err != nil {
		return fmt.Errorf("repos/proof: propose: %w", err)
	}
	rev := cur + 1
	ins := c.Dialect.InsertInto(pr.Table).Cols(pr.SessionID, pr.Rev, pr.State, pr.CreatedAt, pr.ClosedAt, pr.Name, pr.Summary, pr.Reason).
		Values(e.SessionID, rev, "pending", e.Now, e.Now, e.Name, e.Summary, e.Reason)
	if _, err := sqlb.Exec(q, ins); err != nil {
		return fmt.Errorf("repos/proof: propose revision: %w", err)
	}
	if err := c.insertSteps(q, e.SessionID, rev, e.Steps, e.Origin); err != nil {
		return err
	}
	up := c.Dialect.Update(ps.Table)
	up.Set(up.Assign(ps.PendingRev, rev), ps.SeenAt+" = NULL", up.Assign(ps.UpdatedAt, e.Now)).Where(up.Equal(ps.ID, e.SessionID))
	_, err := sqlb.Exec(q, up)
	return err
}

// DropPending deletes the pending revision, if any.
func (c ProofCache) DropPending(q sqlb.Querier, sessionID string, now int64) error {
	ps, pr := kiraschema.ProofSessions, kiraschema.ProofRevisions
	var pend sql.NullInt64
	sel := c.Dialect.Select(ps.PendingRev).From(ps.Table)
	sel.Where(sel.Equal(ps.ID, sessionID))
	if err := sqlb.QueryRow(q, sel).Scan(&pend); err != nil {
		return err
	}
	if !pend.Valid {
		return nil
	}
	del := c.Dialect.DeleteFrom(pr.Table)
	del.Where(del.Equal(pr.SessionID, sessionID), del.Equal(pr.Rev, pend.Int64))
	if _, err := sqlb.Exec(q, del); err != nil {
		return err
	}
	up := c.Dialect.Update(ps.Table)
	up.Set(ps.PendingRev+" = NULL", up.Assign(ps.UpdatedAt, now)).Where(up.Equal(ps.ID, sessionID))
	_, err := sqlb.Exec(q, up)
	return err
}

// Delete removes a proof with everything under it.
func (c ProofCache) Delete(q sqlb.Querier, sessionID string) error {
	ps := kiraschema.ProofSessions
	del := c.Dialect.DeleteFrom(ps.Table)
	del.Where(del.Equal(ps.ID, sessionID))
	_, err := sqlb.Exec(q, del)
	return err
}

func (c ProofCache) insertSteps(q sqlb.Querier, id string, rev int, steps []ProofStepRow, origin map[int]int) error {
	st := kiraschema.ProofSteps
	for i, s := range steps {
		diff := s.Diff
		if diff == "" {
			diff = "added"
		}
		ins := c.Dialect.InsertInto(st.Table).
			Cols(st.SessionID, st.Rev, st.Ordinal, st.StepKey, st.Description, st.Query, st.Language, st.Source, st.View, st.Payload, st.PayloadBytes, st.Diff, st.OriginOrdinal).
			Values(id, rev, i+1, s.StepKey, s.Description, s.Query, s.Language, s.Source, s.View, s.Payload, len(s.Payload), diff, originOf(origin, i))
		if _, err := sqlb.Exec(q, ins); err != nil {
			return fmt.Errorf("repos/proof: step %d: %w", i+1, err)
		}
	}
	return nil
}

func originOf(m map[int]int, i int) any {
	if o, ok := m[i]; ok {
		return o
	}
	return nil
}

// LinkOf returns the task and proof folder of a session; ok is false when it has no folder.
func (r *ProofRepo) LinkOf(sessionID string) (taskID, slug string, ok bool, err error) {
	tl := kiraschema.ProofTaskLinks
	sb := r.Dialect.Select(tl.TaskID, tl.Slug).From(tl.Table)
	sb.Where(sb.Equal(tl.SessionID, sessionID))
	err = sqlb.QueryRow(r.DB, sb).Scan(&taskID, &slug)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	return taskID, slug, err == nil && slug != "", err
}

// SlugOf returns the folder slug of a proof session, "" when unknown.
func (c ProofCache) SlugOf(q sqlb.Querier, sessionID string) (string, error) {
	tl := kiraschema.ProofTaskLinks
	sb := c.Dialect.Select(tl.Slug).From(tl.Table)
	sb.Where(sb.Equal(tl.SessionID, sessionID))
	var slug string
	err := sqlb.QueryRow(q, sb).Scan(&slug)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return slug, err
}

// ByTaskSlug reads the proof of a task folder outside any transaction; nil when none.
func (r *ProofRepo) ByTaskSlug(taskID, slug string) (*ProofUnit, error) {
	return ProofCache{r.Dialect}.ByTaskSlug(r.DB, taskID, slug)
}

// LegacyProof is a closed session attached to a task before proofs moved to files: its link has no slug yet.
type LegacyProof struct {
	Session  ProofSessionRow
	LinkedAt int64
	// Cur and Pend are the steps, with payload, of the current and the pending revision; Pend is nil without one.
	Cur, Pend  []ProofStepRow
	PendReason string
}

// LegacyProofs lists a task's sessions whose link has no folder yet, oldest first.
func (r *ProofRepo) LegacyProofs(taskID string) ([]LegacyProof, error) {
	ps, tl := kiraschema.ProofSessions, kiraschema.ProofTaskLinks
	sb := r.Dialect.Select(append(proofAlias("s", proofSessionCols...), "l."+tl.LinkedAt)...).From(tl.Table + " l")
	sb.Join(ps.Table+" s", "s."+ps.ID+" = l."+tl.SessionID)
	sb.Where(sb.Equal("l."+tl.TaskID, taskID), sb.Equal("l."+tl.Slug, ""), sb.Equal("s."+ps.State, "closed")).
		OrderBy("s."+ps.CreatedAt, "s."+ps.ID)
	rows, err := sqlb.Query(r.DB, sb)
	list, err := queryAll(rows, err, func(rows *sql.Rows) (LegacyProof, error) {
		var p LegacyProof
		var cur, pend, closed, seen sql.NullInt64
		s := &p.Session
		err := rows.Scan(&s.ID, &s.Name, &s.NameKey, &s.State, &cur, &pend, &s.Summary, &s.CreatedAt, &closed, &seen, &s.UpdatedAt, &p.LinkedAt)
		s.CurrentRev, s.PendingRev, s.ClosedAt, s.SeenAt = nullInt(cur), nullInt(pend), nullInt64(closed), nullInt64(seen)
		return p, err
	})
	if err != nil {
		return nil, err
	}
	c := ProofCache{r.Dialect}
	pr := kiraschema.ProofRevisions
	for i := range list {
		s := list[i].Session
		if s.CurrentRev == nil {
			continue
		}
		if list[i].Cur, err = c.Steps(r.DB, s.ID, *s.CurrentRev); err != nil {
			return nil, err
		}
		if s.PendingRev == nil {
			continue
		}
		if list[i].Pend, err = c.Steps(r.DB, s.ID, *s.PendingRev); err != nil {
			return nil, err
		}
		q := r.Dialect.Select(pr.Reason).From(pr.Table)
		q.Where(q.Equal(pr.SessionID, s.ID), q.Equal(pr.Rev, *s.PendingRev))
		if err := sqlb.QueryRow(r.DB, q).Scan(&list[i].PendReason); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return list, nil
}

// TasklessSessions lists the sessions no task holds.
func (r *ProofRepo) TasklessSessions() ([]ProofSessionRow, error) {
	ps, tl := kiraschema.ProofSessions, kiraschema.ProofTaskLinks
	sb := r.Dialect.Select(proofAlias("s", proofSessionCols...)...).From(ps.Table + " s")
	sb.Where("NOT EXISTS (SELECT 1 FROM " + tl.Table + " l WHERE l." + tl.SessionID + " = s." + ps.ID + ")").OrderBy("s." + ps.ID)
	rows, err := sqlb.Query(r.DB, sb)
	return queryAll(rows, err, func(rows *sql.Rows) (ProofSessionRow, error) { return scanProofSession(rows) })
}

// DeleteSession removes a session with everything under it.
func (r *ProofRepo) DeleteSession(id string) error { return ProofCache{r.Dialect}.Delete(r.DB, id) }

// SetSlug gives a legacy link its proof folder.
func (c ProofCache) SetSlug(q sqlb.Querier, sessionID, slug string) error {
	tl := kiraschema.ProofTaskLinks
	up := c.Dialect.Update(tl.Table)
	up.Set(up.Assign(tl.Slug, slug)).Where(up.Equal(tl.SessionID, sessionID))
	_, err := sqlb.Exec(q, up)
	return err
}

// SetStep sets the id and payload of one step; the payload of an image step becomes its metadata.
func (c ProofCache) SetStep(q sqlb.Querier, sessionID string, rev, ordinal int, key string, payload []byte) error {
	st := kiraschema.ProofSteps
	up := c.Dialect.Update(st.Table)
	up.Set(up.Assign(st.StepKey, key), up.Assign(st.Payload, payload), up.Assign(st.PayloadBytes, len(payload))).
		Where(up.Equal(st.SessionID, sessionID), up.Equal(st.Rev, rev), up.Equal(st.Ordinal, ordinal))
	_, err := sqlb.Exec(q, up)
	return err
}

// SetRevisionText sets the name and summary a revision proposes.
func (c ProofCache) SetRevisionText(q sqlb.Querier, sessionID string, rev int, name, summary string) error {
	pr := kiraschema.ProofRevisions
	up := c.Dialect.Update(pr.Table)
	up.Set(up.Assign(pr.Name, name), up.Assign(pr.Summary, summary)).Where(up.Equal(pr.SessionID, sessionID), up.Equal(pr.Rev, rev))
	_, err := sqlb.Exec(q, up)
	return err
}
