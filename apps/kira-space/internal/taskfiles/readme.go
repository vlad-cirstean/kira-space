package taskfiles

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// README limits. The file is read at the start of a run, so it stays one screen: at most 35 lines,
// the index lines at most 300 bytes each, no tool list, one proof example block.
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

// proofExample is the one proof format block: 8 lines, each at most 300 bytes. It rides the README the
// agent reads anyway, so a proof needs no tool and no extra file.
var proofExample = []string{
	"New proof: slug a-z0-9-; write images and big data into proofs/<slug>/ first, then proof.md last. Example:",
	"---",
	"name: Orders match ledger",
	"summary: All 20 orders match their ledger lines.",
	"steps:  # id a-z0-9- (keep it on edits); view table|document|keyvalue|chart|diagram|image; inline: data (<=64 KiB) or data: file here (.json .png .jpg .webp .svg)",
	"  - {id: totals, description: Totals agree, view: table, inline: {columns: [{name: id}, {name: total}], rows: [[1, 1900]]}, query: \"select id, total from orders\", language: sql}",
	"  - {id: shot, description: Report page, view: image, data: shot.png}",
	"---",
}

// Readme renders README.md: at most MaxReadmeLines lines, one idea per line. Index lines beyond the
// budget become one "more" line, so the cap holds for any task.
func Readme(in ReadmeInput) string {
	step := in.Step
	if step == "" {
		step = "<step>"
	}
	lines := append(make([]string, 0, MaxReadmeLines), []string{
		fmt.Sprintf("# Task %s: %s", oneLine(in.TaskID, maxReadmeTitle), oneLine(in.Title, maxReadmeTitle)),
		"Kira applies your edits on save; read state.yaml fileErrors after each edit.",
		"Read-only, Kira rewrites: README.md, task.yaml, state.yaml (step, repos, setup, leases, runs, children). State changes: call state.",
		fmt.Sprintf("Write: handovers/draft-%s.md (your stage's handover, Markdown, <=16 KiB). links.yaml. proofs/<slug>/proof.md (format below).", oneLine(step, 80)),
		"Another agent's notes, handovers, proofs and links: editing an older one or deleting any waits for the user's approval; Kira asks.",
		"Write a proof for any checkable result before finishing.",
	}...)
	lines = append(lines, proofExample...)
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
		out = append(out, indexLine(i, "Proofs:", ProofSummary(p)))
	}
	return out
}

// ProofSummary is the index text of one proof: its folder, name and steps with their views.
func ProofSummary(p ReadmeProof) string {
	steps := make([]string, len(p.Steps))
	for j, s := range p.Steps {
		steps[j] = fmt.Sprintf("%s (%s)", s.ID, s.View)
	}
	return fmt.Sprintf("%s %q: %s", p.Slug, oneLine(p.Name, 80), strings.Join(steps, ", "))
}

// PromptProofLines are the proof lines of a prompt: at most maxReadmeProofs, each cut to the byte cap.
func PromptProofLines(proofs []ReadmeProof) []string {
	var out []string
	for i, p := range proofs {
		if i == maxReadmeProofs {
			break
		}
		out = append(out, capBytes(ProofSummary(p), maxReadmeLineBytes-len("Proofs: ")))
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
