package model

// AdeArtifact* mirror ade_task_artifacts' CHECK values.
const (
	AdeArtifactChangeRequest = "changeRequest"
	AdeArtifactIssue         = "issue"
	AdeArtifactBranch        = "branch"
	AdeArtifactDeployment    = "deployment"
	AdeArtifactDoc           = "doc"
	AdeArtifactLink          = "link"
	AdeArtifactSourceAgent   = "agent"
	AdeArtifactSourceUser    = "user"
)

// AdeTaskArtifact is one row of ade_task_artifacts: something an agent or the user reported for a task
// that the app cannot detect itself. URL or Ref is set.
type AdeTaskArtifact struct {
	ID        string
	TaskID    string
	Kind      string
	Title     string
	URL       string
	Ref       string
	Source    string
	StepID    string
	RunID     string
	CreatedAt int64
	UpdatedAt int64
	// SessionID is the Kira session that reported it; "" for a user add or an old row.
	SessionID string
}
