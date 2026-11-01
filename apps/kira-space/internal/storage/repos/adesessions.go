package repos

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

const adeSessionsSelectColumns = `id, agent_session_id, cwd, state, terminal_id, started_at, last_active_at,
	mode, task_id, branch_id, step_id, run_id, resumes, purpose, provider`

// AdeSessionsRepo reads and writes `ade_sessions` — the history of every task Claude Code session
// (interactive or headless) the app has spawned or resumed.
type AdeSessionsRepo struct {
	DB *sql.DB
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
	rec, err := sqlitex.QueryOne(r.DB, func(row *sql.Row) (*model.AdeSession, error) {
		s, err := scanAdeSessionRow(row)
		if err != nil {
			return nil, err
		}
		return &s, nil
	}, `SELECT `+adeSessionsSelectColumns+` FROM ade_sessions WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("repos: get ade session %s: %w", id, err)
	}
	return rec, nil
}

// ListTask returns every row, running first then most recently active.
func (r *AdeSessionsRepo) ListTask() ([]model.AdeSession, error) {
	rows, err := r.DB.Query(`SELECT ` + adeSessionsSelectColumns + ` FROM ade_sessions
		ORDER BY state = 'running' DESC, last_active_at DESC, id`)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeSession, bool, error) {
		rec, err := scanAdeSessionRow(rows)
		return rec, true, err
	})
}

// HasTask reports whether any session row, running or stopped, belongs to the task.
func (r *AdeSessionsRepo) HasTask(taskID string) (bool, error) {
	var n int
	if err := r.DB.QueryRow(`SELECT COUNT(*) FROM ade_sessions WHERE task_id = ?`, taskID).Scan(&n); err != nil {
		return false, fmt.Errorf("repos: count ade sessions of task %s: %w", taskID, err)
	}
	return n > 0, nil
}

// CountRunningHeadless counts headless task sessions still running.
func (r *AdeSessionsRepo) CountRunningHeadless() (int, error) {
	var n int
	err := r.DB.QueryRow(`SELECT count(*) FROM ade_sessions WHERE state = ? AND mode = ?`,
		model.AdeSessionStateRunning, model.AdeSessionModeHeadless).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("repos: count running headless ade sessions: %w", err)
	}
	return n, nil
}

// ListRunningTUI returns the task rows whose interactive terminal is open.
func (r *AdeSessionsRepo) ListRunningTUI() ([]model.AdeSession, error) {
	rows, err := r.DB.Query(`SELECT `+adeSessionsSelectColumns+` FROM ade_sessions
		WHERE task_id != '' AND mode = ? AND state = ?`, model.AdeSessionModeTUI, model.AdeSessionStateRunning)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeSession, bool, error) {
		rec, err := scanAdeSessionRow(rows)
		return rec, true, err
	})
}

// InsertHeadless writes a headless task session row (no terminal).
func (r *AdeSessionsRepo) InsertHeadless(rec model.AdeSession) error {
	if rec.TaskID == "" {
		return fmt.Errorf("repos: insert headless ade session %s: taskId is required", rec.ID)
	}
	if err := rec.Validate(); err != nil {
		return fmt.Errorf("repos: %w", err)
	}
	if _, err := r.DB.Exec(
		`INSERT INTO ade_sessions (id, mode, task_id, branch_id, step_id, run_id, resumes, purpose,
			agent_session_id, cwd, state, terminal_id, started_at, last_active_at, provider)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.Mode, rec.TaskID, rec.BranchID, rec.StepID, rec.RunID, rec.Resumes, rec.Purpose,
		rec.AgentSessionID, rec.Cwd, rec.State, nullableString(rec.TerminalID), rec.StartedAt, rec.LastActiveAt, rec.Provider,
	); err != nil {
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
	if _, err := r.DB.Exec(
		`INSERT INTO ade_sessions (id, mode, task_id, branch_id, step_id, run_id, resumes, purpose,
			agent_session_id, cwd, state, terminal_id, started_at, last_active_at, provider)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.Mode, rec.TaskID, rec.BranchID, rec.StepID, rec.RunID, rec.Resumes, rec.Purpose,
		rec.AgentSessionID, rec.Cwd, rec.State, nullableString(rec.TerminalID), rec.StartedAt, rec.LastActiveAt, rec.Provider,
	); err != nil {
		return fmt.Errorf("repos: insert tui ade session %s: %w", rec.ID, err)
	}
	return nil
}

// ClearPurpose turns a review agent row into a plain stopped session (its cwd is gone, a fresh
// review agent replaces it).
func (r *AdeSessionsRepo) ClearPurpose(id string) error {
	res, err := r.DB.Exec(`UPDATE ade_sessions SET purpose = '' WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("repos: clear ade session purpose %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "ade session "+id)
}

// ReviewAgent returns the task's review agent row, (nil, nil) when it has none.
func (r *AdeSessionsRepo) ReviewAgent(taskID string) (*model.AdeSession, error) {
	rec, err := sqlitex.QueryOne(r.DB, func(row *sql.Row) (*model.AdeSession, error) {
		s, err := scanAdeSessionRow(row)
		if err != nil {
			return nil, err
		}
		return &s, nil
	}, `SELECT `+adeSessionsSelectColumns+` FROM ade_sessions WHERE task_id = ? AND purpose = 'review'`, taskID)
	if err != nil {
		return nil, fmt.Errorf("repos: get review agent of task %s: %w", taskID, err)
	}
	return rec, nil
}

// MarkRunning sets state='running' and terminal_id — Tracker.Compose's own resume path (a
// previously stopped session picked back up under a fresh terminal id).
func (r *AdeSessionsRepo) MarkRunning(id, terminalID string, lastActiveAt int64) error {
	res, err := r.DB.Exec(
		`UPDATE ade_sessions SET state = ?, terminal_id = ?, last_active_at = ? WHERE id = ?`,
		model.AdeSessionStateRunning, nullableString(terminalID), lastActiveAt, id,
	)
	if err != nil {
		return fmt.Errorf("repos: mark ade session running %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "ade session "+id)
}

// MarkStopped sets state='stopped' and clears terminal_id — Tracker.Reconcile's own exit path
// (the pty is gone; the row's own history stays, just with nothing live to point at).
func (r *AdeSessionsRepo) MarkStopped(id string, lastActiveAt int64) error {
	res, err := r.DB.Exec(
		`UPDATE ade_sessions SET state = ?, terminal_id = NULL, last_active_at = ? WHERE id = ?`,
		model.AdeSessionStateStopped, lastActiveAt, id,
	)
	if err != nil {
		return fmt.Errorf("repos: mark ade session stopped %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "ade session "+id)
}

// Delete removes a row — Tracker.Abort's own path for a fresh session whose terminal never opened.
func (r *AdeSessionsRepo) Delete(id string) error {
	res, err := r.DB.Exec(`DELETE FROM ade_sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("repos: delete ade session %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "ade session "+id)
}

// SetAgentSessionID updates the tracked agent session id — a session's own id can change across
// a /clear (a fresh SessionStart with a different session_id, same terminal_id); this keeps the
// row pointed at whichever id `--resume` must pass next.
func (r *AdeSessionsRepo) SetAgentSessionID(id, agentSessionID string) error {
	res, err := r.DB.Exec(`UPDATE ade_sessions SET agent_session_id = ? WHERE id = ?`, agentSessionID, id)
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
	err := r.DB.QueryRow(`SELECT provider FROM ade_sessions WHERE agent_session_id = ?
		ORDER BY started_at DESC LIMIT 1`, agentSessionID).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("repos: provider of agent session %s: %w", agentSessionID, err)
	}
	return id, nil
}

// SetLastActive bumps last_active_at alone — every hook event Tracker.HandleEvent sees for a
// running session touches this, without needing a full row re-read first.
func (r *AdeSessionsRepo) SetLastActive(id string, lastActiveAt int64) error {
	res, err := r.DB.Exec(`UPDATE ade_sessions SET last_active_at = ? WHERE id = ?`, lastActiveAt, id)
	if err != nil {
		return fmt.Errorf("repos: set ade session last active %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "ade session "+id)
}

// StopAllTaskRunning marks every still-'running' row 'stopped' and clears its terminal_id (boot
// recovery: no process or PTY of the previous life survives).
func (r *AdeSessionsRepo) StopAllTaskRunning(now int64) error {
	if _, err := r.DB.Exec(
		`UPDATE ade_sessions SET state = ?, terminal_id = NULL, last_active_at = ? WHERE state = ?`,
		model.AdeSessionStateStopped, now, model.AdeSessionStateRunning,
	); err != nil {
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
