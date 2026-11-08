package model

// AdeHandover* mirror ade_task_handovers' CHECK values.
const (
	AdeHandoverSourceAgent = "agent"
	AdeHandoverSourceRun   = "run"
	AdeHandoverSourceUser  = "user"
	// AdeHandoverSourceOrchestrator is a child's brief, Pass the start of an orchestrator pass, Child a
	// finished child's handover mirrored into its orchestrator's chain.
	AdeHandoverSourceOrchestrator = "orchestrator"
	AdeHandoverSourcePass         = "pass"
	AdeHandoverSourceChild        = "child"
	AdeHandoverDraft              = "draft"
	AdeHandoverHanded             = "handed"
	AdeHandoverNote               = "note"
)

// AdeHandover is one row of ade_task_handovers: the text one stage leaves for the next. A draft
// (ToStepID "") is open until the task moves; a handed row is one finished stage move. Step names
// are snapshots taken when the row was written.
type AdeHandover struct {
	ID           string
	TaskID       string
	FromStepID   string
	FromStepName string
	ToStepID     string
	ToStepName   string
	Body         string
	Source       string
	State        string
	Finished     bool
	Dismissed    bool
	Edited       bool
	CreatedAt    int64
	UpdatedAt    int64
	// SessionID is the Kira session that wrote the row; "" for a user row or an old one.
	SessionID string
	// FromTaskID is the child task a mirrored row (source child) copies; "" otherwise.
	FromTaskID string
	// FromTaskTitle is that child's title, filled only by AdeTaskRepo.Handovers and HandoverPage.
	FromTaskTitle string
}

// AdeHandoff turns the open draft of FromStepID into a handed row in the same transaction that moves
// the task. ID names the row a fresh insert creates.
//
// Source user: Notes nil keeps the draft body, "" deletes the draft, other text replaces it (marking it
// edited when it differs); with no draft, non-empty Notes insert a handed user row.
// Source run: Notes is the finished body; it replaces any draft and always inserts a handed run row.
// Source pass: Notes is the pass body; it always inserts a handed pass row and leaves drafts alone.
type AdeHandoff struct {
	ID           string
	FromStepID   string
	FromStepName string
	ToStepID     string
	ToStepName   string
	Notes        *string
	Source       string
	// SessionID is the session behind a run row; a promoted draft keeps its own.
	SessionID string
	Now       int64
}
