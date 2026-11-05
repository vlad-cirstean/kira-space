package scriptruns

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/kirathecat/kira-studio/internal/runoutcome"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

const columns = `id, script_id, script_name, color, kind, trigger_kind, state, terminal_id, cwd, command, outcome_json, created_at, started_at, finished_at, model, session_id, prompt, params_json, tools_json, task_id, task_title, branch_id, branch_label, resume_id, resumed_from, provider`

// keepFinished is how many finished runs are retained.
const keepFinished = 500

// Repo reads and writes script_runs.
type Repo struct{ DB *sql.DB }

type rowScanner interface{ Scan(dest ...any) error }

func scan(row rowScanner) (Run, error) {
	var r Run
	var outcome, paramsJSON, toolsJSON, resumeID string
	var started, finished sql.NullInt64
	if err := row.Scan(&r.ID, &r.ScriptID, &r.ScriptName, &r.Color, &r.Kind, &r.Trigger, &r.State, &r.TerminalID,
		&r.Cwd, &r.Command, &outcome, &r.CreatedAt, &started, &finished,
		&r.Model, &r.SessionID, &r.Prompt, &paramsJSON, &toolsJSON,
		&r.TaskID, &r.TaskTitle, &r.BranchID, &r.BranchLabel, &resumeID, &r.ResumedFrom, &r.Provider); err != nil {
		return Run{}, err
	}
	r.Resumable = resumeID != ""
	r.Params = []RunParam{}
	if err := json.Unmarshal([]byte(paramsJSON), &r.Params); err != nil {
		return Run{}, fmt.Errorf("scriptruns: decode run %s params: %w", r.ID, err)
	}
	if err := json.Unmarshal([]byte(toolsJSON), &r.Tools); err != nil {
		return Run{}, fmt.Errorf("scriptruns: decode run %s tools: %w", r.ID, err)
	}
	r.Tools = r.Tools.nonNil()
	if outcome != "" {
		var o runoutcome.Outcome
		if err := json.Unmarshal([]byte(outcome), &o); err != nil {
			return Run{}, fmt.Errorf("scriptruns: decode run %s outcome: %w", r.ID, err)
		}
		r.Outcome = &o
	}
	if started.Valid {
		r.StartedAt = &started.Int64
	}
	if finished.Valid {
		r.FinishedAt = &finished.Int64
	}
	return r, nil
}

func encode(o *runoutcome.Outcome) (string, error) {
	if o == nil {
		return "", nil
	}
	b, err := json.Marshal(o)
	if err != nil {
		return "", fmt.Errorf("scriptruns: encode outcome: %w", err)
	}
	return string(b), nil
}

// Insert writes a new run.
func (r *Repo) Insert(run Run) error {
	outcome, err := encode(run.Outcome)
	if err != nil {
		return err
	}
	params, err := json.Marshal(nonNilParams(run.Params))
	if err != nil {
		return fmt.Errorf("scriptruns: encode params: %w", err)
	}
	tools, err := json.Marshal(run.Tools.nonNil())
	if err != nil {
		return fmt.Errorf("scriptruns: encode tools: %w", err)
	}
	if _, err := r.DB.Exec(`INSERT INTO script_runs (`+columns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		run.ID, run.ScriptID, run.ScriptName, run.Color, run.Kind, run.Trigger, run.State, run.TerminalID,
		run.Cwd, run.Command, outcome, run.CreatedAt, run.StartedAt, run.FinishedAt,
		run.Model, run.SessionID, run.Prompt, string(params), string(tools),
		run.TaskID, run.TaskTitle, run.BranchID, run.BranchLabel, run.ResumedFrom, run.Provider); err != nil {
		return fmt.Errorf("scriptruns: insert %s: %w", run.ID, err)
	}
	return nil
}

// Get returns one run; sql.ErrNoRows wrapped when absent.
func (r *Repo) Get(id string) (Run, error) {
	run, err := scan(r.DB.QueryRow(`SELECT `+columns+` FROM script_runs WHERE id = ?`, id))
	if err != nil {
		return Run{}, fmt.Errorf("scriptruns: get %s: %w", id, err)
	}
	return run, nil
}

// List returns the newest runs first.
func (r *Repo) List(limit int) ([]Run, error) {
	rows, err := r.DB.Query(`SELECT `+columns+` FROM script_runs ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (Run, bool, error) {
		run, err := scan(rows)
		return run, true, err
	})
}

// Live returns the running and waiting runs, newest first.
func (r *Repo) Live() ([]Run, error) {
	rows, err := r.DB.Query(`SELECT ` + columns + ` FROM script_runs WHERE state IN ('running', 'waiting') ORDER BY created_at DESC, id DESC`)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (Run, bool, error) {
		run, err := scan(rows)
		return run, true, err
	})
}

// ListByTask returns the newest runs started for a task first.
func (r *Repo) ListByTask(taskID string, limit int) ([]Run, error) {
	rows, err := r.DB.Query(`SELECT `+columns+` FROM script_runs WHERE task_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, taskID, limit)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (Run, bool, error) {
		run, err := scan(rows)
		return run, true, err
	})
}

// restoreExpr marks a run a quit or crash ended (source `restart`) to be offered at the next start;
// the argument is the source the outcome carries.
const restoreExpr = `CASE WHEN ? = 'restart' THEN 1 ELSE restore_pending END`

// resumeExpr keeps a smart run's Claude session as resume_id when a restart ended it; the argument
// is the source the outcome carries.
const resumeExpr = `CASE WHEN kind = 'smart' AND ? = 'restart' THEN session_id ELSE '' END`

// Finish ends the running run matched by where (a column and value) with out; it returns the updated
// run, or (nil, nil) when no running row matches (a repeated exit).
func (r *Repo) finish(column, value string, out runoutcome.Outcome, now int64) (*Run, error) {
	outcome, err := encode(&out)
	if err != nil {
		return nil, err
	}
	var id string
	err = r.DB.QueryRow(`UPDATE script_runs SET state = ?, outcome_json = ?, finished_at = ?, resume_id = `+resumeExpr+`, restore_pending = `+restoreExpr+`
		WHERE `+column+` = ? AND state = 'running' RETURNING id`, string(out.Status), outcome, now, string(out.Source), string(out.Source), value).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scriptruns: finish %s %s: %w", column, value, err)
	}
	run, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// FinishByRun ends a run by its id.
func (r *Repo) FinishByRun(id string, out runoutcome.Outcome, now int64) (*Run, error) {
	return r.finish("id", id, out, now)
}

// FinishByTerminal ends the running run bound to a terminal session.
func (r *Repo) FinishByTerminal(terminalID string, out runoutcome.Outcome, now int64) (*Run, error) {
	return r.finish("terminal_id", terminalID, out, now)
}

// FailRunning ends every running run with out and returns them.
func (r *Repo) FailRunning(out runoutcome.Outcome, now int64) ([]Run, error) {
	outcome, err := encode(&out)
	if err != nil {
		return nil, err
	}
	rows, err := r.DB.Query(`UPDATE script_runs SET state = ?, outcome_json = ?, finished_at = ?, resume_id = `+resumeExpr+`, restore_pending = `+restoreExpr+`
		WHERE state = 'running' RETURNING `+columns, string(out.Status), outcome, now, string(out.Source), string(out.Source))
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (Run, bool, error) {
		run, err := scan(rows)
		return run, true, err
	})
}

// TakeRestore clears every restore mark and returns the marked run ids.
func (r *Repo) TakeRestore() ([]string, error) {
	rows, err := r.DB.Query(`UPDATE script_runs SET restore_pending = 0 WHERE restore_pending = 1 RETURNING id`)
	ids, err := sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (string, bool, error) {
		var id string
		err := rows.Scan(&id)
		return id, true, err
	})
	if err != nil {
		return nil, fmt.Errorf("scriptruns: take restore: %w", err)
	}
	return ids, nil
}

// TakeResume claims a run's resume spec once: it clears resume_id and returns the Claude session
// id, ok false when the run has none (already taken, or not resumable).
func (r *Repo) TakeResume(id string) (sessionID string, ok bool, err error) {
	err = r.DB.QueryRow(`UPDATE script_runs SET resume_id = '' WHERE id = ? AND resume_id <> '' RETURNING session_id`, id).Scan(&sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("scriptruns: take resume %s: %w", id, err)
	}
	return sessionID, true, nil
}

// RestoreResume puts a taken resume spec back when the launch failed before it began.
func (r *Repo) RestoreResume(id, sessionID string) error {
	if _, err := r.DB.Exec(`UPDATE script_runs SET resume_id = ? WHERE id = ?`, sessionID, id); err != nil {
		return fmt.Errorf("scriptruns: restore resume %s: %w", id, err)
	}
	return nil
}

// SetSessionID stores the conversation id an agent assigned a running run.
func (r *Repo) SetSessionID(id, sessionID string) error {
	if _, err := r.DB.Exec(`UPDATE script_runs SET session_id = ? WHERE id = ?`, sessionID, id); err != nil {
		return fmt.Errorf("scriptruns: set session id %s: %w", id, err)
	}
	return nil
}

// Active returns the newest running or waiting run of a script other than exceptID, (nil, nil)
// when none.
func (r *Repo) Active(scriptID, exceptID string) (*Run, error) {
	run, err := scan(r.DB.QueryRow(`SELECT `+columns+` FROM script_runs WHERE script_id = ? AND id <> ? AND state IN ('running', 'waiting')
		ORDER BY created_at DESC, id DESC LIMIT 1`, scriptID, exceptID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scriptruns: active run of %s: %w", scriptID, err)
	}
	return &run, nil
}

// Running returns the newest running run of one script for one task and branch ("" for none),
// (nil, nil) when none.
func (r *Repo) Running(scriptID, taskID, branchID string) (*Run, error) {
	run, err := scan(r.DB.QueryRow(`SELECT `+columns+` FROM script_runs WHERE script_id = ? AND task_id = ? AND branch_id = ? AND state = 'running'
		ORDER BY created_at DESC, id DESC LIMIT 1`, scriptID, taskID, branchID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scriptruns: running run of %s: %w", scriptID, err)
	}
	return &run, nil
}

// Waiting returns the runs waiting for the user's answer, oldest first.
func (r *Repo) Waiting() ([]Run, error) {
	rows, err := r.DB.Query(`SELECT ` + columns + ` FROM script_runs WHERE state = 'waiting' ORDER BY created_at ASC, id ASC`)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (Run, bool, error) {
		run, err := scan(rows)
		return run, true, err
	})
}

// BeginWaiting turns a waiting run into a running one with the fields the start resolved. It
// reports false when the run was already answered.
func (r *Repo) BeginWaiting(id string, run Run) (bool, error) {
	params, err := json.Marshal(nonNilParams(run.Params))
	if err != nil {
		return false, fmt.Errorf("scriptruns: encode params: %w", err)
	}
	tools, err := json.Marshal(run.Tools.nonNil())
	if err != nil {
		return false, fmt.Errorf("scriptruns: encode tools: %w", err)
	}
	res, err := r.DB.Exec(`UPDATE script_runs SET state = 'running', started_at = ?, cwd = ?, command = ?, prompt = ?, params_json = ?,
		tools_json = ?, model = ?, session_id = ?, provider = ?, task_id = ?, task_title = ?, branch_id = ?, branch_label = ?
		WHERE id = ? AND state = 'waiting'`, run.StartedAt, run.Cwd, run.Command, run.Prompt, string(params), string(tools),
		run.Model, run.SessionID, run.Provider, run.TaskID, run.TaskTitle, run.BranchID, run.BranchLabel, id)
	if err != nil {
		return false, fmt.Errorf("scriptruns: begin waiting %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("scriptruns: begin waiting %s: %w", id, err)
	}
	return n > 0, nil
}

// What SkipWaiting matches: one run, every waiting run of a script, or all of them.
const (
	skipByID     = "id"
	skipByScript = "script_id"
	skipAll      = ""
)

// SkipWaiting ends the waiting runs matched by by/value (value is ignored for skipAll) as skipped
// with reason and returns them.
func (r *Repo) SkipWaiting(by, value, reason string, src runoutcome.Source, now int64) ([]Run, error) {
	out := runoutcome.Skipped(reason, src)
	outcome, err := encode(&out)
	if err != nil {
		return nil, err
	}
	query := `UPDATE script_runs SET state = ?, outcome_json = ?, finished_at = ? WHERE state = 'waiting'`
	args := []any{string(out.Status), outcome, now}
	if by != skipAll {
		query += ` AND ` + by + ` = ?`
		args = append(args, value)
	}
	rows, err := r.DB.Query(query+` RETURNING `+columns, args...)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (Run, bool, error) {
		run, err := scan(rows)
		return run, true, err
	})
}

// Purge keeps only the newest keepFinished finished runs and drops the logs of the rest.
func (r *Repo) Purge() error {
	if _, err := r.DB.Exec(`DELETE FROM script_runs WHERE state NOT IN ('running', 'waiting') AND id NOT IN
		(SELECT id FROM script_runs WHERE state NOT IN ('running', 'waiting') ORDER BY created_at DESC, id DESC LIMIT ?)`, keepFinished); err != nil {
		return fmt.Errorf("scriptruns: purge: %w", err)
	}
	if _, err := r.DB.Exec(`DELETE FROM script_run_logs WHERE run_id NOT IN (SELECT id FROM script_runs)`); err != nil {
		return fmt.Errorf("scriptruns: purge logs: %w", err)
	}
	return nil
}

func (t RunTools) nonNil() RunTools {
	if t.Tools == nil {
		t.Tools = []string{}
	}
	if t.AllowedTools == nil {
		t.AllowedTools = []string{}
	}
	if t.McpServers == nil {
		t.McpServers = []string{}
	}
	return t
}

func nonNilParams(p []RunParam) []RunParam {
	if p == nil {
		return []RunParam{}
	}
	return p
}

// LogChunk is one stored log line.
type LogChunk struct {
	Seq    int    `json:"seq"`
	Stream string `json:"stream"`
	Text   string `json:"text"`
}

// maxLogLines is how many lines of one run are kept; older ones are dropped.
const maxLogLines = 5000

// AppendLogs stores chunks whose Seq the caller assigned, then drops lines older than the newest
// maxLogLines.
func (r *Repo) AppendLogs(runID string, chunks []LogChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("scriptruns: append logs: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`INSERT INTO script_run_logs (run_id, seq, stream, text) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("scriptruns: append logs: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	for _, c := range chunks {
		if _, err := stmt.Exec(runID, c.Seq, c.Stream, c.Text); err != nil {
			return fmt.Errorf("scriptruns: append logs: %w", err)
		}
	}
	last := chunks[len(chunks)-1].Seq
	if _, err := tx.Exec(`DELETE FROM script_run_logs WHERE run_id = ? AND seq <= ?`, runID, last-maxLogLines); err != nil {
		return fmt.Errorf("scriptruns: trim logs: %w", err)
	}
	return tx.Commit()
}

// LogPage is what ReadLog answers.
type LogPage struct {
	Chunks []LogChunk `json:"chunks"`
	// Truncated is true when older lines were dropped.
	Truncated bool `json:"truncated"`
}

// LogQuery picks lines of one run: FromEnd takes the newest Limit lines (tail), else the oldest (head);
// a non-empty Contains keeps lines holding it as a case-sensitive substring.
type LogQuery struct {
	FromEnd  bool
	Limit    int
	Contains string
}

// LogSlice is what QueryLog answers: the picked lines oldest first, how many lines matched before
// the limit, and whether older lines were dropped from the store.
type LogSlice struct {
	Chunks    []LogChunk
	Matched   int
	Truncated bool
}

// logFilter is the WHERE clause and arguments of a log query.
func logFilter(runID, contains string) (string, []any) {
	if contains == "" {
		return `run_id = ?`, []any{runID}
	}
	return `run_id = ? AND instr(text, ?) > 0`, []any{runID, contains}
}

// QueryLog returns up to q.Limit stored lines of a run, filtered, oldest first.
func (r *Repo) QueryLog(runID string, q LogQuery) (LogSlice, error) {
	where, args := logFilter(runID, q.Contains)
	var out LogSlice
	if err := r.DB.QueryRow(`SELECT COUNT(*) FROM script_run_logs WHERE `+where, args...).Scan(&out.Matched); err != nil {
		return LogSlice{}, fmt.Errorf("scriptruns: count log %s: %w", runID, err)
	}
	order := "ASC"
	if q.FromEnd {
		order = "DESC"
	}
	rows, err := r.DB.Query(`SELECT seq, stream, text FROM script_run_logs WHERE `+where+` ORDER BY seq `+order+` LIMIT ?`, append(args, q.Limit)...)
	chunks, err := sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (LogChunk, bool, error) {
		var c LogChunk
		err := rows.Scan(&c.Seq, &c.Stream, &c.Text)
		return c, true, err
	})
	if err != nil {
		return LogSlice{}, fmt.Errorf("scriptruns: query log %s: %w", runID, err)
	}
	if q.FromEnd {
		slices.Reverse(chunks)
	}
	out.Chunks = chunks
	if out.Chunks == nil {
		out.Chunks = []LogChunk{}
	}
	var first sql.NullInt64
	if err := r.DB.QueryRow(`SELECT MIN(seq) FROM script_run_logs WHERE run_id = ?`, runID).Scan(&first); err != nil {
		return LogSlice{}, fmt.Errorf("scriptruns: query log %s: %w", runID, err)
	}
	out.Truncated = first.Valid && first.Int64 > 1
	return out, nil
}

// ReadLog returns the lines of a run after afterSeq, oldest first.
func (r *Repo) ReadLog(runID string, afterSeq int) (LogPage, error) {
	rows, err := r.DB.Query(`SELECT seq, stream, text FROM script_run_logs WHERE run_id = ? AND seq > ? ORDER BY seq ASC LIMIT ?`,
		runID, afterSeq, maxLogLines)
	chunks, err := sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (LogChunk, bool, error) {
		var c LogChunk
		err := rows.Scan(&c.Seq, &c.Stream, &c.Text)
		return c, true, err
	})
	if err != nil {
		return LogPage{}, fmt.Errorf("scriptruns: read log %s: %w", runID, err)
	}
	if chunks == nil {
		chunks = []LogChunk{}
	}
	var first sql.NullInt64
	if err := r.DB.QueryRow(`SELECT MIN(seq) FROM script_run_logs WHERE run_id = ?`, runID).Scan(&first); err != nil {
		return LogPage{}, fmt.Errorf("scriptruns: read log %s: %w", runID, err)
	}
	return LogPage{Chunks: chunks, Truncated: first.Valid && first.Int64 > 1}, nil
}
