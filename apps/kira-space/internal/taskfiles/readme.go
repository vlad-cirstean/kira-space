package taskfiles

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// README limits. The file is read at the start of a run, so it stays one screen: at most 35 lines,
// the index lines at most 300 bytes each, no tool list, no proof format.
const (
	MaxReadmeLines     = 35
	maxReadmeLineBytes = 300
	maxReadmeProofs    = 5
	maxReadmeChildren  = 20
	maxReadmeTitle     = 120
)

// ReadmeProof is one proof of the task in the README index.
type ReadmeProof struct {
	Slug, Name string
	Steps      []ReadmeStep
}

// ReadmeStep is one step id and its view.
type ReadmeStep struct {
	ID, View string
}

// ReadmeChild is one child task of an orchestrator in the README index.
type ReadmeChild struct {
	ID, Title string
	// LatestHandover is the child's newest handed file, relative to the child's folder; "" when none.
	LatestHandover string
}

// ReadmeInput is what README.md is generated from. Proofs and Children are the task's own; only an
// orchestrator has children.
type ReadmeInput struct {
	TaskID, Title string
	// Step is the current step id, which names the draft file; "" prints the <step> placeholder.
	Step     string
	Proofs   []ReadmeProof
	Children []ReadmeChild
}

// Readme renders README.md: at most MaxReadmeLines lines, one idea per line. Index lines beyond the
// budget become one "more" line, so the cap holds for any task.
func Readme(in ReadmeInput) string {
	step := in.Step
	if step == "" {
		step = "<step>"
	}
	lines := []string{
		fmt.Sprintf("# Task %s: %s", oneLine(in.TaskID, maxReadmeTitle), oneLine(in.Title, maxReadmeTitle)),
		"Edit, then call submit: a change counts only after submit answers per file. Pass reason when the change needs the user's approval.",
		"Read-only, edits undone: README.md, task.yaml, state.yaml (step, branches, setup, leases, runs, children).",
		fmt.Sprintf("Write: handovers/draft-%s.md (your stage's handover, Markdown, <=16 KiB). links.yaml. proofs/<slug>/proof.md.", oneLine(step, 80)),
		"Ask first (submit with reason): notes/*.md (user notes), handovers/NNNN-*.md (handed), others' links, accepted proofs.",
	}
	lines = append(lines, proofLines(in.Proofs)...)
	room := MaxReadmeLines - len(lines)
	lines = append(lines, childLines(in.Children, room)...)
	return strings.Join(lines, "\n") + "\n"
}

func proofLines(proofs []ReadmeProof) []string {
	if len(proofs) == 0 {
		return []string{"Proofs: none yet."}
	}
	var out []string
	for i, p := range proofs {
		if i == maxReadmeProofs {
			out = append(out, fmt.Sprintf("  and %d more under proofs/", len(proofs)-maxReadmeProofs))
			break
		}
		steps := make([]string, len(p.Steps))
		for j, s := range p.Steps {
			steps[j] = fmt.Sprintf("%s (%s)", s.ID, s.View)
		}
		out = append(out, indexLine(i, "Proofs:", fmt.Sprintf("%s %q: %s", p.Slug, oneLine(p.Name, 80), strings.Join(steps, ", "))))
	}
	return out
}

// childLines lists the children in at most room lines; the last line counts what did not fit.
func childLines(children []ReadmeChild, room int) []string {
	if len(children) == 0 || room <= 0 {
		return nil
	}
	shown := min(len(children), maxReadmeChildren, room)
	if shown < len(children) {
		shown = max(shown-1, 0) // one line goes to the "more" marker
	}
	out := make([]string, 0, shown+1)
	for i, c := range children[:shown] {
		text := fmt.Sprintf("%s %q ../%s/", c.ID, oneLine(c.Title, 80), c.ID)
		if c.LatestHandover != "" {
			text += fmt.Sprintf(" (latest handover: %s)", c.LatestHandover)
		}
		out = append(out, indexLine(i, "Children:", text))
	}
	if shown < len(children) {
		out = append(out, fmt.Sprintf("  and %d more, see state.yaml chain", len(children)-shown))
	}
	return out
}

// indexLine is one line of an index: the label on the first line, an indent after, cut to the byte cap.
func indexLine(i int, label, text string) string {
	prefix := "  "
	if i == 0 {
		prefix = label + " "
	}
	return capBytes(prefix+text, maxReadmeLineBytes)
}

// oneLine collapses whitespace and control characters to single spaces and cuts to max runes.
func oneLine(s string, max int) string {
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)
		s = string(r[:max-1]) + "…"
	}
	return s
}

// capBytes cuts s to at most max bytes at a rune boundary, ending in an ellipsis when it cut.
func capBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max - len("…")
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
