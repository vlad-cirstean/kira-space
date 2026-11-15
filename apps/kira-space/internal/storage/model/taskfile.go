package model

// Review states of a task file.
const (
	TaskFileAccepted = "accepted"
	TaskFilePending  = "pending"
	TaskFileInvalid  = "invalid"
)

// TaskFile is one row of task_files: the index entry of a file under a task's folder. Size, Version
// and SHA256 rebuild from the folder; AcceptedSHA256, AcceptedBy, AcceptedAt and Review are state.
type TaskFile struct {
	TaskID         string
	RelPath        string
	Kind           string
	Size           int64
	Version        string
	SHA256         string
	AcceptedSHA256 string
	AcceptedBy     string
	AcceptedAt     int64
	Review         string
	Problem        string
}

// TaskFileIntent is one row of task_file_intents: a file write started and not yet recorded.
type TaskFileIntent struct {
	TaskID    string
	RelPath   string
	SHA256    string
	Writer    string
	CreatedAt int64
}

// Migration states of one task part.
const (
	TaskFilesMigrationRunning = "running"
	TaskFilesMigrationDone    = "done"
	TaskFilesMigrationFailed  = "failed"
)

// TaskFilesMigration is one row of task_files_migration: the marker of one task's export of one part.
type TaskFilesMigration struct {
	TaskID     string
	Part       string
	State      string
	DoneAt     int64
	VerifyJSON string
}
