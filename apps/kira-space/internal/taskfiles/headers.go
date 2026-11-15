package taskfiles

// Header texts. A Kira-created content file starts with one comment line (YAML `#`, bare Markdown
// `<!-- -->`); notes and handovers carry none. An agent-created file may omit it and validation never
// needs it. Kira never rewrites an agent-written file to add one.
const (
	headerLinks = "Kira links. Edit, then call submit (reason when you change another author's link)."
	headerProof = "Kira proof. Edit, then call submit (reason when the proof is accepted). Or call the proof tool."
	headerState = "Read-only. Kira rewrites this file. Change state only with Kira tools: branches, lease, cancel_operation, finish_step; task_info waits for a setup."
	headerTask  = "Read-only. Kira rewrites this file. It mirrors the task definition; change it only in the app."
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
