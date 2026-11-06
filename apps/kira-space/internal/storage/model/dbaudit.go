package model

// DbAudit* mirror ade_db_audit's kind and the call statuses.
const (
	DbAuditKindCall     = "call"
	DbAuditKindApproval = "approval"

	DbAuditRunning   = "running"
	DbAuditOK        = "ok"
	DbAuditError     = "error"
	DbAuditRefused   = "refused"
	DbAuditCancelled = "cancelled"

	DbAuditPending = "pending"
)

// DbAuditCall is one session-server tool call with its approvals (oldest first). Timestamps are
// unix milliseconds; EndedMs is nil while the call runs.
type DbAuditCall struct {
	ID                 string
	TaskID             string
	SessionID          string
	StepID             string
	RunID              string
	StartedMs          int64
	EndedMs            *int64
	Status             string
	Tool               string
	ConnectionID       string
	ConnectionName     string
	Mode               string
	Target             string
	Statement          string
	StatementLen       int
	StatementWithheld  bool
	StatementTruncated bool
	Summary            string
	Error              string
	Approvals          []DbAuditApproval
}

// DbAuditApproval is one approval raised inside a call. Outcome is "pending" until resolved.
type DbAuditApproval struct {
	ID            string
	CallID        string
	TaskID        string
	SessionID     string
	StepID        string
	RunID         string
	StartedMs     int64
	EndedMs       *int64
	Outcome       string
	Subject       string
	Reason        string
	Stage         string
	ExemptionJSON string
}
