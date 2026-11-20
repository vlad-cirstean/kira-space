package taskfiles

// Header texts. A Kira-created content file starts with one comment line (YAML `#`, bare Markdown
// `<!-- -->`); notes and handovers carry none. An agent-created file may omit it and validation never
// needs it. Kira never rewrites an agent-written file to add one.
const (
	headerLinks = "Kira links. Kira applies edits on save; editing another agent's link waits for the user's approval."
	headerProof = "Kira proof. Kira applies edits on save; editing another agent's older proof waits for the user's approval."
	headerState = "Read-only. Kira rewrites this file. State changes: call state. status is done, pending (the user must answer), waiting (Kira or a queue is on it), denied or failed (see note)."
	headerTask  = "Read-only. Kira rewrites this file. State changes: call state."
)

// Header returns the comment line a Kira-created file of kind starts with, newline included, or "".
// For a proof it belongs inside the frontmatter, right after the opening `---`.
func Header(k Kind) string {
	switch k {
	case KindLinks:
		return "# " + headerLinks + "\n"
	case KindProof:
		return "# " + headerProof + "\n"
	case KindState:
		return "# " + headerState + "\n"
	case KindTask:
		return "# " + headerTask + "\n"
	}
	return ""
}
