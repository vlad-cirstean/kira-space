package taskfiles

import (
	"strings"

	"github.com/aymanbagabas/go-udiff"
)

const diffContext = 3

// TextDiff is the unified diff of two texts as the approval and the disk banner show it; "" when equal.
func TextDiff(name, before, after string) string {
	if before == after {
		return ""
	}
	if before != "" && !strings.HasSuffix(before, "\n") {
		before += "\n"
	}
	if after != "" && !strings.HasSuffix(after, "\n") {
		after += "\n"
	}
	out, err := udiff.ToUnified("a/"+name, "b/"+name, before, udiff.Lines(before, after), diffContext)
	if err != nil {
		return ""
	}
	return out
}

// Change is one semantic difference between the accepted copy of a file and its new content.
type Change struct {
	// Op is what the change does to its target.
	Op Op
	// Key names the target: a link entry key, a step id, or "" for a whole-file text change.
	Key string
	// Slot is true for a Jira or PR slot of links.yaml.
	Slot bool
	// Author is who wrote the target, from the writing session's view; the handler sets it.
	Author Author
	// Latest is true when the target is the newest note or handed handover of the task.
	Latest bool
	// Field names a changed top-level field of a keyed target ("name", "summary"); "" for the target itself.
	Field string
	// Text is the unified diff of a text change, or a one-line label for a keyed one.
	Text string
}

// LinkEntry is one entry of links.yaml in its cleaned form: URL normalized, kinds and caps checked.
type LinkEntry struct {
	Kind, Title, URL, Ref string
}

// Key is the identity of an entry: its normalized URL, else its kind and ref.
func (e LinkEntry) Key() string {
	if e.URL != "" {
		return e.URL
	}
	return e.Kind + "\x00" + e.Ref
}

// Links is the content of links.yaml.
type Links struct {
	// Jira is the Jira slot as key and URL; both empty when unset.
	JiraKey, JiraURL string
	// PR is the pull request URL; empty when unset.
	PR      string
	Entries []LinkEntry
}

// DiffLinks lists the entry and slot changes from before to after, in file order of the new content
// followed by removals in the order of the old content. An entry whose key stays and whose kind or
// title or ref changed is an edit.
func DiffLinks(before, after Links) []Change {
	var out []Change
	out = append(out, diffSlot("jira", before.JiraKey+"\n"+before.JiraURL, after.JiraKey+"\n"+after.JiraURL, before.JiraKey+before.JiraURL == "", after.JiraKey+after.JiraURL == "")...)
	out = append(out, diffSlot("pr", before.PR, after.PR, before.PR == "", after.PR == "")...)
	old := make(map[string]LinkEntry, len(before.Entries))
	for _, e := range before.Entries {
		old[e.Key()] = e
	}
	seen := make(map[string]bool, len(after.Entries))
	for _, e := range after.Entries {
		k := e.Key()
		seen[k] = true
		prev, ok := old[k]
		switch {
		case !ok:
			out = append(out, Change{Op: OpCreate, Key: k, Text: "add " + e.Title})
		case prev != e:
			out = append(out, Change{Op: OpEdit, Key: k, Text: "edit " + prev.Title})
		}
	}
	for _, e := range before.Entries {
		if !seen[e.Key()] {
			out = append(out, Change{Op: OpDelete, Key: e.Key(), Text: "remove " + e.Title})
		}
	}
	return out
}

func diffSlot(key, before, after string, emptyBefore, emptyAfter bool) []Change {
	switch {
	case before == after:
		return nil
	case emptyBefore:
		return []Change{{Op: OpCreate, Key: key, Slot: true, Text: "set " + key}}
	case emptyAfter:
		return []Change{{Op: OpDelete, Key: key, Slot: true, Text: "clear " + key}}
	}
	return []Change{{Op: OpEdit, Key: key, Slot: true, Text: "replace " + key}}
}

// Step is the identity and content digest of one proof step; the caller hashes the normalized fields.
type Step struct {
	ID, Hash string
}

// ProofHead is the proof-level fields that can change beside its steps.
type ProofHead struct {
	Name, Summary string
	Steps         []Step
}

// DiffProof compares two proofs by step id: the same id and hash is unchanged, the same id with
// another hash is an edit, a new id is a create, a missing id is a delete. Name and summary changes
// come first as edits of the proof itself. A reorder of unchanged steps is a change of the proof
// ("order"), because the window shows steps in file order.
func DiffProof(before, after ProofHead) []Change {
	var out []Change
	if before.Name != after.Name {
		out = append(out, Change{Op: OpEdit, Field: "name", Text: "name: " + before.Name + " to " + after.Name})
	}
	if before.Summary != after.Summary {
		out = append(out, Change{Op: OpEdit, Field: "summary", Text: "summary changed"})
	}
	old := make(map[string]string, len(before.Steps))
	for _, s := range before.Steps {
		old[s.ID] = s.Hash
	}
	seen := make(map[string]bool, len(after.Steps))
	var keptOrder []string
	for _, s := range after.Steps {
		seen[s.ID] = true
		h, ok := old[s.ID]
		switch {
		case !ok:
			out = append(out, Change{Op: OpCreate, Key: s.ID, Text: "add step " + s.ID})
		case h != s.Hash:
			out = append(out, Change{Op: OpEdit, Key: s.ID, Text: "edit step " + s.ID})
			keptOrder = append(keptOrder, s.ID)
		default:
			keptOrder = append(keptOrder, s.ID)
		}
	}
	var oldOrder []string
	for _, s := range before.Steps {
		if seen[s.ID] {
			oldOrder = append(oldOrder, s.ID)
		} else {
			out = append(out, Change{Op: OpDelete, Key: s.ID, Text: "remove step " + s.ID})
		}
	}
	if !sameOrder(oldOrder, keptOrder) {
		out = append(out, Change{Op: OpEdit, Field: "order", Text: "steps reordered"})
	}
	return out
}

func sameOrder(a, b []string) bool {
	return strings.Join(a, "\x00") == strings.Join(b, "\x00")
}
