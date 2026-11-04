package updates

import "encoding/json"

// Version is the snapshot layout. A stored snapshot of another version is never diffed.
const Version = 1

// CRSnap is what is observed of a branch's change request. Present false = the branch has none.
// CommentCount and Assignees are nil when the host does not say.
type CRSnap struct {
	V              int      `json:"v"`
	Present        bool     `json:"present"`
	State          string   `json:"state,omitempty"` // open | draft | merged | closed
	Title          string   `json:"title,omitempty"`
	HeadSha        string   `json:"headSha,omitempty"`
	ReviewDecision string   `json:"reviewDecision,omitempty"`
	ChecksState    string   `json:"checksState,omitempty"`
	CommentCount   *int     `json:"commentCount,omitempty"`
	Assignees      []string `json:"assignees,omitempty"`
}

// IssueSnap is what is observed of a task's linked issue. Comments and Linked are nil when the
// tracker does not say.
type IssueSnap struct {
	V        int    `json:"v"`
	Title    string `json:"title,omitempty"`
	Status   string `json:"status,omitempty"`
	Priority string `json:"priority,omitempty"`
	Assignee string `json:"assignee,omitempty"` // tracker account id
	Comments *int   `json:"comments,omitempty"`
	Linked   *int   `json:"linked,omitempty"`
}

// BranchSnap is what is observed of a task branch. ConflictsKnown is false until a conflict check
// has finished.
type BranchSnap struct {
	V              int               `json:"v"`
	Tip            string            `json:"tip,omitempty"`
	Behind         int               `json:"behind,omitempty"`
	UpstreamBehind int               `json:"upstreamBehind,omitempty"`
	Merged         bool              `json:"merged,omitempty"`
	ConflictsKnown bool              `json:"conflictsKnown,omitempty"`
	Conflicts      []string          `json:"conflicts,omitempty"`
	Deployments    map[string]string `json:"deployments,omitempty"` // environment -> status
}

// Encode returns the stored form, stamped with the current Version.
func (s CRSnap) Encode() string { s.V = Version; return marshal(s) }

// Encode returns the stored form, stamped with the current Version.
func (s IssueSnap) Encode() string { s.V = Version; return marshal(s) }

// Encode returns the stored form, stamped with the current Version.
func (s BranchSnap) Encode() string { s.V = Version; return marshal(s) }

// marshal cannot fail: every snapshot field is a string, number, bool, slice or string map.
func marshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// decode fills v from stored; ok is false when the JSON is unreadable or of another version.
func decode[T any](stored string, v *T, version func(T) int) bool {
	if err := json.Unmarshal([]byte(stored), v); err != nil {
		return false
	}
	return version(*v) == Version
}
