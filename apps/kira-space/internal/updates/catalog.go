// Package updates decides which changes to a tracked item count as updates. A tracked item (a
// change request, an issue, a branch) keeps a seen snapshot and a current one; Diff names the
// concepts that differ. Which concepts the user wants flagged is a setting, applied by the renderer.
package updates

// Item kinds.
const (
	KindChangeRequest = "changeRequest"
	KindIssue         = "issue"
	KindBranch        = "branch"
)

// Concept is one kind of change and whether it is flagged until the user says otherwise.
type Concept struct {
	ID      string
	Default bool
}

// KindConcepts lists one item kind's concepts in display order.
type KindConcepts struct {
	Kind     string
	Concepts []Concept
}

// Catalog is the single source of the trackable concepts, mirrored by the renderer's
// state/trackingConcepts.ts.
var Catalog = []KindConcepts{
	{KindChangeRequest, []Concept{
		{"opened", true},
		{"commentAdded", true},
		{"merged", true},
		{"closed", true},
		{"reopened", true},
		{"readyForReview", true},
		{"checksFailed", true},
		{"checksPassed", false},
		{"approved", true},
		{"changesRequested", true},
		{"newCommits", false},
		{"titleChanged", false},
		{"assigneeChanged", false},
	}},
	{KindIssue, []Concept{
		{"statusChanged", true},
		{"commentAdded", true},
		{"assigneeChanged", false},
		{"priorityChanged", false},
		{"titleChanged", false},
		{"linkedChangeRequests", false},
	}},
	{KindBranch, []Concept{
		{"remoteCommits", true},
		{"merged", true},
		{"conflicts", true},
		{"deploymentChanged", true},
		{"baseMoved", false},
		{"localCommits", false},
	}},
}

// Concepts returns kind's concepts; nil for an unknown kind.
func Concepts(kind string) []Concept {
	for _, k := range Catalog {
		if k.Kind == kind {
			return k.Concepts
		}
	}
	return nil
}

// Known reports whether id is a concept of kind.
func Known(kind, id string) bool {
	for _, c := range Concepts(kind) {
		if c.ID == id {
			return true
		}
	}
	return false
}

// order returns the catalog position of each concept of kind.
func order(kind string) map[string]int {
	cs := Concepts(kind)
	out := make(map[string]int, len(cs))
	for i, c := range cs {
		out[c.ID] = i
	}
	return out
}
