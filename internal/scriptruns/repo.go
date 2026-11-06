package scriptruns

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/huandu/go-sqlbuilder"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/runoutcome"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

var columns = []string{
	kiraschema.ScriptRuns.ID, kiraschema.ScriptRuns.ScriptID, kiraschema.ScriptRuns.ScriptName, kiraschema.ScriptRuns.Color,
	kiraschema.ScriptRuns.Kind, kiraschema.ScriptRuns.TriggerKind, kiraschema.ScriptRuns.State, kiraschema.ScriptRuns.TerminalID,
	kiraschema.ScriptRuns.Cwd, kiraschema.ScriptRuns.Command, kiraschema.ScriptRuns.OutcomeJSON, kiraschema.ScriptRuns.CreatedAt,
	kiraschema.ScriptRuns.StartedAt, kiraschema.ScriptRuns.FinishedAt, kiraschema.ScriptRuns.Model, kiraschema.ScriptRuns.SessionID,
	kiraschema.ScriptRuns.Prompt, kiraschema.ScriptRuns.ParamsJSON, kiraschema.ScriptRuns.ToolsJSON, kiraschema.ScriptRuns.TaskID,
	kiraschema.ScriptRuns.TaskTitle, kiraschema.ScriptRuns.BranchID, kiraschema.ScriptRuns.BranchLabel, kiraschema.ScriptRuns.ResumeID,
	kiraschema.ScriptRuns.ResumedFrom, kiraschema.ScriptRuns.Provider,
}

// liveStates are the states of a run that has not finished.
var liveStates = []any{StateRunning, StateWaiting}

// keepFinished is how many finished runs are retained.
const keepFinished = 500

// Repo reads and writes script_runs.
type Repo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

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
	ib := r.Dialect.InsertInto(kiraschema.ScriptRuns.Table).Cols(columns...).Values(
		run.ID, run.ScriptID, run.ScriptName, run.Color, run.Kind, run.Trigger, run.State, run.TerminalID,
		run.Cwd, run.Command, outcome, run.CreatedAt, run.StartedAt, run.FinishedAt,
		run.Model, run.SessionID, run.Prompt, string(params), string(tools),
		run.TaskID, run.TaskTitle, run.BranchID, run.BranchLabel, "", run.ResumedFrom, run.Provider)
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("scriptruns: insert %s: %w", run.ID, err)
	}
	return nil
}

// Get returns one run; sql.ErrNoRows wrapped when absent.
func (r *Repo) Get(id string) (Run, error) {
	c := kiraschema.ScriptRuns
	sb := r.Dialect.Select(columns...).From(c.Table)
	sb.Where(sb.Equal(c.ID, id))
	run, err := scan(sqlb.QueryRow(r.DB, sb))
	if err != nil {
		return Run{}, fmt.Errorf("scriptruns: get %s: %w", id, err)
	}
	return run, nil
}

// List returns the newest runs first.
func (r *Repo) List(limit int) ([]Run, error) {
	c := kiraschema.ScriptRuns
	sb := r.Dialect.Select(columns...).From(c.Table).OrderBy(c.CreatedAt+" DESC", c.ID+" DESC").Limit(limit)
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (Run, bool, error) {
		run, err := scan(rows)
		return run, true, err
	})
}

// Live returns the running and waiting runs, newest first.
func (r *Repo) Live() ([]Run, error) {
	c := kiraschema.ScriptRuns
	sb := r.Dialect.Select(columns...).From(c.Table)
	sb.Where(sb.In(c.State, liveStates...)).OrderBy(c.CreatedAt+" DESC", c.ID+" DESC")
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (Run, bool, error) {
		run, err := scan(rows)
		return run, true, err
	})
}

// ListByTask returns the newest runs started for a task first.
func (r *Repo) ListByTask(taskID string, limit int) ([]Run, error) {
	c := kiraschema.ScriptRuns
	sb := r.Dialect.Select(columns...).From(c.Table)
	sb.Where(sb.Equal(c.TaskID, taskID)).OrderBy(c.CreatedAt+" DESC", c.ID+" DESC").Limit(limit)
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (Run, bool, error) {
		run, err := scan(rows)
		return run, true, err
	})
}

// restoreExpr marks a run a quit or crash ended (source `restart`) to be offered at the next start;
// src is the source the outcome carries.
func restoreExpr(ub *sqlbuilder.UpdateBuilder, src runoutcome.Source) string {
	c := kiraschema.ScriptRuns
	return c.RestorePending + " = CASE WHEN " + ub.Var(string(src)) + " = " + ub.Var(string(runoutcome.SourceRestart)) +
		" THEN " + ub.Var(true) + " ELSE " + c.RestorePending + " END"
}

// resumeExpr keeps a smart run's Claude session as resume_id when a restart ended it; src is the
// source the outcome carries.
func resumeExpr(ub *sqlbuilder.UpdateBuilder, src runoutcome.Source) string {
	c := kiraschema.ScriptRuns
	return c.ResumeID + " = CASE WHEN " + c.Kind + " = " + ub.Var(scripts.KindSmart) + " AND " + ub.Var(string(src)) + " = " +
		ub.Var(string(runoutcome.SourceRestart)) + " THEN " + c.SessionID + " ELSE " + ub.Var("") + " END"
}

// finishSet is the assignment list that ends a run with out.
func finishSet(ub *sqlbuilder.UpdateBuilder, out runoutcome.Outcome, outcome string, now int64) []string {
	c := kiraschema.ScriptRuns
	return []string{
		ub.Assign(c.State, string(out.Status)), ub.Assign(c.OutcomeJSON, outcome), ub.Assign(c.FinishedAt, now),
		resumeExpr(ub, out.Source), restoreExpr(ub, out.Source),
	}
}

// Finish ends the running run matched by where (a column and value) with out; it returns the updated
// run, or (nil, nil) when no running row matches (a repeated exit).
func (r *Repo) finish(column, value string, out runoutcome.Outcome, now int64) (*Run, error) {
	outcome, err := encode(&out)
	if err != nil {
		return nil, err
	}
	var id string
	c := kiraschema.ScriptRuns
	ub := r.Dialect.Update(c.Table)
	ub.Set(finishSet(ub, out, outcome, now)...).Where(ub.Equal(column, value), ub.Equal(c.State, StateRunning)).Returning(c.ID)
	err = sqlb.QueryRow(r.DB, ub).Scan(&id)
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
	c := kiraschema.ScriptRuns
	ub := r.Dialect.Update(c.Table)
	ub.Set(finishSet(ub, out, outcome, now)...).Where(ub.Equal(c.State, StateRunning)).Returning(columns...)
	rows, err := sqlb.Query(r.DB, ub)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (Run, bool, error) {
		run, err := scan(rows)
		return run, true, err
	})
}

// TakeRestore clears every restore mark and returns the marked run ids.
func (r *Repo) TakeRestore() ([]string, error) {
	c := kiraschema.ScriptRuns
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.RestorePending, false)).Where(ub.Equal(c.RestorePending, true)).Returning(c.ID)
	rows, err := sqlb.Query(r.DB, ub)
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
	c := kiraschema.ScriptRuns
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.ResumeID, "")).Where(ub.Equal(c.ID, id), ub.NotEqual(c.ResumeID, "")).Returning(c.SessionID)
	err = sqlb.QueryRow(r.DB, ub).Scan(&sessionID)
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
	c := kiraschema.ScriptRuns
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.ResumeID, sessionID)).Where(ub.Equal(c.ID, id))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("scriptruns: restore resume %s: %w", id, err)
	}
	return nil
}

// SetSessionID stores the conversation id an agent assigned a running run.
func (r *Repo) SetSessionID(id, sessionID string) error {
	c := kiraschema.ScriptRuns
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.SessionID, sessionID)).Where(ub.Equal(c.ID, id))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("scriptruns: set session id %s: %w", id, err)
	}
	return nil
}

// Active returns the newest running or waiting run of a script other than exceptID, (nil, nil)
// when none.
func (r *Repo) Active(scriptID, exceptID string) (*Run, error) {
	c := kiraschema.ScriptRuns
	sb := r.Dialect.Select(columns...).From(c.Table)
	sb.Where(sb.Equal(c.ScriptID, scriptID), sb.NotEqual(c.ID, exceptID), sb.In(c.State, liveStates...)).
		OrderBy(c.CreatedAt+" DESC", c.ID+" DESC").Limit(1)
	run, err := scan(sqlb.QueryRow(r.DB, sb))
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
	c := kiraschema.ScriptRuns
	sb := r.Dialect.Select(columns...).From(c.Table)
	sb.Where(sb.Equal(c.ScriptID, scriptID), sb.Equal(c.TaskID, taskID), sb.Equal(c.BranchID, branchID), sb.Equal(c.State, StateRunning)).
		OrderBy(c.CreatedAt+" DESC", c.ID+" DESC").Limit(1)
	run, err := scan(sqlb.QueryRow(r.DB, sb))
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
	c := kiraschema.ScriptRuns
	sb := r.Dialect.Select(columns...).From(c.Table)
	sb.Where(sb.Equal(c.State, StateWaiting)).OrderBy(c.CreatedAt+" ASC", c.ID+" ASC")
	rows, err := sqlb.Query(r.DB, sb)
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
	c := kiraschema.ScriptRuns
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.State, StateRunning), ub.Assign(c.StartedAt, run.StartedAt), ub.Assign(c.Cwd, run.Cwd),
		ub.Assign(c.Command, run.Command), ub.Assign(c.Prompt, run.Prompt), ub.Assign(c.ParamsJSON, string(params)),
		ub.Assign(c.ToolsJSON, string(tools)), ub.Assign(c.Model, run.Model), ub.Assign(c.SessionID, run.SessionID),
		ub.Assign(c.Provider, run.Provider), ub.Assign(c.TaskID, run.TaskID), ub.Assign(c.TaskTitle, run.TaskTitle),
		ub.Assign(c.BranchID, run.BranchID), ub.Assign(c.BranchLabel, run.BranchLabel)).
		Where(ub.Equal(c.ID, id), ub.Equal(c.State, StateWaiting))
	res, err := sqlb.Exec(r.DB, ub)
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
var (
	skipByID     = kiraschema.ScriptRuns.ID
	skipByScript = kiraschema.ScriptRuns.ScriptID
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
	c := kiraschema.ScriptRuns
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.State, string(out.Status)), ub.Assign(c.OutcomeJSON, outcome), ub.Assign(c.FinishedAt, now))
	conds := []string{ub.Equal(c.State, StateWaiting)}
	if by != skipAll {
		conds = append(conds, ub.Equal(by, value))
	}
	ub.Where(conds...).Returning(columns...)
	rows, err := sqlb.Query(r.DB, ub)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (Run, bool, error) {
		run, err := scan(rows)
		return run, true, err
	})
}

// Purge keeps only the newest keepFinished finished runs; the logs of the rest cascade.
func (r *Repo) Purge() error {
	c := kiraschema.ScriptRuns
	keep := r.Dialect.Select(c.ID).From(c.Table)
	keep.Where(keep.NotIn(c.State, liveStates...)).OrderBy(c.CreatedAt+" DESC", c.ID+" DESC").Limit(keepFinished)
	del := r.Dialect.DeleteFrom(c.Table)
	del.Where(del.NotIn(c.State, liveStates...), del.NotIn(c.ID, keep))
	if _, err := sqlb.Exec(r.DB, del); err != nil {
		return fmt.Errorf("scriptruns: purge: %w", err)
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
	c := kiraschema.ScriptRunLogs
	insert, _ := r.Dialect.InsertInto(c.Table).Cols(c.RunID, c.Seq, c.Stream, c.Text).Values(runID, 0, "", "").Build()
	stmt, err := tx.Prepare(insert)
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
	trim := r.Dialect.DeleteFrom(c.Table)
	trim.Where(trim.Equal(c.RunID, runID), trim.LessEqualThan(c.Seq, last-maxLogLines))
	if _, err := sqlb.Exec(tx, trim); err != nil {
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

// logFilter is the WHERE conditions of a log query on sb.
func (r *Repo) logFilter(sb *sqlbuilder.SelectBuilder, runID, contains string) []string {
	c := kiraschema.ScriptRunLogs
	conds := []string{sb.Equal(c.RunID, runID)}
	if contains != "" {
		conds = append(conds, sb.GreaterThan(sqlb.ContainsExpr(r.Dialect, c.Text, sb.Var(contains)), 0))
	}
	return conds
}

// QueryLog returns up to q.Limit stored lines of a run, filtered, oldest first.
func (r *Repo) QueryLog(runID string, q LogQuery) (LogSlice, error) {
	c := kiraschema.ScriptRunLogs
	var out LogSlice
	cq := r.Dialect.Select("COUNT(*)").From(c.Table)
	cq.Where(r.logFilter(cq, runID, q.Contains)...)
	if err := sqlb.QueryRow(r.DB, cq).Scan(&out.Matched); err != nil {
		return LogSlice{}, fmt.Errorf("scriptruns: count log %s: %w", runID, err)
	}
	order := "ASC"
	if q.FromEnd {
		order = "DESC"
	}
	sb := r.Dialect.Select(c.Seq, c.Stream, c.Text).From(c.Table)
	sb.Where(r.logFilter(sb, runID, q.Contains)...).OrderBy(c.Seq + " " + order).Limit(q.Limit)
	rows, err := sqlb.Query(r.DB, sb)
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
	if err := r.firstSeq(runID).Scan(&first); err != nil {
		return LogSlice{}, fmt.Errorf("scriptruns: query log %s: %w", runID, err)
	}
	out.Truncated = first.Valid && first.Int64 > 1
	return out, nil
}

// ReadLog returns the lines of a run after afterSeq, oldest first.
func (r *Repo) ReadLog(runID string, afterSeq int) (LogPage, error) {
	c := kiraschema.ScriptRunLogs
	sb := r.Dialect.Select(c.Seq, c.Stream, c.Text).From(c.Table)
	sb.Where(sb.Equal(c.RunID, runID), sb.GreaterThan(c.Seq, afterSeq)).OrderBy(c.Seq + " ASC").Limit(maxLogLines)
	rows, err := sqlb.Query(r.DB, sb)
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
	if err := r.firstSeq(runID).Scan(&first); err != nil {
		return LogPage{}, fmt.Errorf("scriptruns: read log %s: %w", runID, err)
	}
	return LogPage{Chunks: chunks, Truncated: first.Valid && first.Int64 > 1}, nil
}

// firstSeq selects the oldest stored line number of a run.
func (r *Repo) firstSeq(runID string) *sql.Row {
	c := kiraschema.ScriptRunLogs
	sb := r.Dialect.Select("MIN(" + c.Seq + ")").From(c.Table)
	sb.Where(sb.Equal(c.RunID, runID))
	return sqlb.QueryRow(r.DB, sb)
}
