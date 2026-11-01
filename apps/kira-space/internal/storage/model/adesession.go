package model

import "fmt"

// AdeSessionStateRunning/AdeSessionStateStopped mirror ade_sessions.state's own CHECK constraint
// (migrations/0004_p129_ade_sessions.sql) — the two states ade.Tracker ever writes.
const (
	AdeSessionStateRunning = "running"
	AdeSessionStateStopped = "stopped"
)

// AdeSessionMode* mirror ade_sessions.mode's CHECK (migrations/0010_p146_ade_runs.sql).
const (
	AdeSessionModeTUI      = "tui"
	AdeSessionModeHeadless = "headless"
)

// AdeSession is one row of `ade_sessions` — a Claude Code session of a task, interactive (tui) or a
// background run (headless), keyed on ID (never AgentSessionID, which can change across a
// session's own /clear). TerminalID is "" once the session has stopped — a history row outlives its
// own live terminal.
type AdeSession struct {
	ID             string `json:"id"`
	AgentSessionID string `json:"agentSessionId"`
	// Provider is the harness provider id that ran the conversation; resume uses it.
	Provider     string `json:"provider"`
	Cwd          string `json:"cwd"`
	State        string `json:"state"`
	TerminalID   string `json:"terminalId"`
	StartedAt    int64  `json:"startedAt"`
	LastActiveAt int64  `json:"lastActiveAt"`
	Mode         string `json:"mode"`
	TaskID       string `json:"taskId"`
	BranchID     string `json:"branchId"`
	StepID       string `json:"stepId"`
	RunID        string `json:"runId"`
	Resumes      string `json:"resumes"`
	// Purpose is "" for a step or branch session, "review" for the task's one review agent.
	Purpose string `json:"purpose"`
}

// AdeSessionPurposeReview marks the task's review agent row (ade_sessions.purpose).
const AdeSessionPurposeReview = "review"

// Validate asserts the identity/shape fields no SQL constraint covers by itself (the CHECKs on
// state and mode still guard the DB directly; this catches the same mistakes earlier, with a
// message naming the record).
func (s AdeSession) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("model: ade session: id is required")
	}
	if s.TaskID == "" {
		return fmt.Errorf("model: ade session %q: taskId is required", s.ID)
	}
	if s.Mode != AdeSessionModeTUI && s.Mode != AdeSessionModeHeadless {
		return fmt.Errorf("model: ade session %q: invalid mode %q", s.ID, s.Mode)
	}
	if s.Mode == AdeSessionModeHeadless && s.RunID == "" {
		return fmt.Errorf("model: ade session %q: a headless session needs a runId", s.ID)
	}
	if s.Mode == AdeSessionModeHeadless && s.AgentSessionID == "" {
		return fmt.Errorf("model: ade session %q: a headless session needs an agentSessionId", s.ID)
	}
	if s.Cwd == "" {
		return fmt.Errorf("model: ade session %q: cwd is required", s.ID)
	}
	if s.State != AdeSessionStateRunning && s.State != AdeSessionStateStopped {
		return fmt.Errorf("model: ade session %q: invalid state %q", s.ID, s.State)
	}
	return nil
}
