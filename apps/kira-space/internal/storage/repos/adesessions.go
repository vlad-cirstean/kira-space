package repos

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/huandu/go-sqlbuilder"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// adeSessionsSelectColumns is in scan order.
var adeSessionsSelectColumns = []string{
	kiraschema.AdeSessions.ID, kiraschema.AdeSessions.AgentSessionID, kiraschema.AdeSessions.Cwd, kiraschema.AdeSessions.State,
	kiraschema.AdeSessions.TerminalID, kiraschema.AdeSessions.StartedAt, kiraschema.AdeSessions.LastActiveAt,
	kiraschema.AdeSessions.Mode, kiraschema.AdeSessions.TaskID, kiraschema.AdeSessions.BranchID, kiraschema.AdeSessions.StepID,
	kiraschema.AdeSessions.RunID, kiraschema.AdeSessions.Resumes, kiraschema.AdeSessions.Purpose, kiraschema.AdeSessions.Provider,
}

func (r *AdeSessionsRepo) selectSessions() *sqlbuilder.SelectBuilder {
	return r.Dialect.Select(adeSessionsSelectColumns...).From(kiraschema.AdeSessions.Table)
}

// AdeSessionsRepo reads and writes `ade_sessions` — the history of every task Claude Code session
// (interactive or headless) the app has spawned or resumed.
type AdeSessionsRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

func scanAdeSessionRow(row rowScanner) (model.AdeSession, error) {
	var rec model.AdeSession
	var terminalID sql.NullString
	if err := row.Scan(
		&rec.ID, &rec.AgentSessionID, &rec.Cwd,
		&rec.State, &terminalID, &rec.StartedAt, &rec.LastActiveAt,
		&rec.Mode, &rec.TaskID, &rec.BranchID, &rec.StepID, &rec.RunID, &rec.Resumes, &rec.Purpose, &rec.Provider,
	); err != nil {
		return model.AdeSession{}, err
	}
	rec.TerminalID = terminalID.String
	return rec, nil
}

// Get reads one row by id, (nil, nil) when not found — the same "no error on a miss" discipline
// CodeReposRepo.Get already follows.
func (r *AdeSessionsRepo) Get(id string) (*model.AdeSession, error) {
	sb := r.selectSessions()
	sb.Where(sb.Equal(kiraschema.AdeSessions.ID, id))
	rec, err := adeOne(r.DB, sb, func(row *sql.Row) (*model.AdeSession, error) {
		s, err := scanAdeSessionRow(row)
		if err != nil {
			return nil, err
		}
		return &s, nil
	})
	if err != nil {
		return nil, fmt.Errorf("repos: get ade session %s: %w", id, err)
	}
	return rec, nil
}

// ListTask returns every row, running first then most recently active.
func (r *AdeSessionsRepo) ListTask() ([]model.AdeSession, error) {
	c := kiraschema.AdeSessions
	sb := r.selectSessions()
	sb.OrderBy(c.State+" = "+sb.Var(model.AdeSessionStateRunning)+" DESC", c.LastActiveAt+" DESC", c.ID)
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeSession, bool, error) {
		rec, err := scanAdeSessionRow(rows)
		return rec, true, err
	})
}

// HasTask reports whether any session row, running or stopped, belongs to the task.
func (r *AdeSessionsRepo) HasTask(taskID string) (bool, error) {
	c := kiraschema.AdeSessions
	n, err := adeCount(r.DB, r.Dialect, c.Table, func(sb *sqlbuilder.SelectBuilder) []string { return []string{sb.Equal(c.TaskID, taskID)} })
	if err != nil {
		return false, fmt.Errorf("repos: count ade sessions of task %s: %w", taskID, err)
	}
	return n > 0, nil
}

// CountRunningHeadless counts headless task sessions still running.
func (r *AdeSessionsRepo) CountRunningHeadless() (int, error) {
	c := kiraschema.AdeSessions
	n, err := adeCount(r.DB, r.Dialect, c.Table, func(sb *sqlbuilder.SelectBuilder) []string {
		return []string{sb.Equal(c.State, model.AdeSessionStateRunning), sb.Equal(c.Mode, model.AdeSessionModeHeadless)}
	})
	if err != nil {
		return 0, fmt.Errorf("repos: count running headless ade sessions: %w", err)
	}
	return n, nil
}

// ListRunningTUI returns the task rows whose interactive terminal is open.
func (r *AdeSessionsRepo) ListRunningTUI() ([]model.AdeSession, error) {
	c := kiraschema.AdeSessions
	sb := r.selectSessions()
	sb.Where(sb.NotEqual(c.TaskID, ""), sb.Equal(c.Mode, model.AdeSessionModeTUI), sb.Equal(c.State, model.AdeSessionStateRunning))
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeSession, bool, error) {
		rec, err := scanAdeSessionRow(rows)
		return rec, true, err
	})
}

// ListRunning returns the task's running sessions of either mode, served by the task_id index.
func (r *AdeSessionsRepo) ListRunning(taskID string) ([]model.AdeSession, error) {
	c := kiraschema.AdeSessions
	sb := r.selectSessions()
	sb.Where(sb.Equal(c.TaskID, taskID), sb.Equal(c.State, model.AdeSessionStateRunning))
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeSession, bool, error) {
		rec, err := scanAdeSessionRow(rows)
		return rec, true, err
	})
}

func (r *AdeSessionsRepo) insertSession(rec model.AdeSession) *sqlbuilder.InsertBuilder {
	c := kiraschema.AdeSessions
	return r.Dialect.InsertInto(c.Table).Cols(c.ID, c.Mode, c.TaskID, c.BranchID, c.StepID, c.RunID, c.Resumes, c.Purpose,
		c.AgentSessionID, c.Cwd, c.State, c.TerminalID, c.StartedAt, c.LastActiveAt, c.Provider).
		Values(rec.ID, rec.Mode, rec.TaskID, rec.BranchID, rec.StepID, rec.RunID, rec.Resumes, rec.Purpose,
			rec.AgentSessionID, rec.Cwd, rec.State, nullableString(rec.TerminalID), rec.StartedAt, rec.LastActiveAt, rec.Provider)
}

// InsertHeadless writes a headless task session row (no terminal).
func (r *AdeSessionsRepo) InsertHeadless(rec model.AdeSession) error {
	if rec.TaskID == "" {
		return fmt.Errorf("repos: insert headless ade session %s: taskId is required", rec.ID)
	}
	if err := rec.Validate(); err != nil {
		return fmt.Errorf("repos: %w", err)
	}
	if _, err := sqlb.Exec(r.DB, r.insertSession(rec)); err != nil {
		return fmt.Errorf("repos: insert headless ade session %s: %w", rec.ID, err)
	}
	return nil
}

// InsertTUI writes a task session row for an interactive Claude Code terminal.
func (r *AdeSessionsRepo) InsertTUI(rec model.AdeSession) error {
	if rec.TaskID == "" || rec.Mode != model.AdeSessionModeTUI {
		return fmt.Errorf("repos: insert tui ade session %s: a tui task row needs taskId and mode tui", rec.ID)
	}
	if err := rec.Validate(); err != nil {
		return fmt.Errorf("repos: %w", err)
	}
	if _, err := sqlb.Exec(r.DB, r.insertSession(rec)); err != nil {
		return fmt.Errorf("repos: insert tui ade session %s: %w", rec.ID, err)
	}
	return nil
}

// ClearPurpose turns a review agent row into a plain stopped session (its cwd is gone, a fresh
// review agent replaces it).
func (r *AdeSessionsRepo) ClearPurpose(id string) error {
	c := kiraschema.AdeSessions
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.Purpose, "")).Where(ub.Equal(c.ID, id))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("repos: clear ade session purpose %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "ade session "+id)
}

// ReviewAgent returns the task's review agent row, (nil, nil) when it has none.
func (r *AdeSessionsRepo) ReviewAgent(taskID string) (*model.AdeSession, error) {
	c := kiraschema.AdeSessions
	sb := r.selectSessions()
	sb.Where(sb.Equal(c.TaskID, taskID), sb.Equal(c.Purpose, model.AdeSessionPurposeReview))
	rec, err := adeOne(r.DB, sb, func(row *sql.Row) (*model.AdeSession, error) {
		s, err := scanAdeSessionRow(row)
		if err != nil {
			return nil, err
		}
		return &s, nil
	})
	if err != nil {
		return nil, fmt.Errorf("repos: get review agent of task %s: %w", taskID, err)
	}
	return rec, nil
}

// MarkRunning sets state='running' and terminal_id — Tracker.Compose's own resume path (a
// previously stopped session picked back up under a fresh terminal id).
func (r *AdeSessionsRepo) MarkRunning(id, terminalID string, lastActiveAt int64) error {
	c := kiraschema.AdeSessions
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.State, model.AdeSessionStateRunning), ub.Assign(c.TerminalID, nullableString(terminalID)),
		ub.Assign(c.LastActiveAt, lastActiveAt)).Where(ub.Equal(c.ID, id))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("repos: mark ade session running %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "ade session "+id)
}

// MarkStopped sets state='stopped' and clears terminal_id — Tracker.Reconcile's own exit path
// (the pty is gone; the row's own history stays, just with nothing live to point at).
func (r *AdeSessionsRepo) MarkStopped(id string, lastActiveAt int64) error {
	c := kiraschema.AdeSessions
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.State, model.AdeSessionStateStopped), ub.Assign(c.TerminalID, nil), ub.Assign(c.LastActiveAt, lastActiveAt)).
		Where(ub.Equal(c.ID, id))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("repos: mark ade session stopped %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "ade session "+id)
}

// Delete removes a row — Tracker.Abort's own path for a fresh session whose terminal never opened.
func (r *AdeSessionsRepo) Delete(id string) error {
	c := kiraschema.AdeSessions
	del := r.Dialect.DeleteFrom(c.Table)
	del.Where(del.Equal(c.ID, id))
	res, err := sqlb.Exec(r.DB, del)
	if err != nil {
		return fmt.Errorf("repos: delete ade session %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "ade session "+id)
}

// SetAgentSessionID updates the tracked agent session id — a session's own id can change across
// a /clear (a fresh SessionStart with a different session_id, same terminal_id); this keeps the
// row pointed at whichever id `--resume` must pass next.
func (r *AdeSessionsRepo) SetAgentSessionID(id, agentSessionID string) error {
	c := kiraschema.AdeSessions
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.AgentSessionID, agentSessionID)).Where(ub.Equal(c.ID, id))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("repos: set ade session agent session id %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "ade session "+id)
}

// ProviderOfAgentSession is the provider id recorded with the newest row of an agent session, ""
// when no row has it.
func (r *AdeSessionsRepo) ProviderOfAgentSession(agentSessionID string) (string, error) {
	if agentSessionID == "" {
		return "", nil
	}
	var id string
	c := kiraschema.AdeSessions
	sb := r.Dialect.Select(c.Provider).From(c.Table)
	sb.Where(sb.Equal(c.AgentSessionID, agentSessionID)).OrderBy(c.StartedAt + " DESC").Limit(1)
	err := sqlb.QueryRow(r.DB, sb).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("repos: provider of agent session %s: %w", agentSessionID, err)
	}
	return id, nil
}

// AgentSessionIDsExcept lists the non-empty agent session ids of every row but id.
func (r *AdeSessionsRepo) AgentSessionIDsExcept(id string) ([]string, error) {
	c := kiraschema.AdeSessions
	sb := r.Dialect.Select(c.AgentSessionID).From(c.Table)
	sb.Where(sb.NotEqual(c.AgentSessionID, ""), sb.NotEqual(c.ID, id))
	rows, err := sqlb.Query(r.DB, sb)
	if err != nil {
		return nil, fmt.Errorf("repos: list agent session ids: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("repos: scan agent session id: %w", err)
		}
		ids = append(ids, s)
	}
	return ids, rows.Err()
}

// SetLastActive bumps last_active_at alone — every hook event Tracker.HandleEvent sees for a
// running session touches this, without needing a full row re-read first.
func (r *AdeSessionsRepo) SetLastActive(id string, lastActiveAt int64) error {
	c := kiraschema.AdeSessions
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.LastActiveAt, lastActiveAt)).Where(ub.Equal(c.ID, id))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("repos: set ade session last active %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "ade session "+id)
}

// StopAllTaskRunning marks every still-'running' row 'stopped' and clears its terminal_id (boot
// recovery: no process or PTY of the previous life survives).
func (r *AdeSessionsRepo) StopAllTaskRunning(now int64) error {
	c := kiraschema.AdeSessions
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.State, model.AdeSessionStateStopped), ub.Assign(c.TerminalID, nil), ub.Assign(c.LastActiveAt, now)).
		Where(ub.Equal(c.State, model.AdeSessionStateRunning))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos: stop all running task sessions: %w", err)
	}
	return nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
