package updates

import (
	"slices"
	"sort"
)

// Change is the difference between two snapshots of one item.
type Change struct {
	// Concepts are the concepts that differ, in catalog order.
	Concepts []string
	// Counts holds the size of a countable concept: commentAdded -> new comments.
	Counts map[string]int
	// Reset is true when either snapshot is unreadable or of another version: the caller sets seen
	// to current instead of reporting a change.
	Reset bool
}

// Diff compares the seen and current stored snapshots of an item of kind.
func Diff(kind, seen, current string) Change {
	var c Change
	switch kind {
	case KindChangeRequest:
		var a, b CRSnap
		if !DecodeCR(seen, &a) || !DecodeCR(current, &b) {
			return Change{Reset: true}
		}
		c = diffCR(a, b)
	case KindIssue:
		var a, b IssueSnap
		if !DecodeIssue(seen, &a) || !DecodeIssue(current, &b) {
			return Change{Reset: true}
		}
		c = diffIssue(a, b)
	case KindBranch:
		var a, b BranchSnap
		if !DecodeBranch(seen, &a) || !DecodeBranch(current, &b) {
			return Change{Reset: true}
		}
		c = diffBranch(a, b)
	default:
		return Change{Reset: true}
	}
	pos := order(kind)
	sort.SliceStable(c.Concepts, func(i, j int) bool { return pos[c.Concepts[i]] < pos[c.Concepts[j]] })
	return c
}

type collector struct{ Change }

func (c *collector) add(id string, fired bool) {
	if fired {
		c.Concepts = append(c.Concepts, id)
	}
}

// grew reports whether both counts are known and the second is larger; delta is the growth.
func grew(a, b *int) (delta int, ok bool) {
	if a == nil || b == nil || *b <= *a {
		return 0, false
	}
	return *b - *a, true
}

func isFailed(checks string) bool { return checks == "FAILURE" || checks == "ERROR" }

func isEnded(state string) bool { return state == "merged" || state == "closed" }

func isLive(state string) bool { return state == "open" || state == "draft" }

func diffCR(a, b CRSnap) Change {
	var c collector
	if !a.Present && b.Present {
		c.add("opened", true)
		return c.Change
	}
	if !a.Present || !b.Present {
		return c.Change
	}
	if a.Number != 0 && b.Number != 0 && a.Number != b.Number {
		// A new change request took the branch's place.
		c.add("opened", true)
		return c.Change
	}
	if d, ok := grew(a.CommentCount, b.CommentCount); ok {
		c.add("commentAdded", true)
		c.Counts = map[string]int{"commentAdded": d}
	}
	c.add("merged", a.State != "merged" && b.State == "merged")
	c.add("closed", a.State != "closed" && b.State == "closed")
	c.add("reopened", isEnded(a.State) && isLive(b.State))
	c.add("readyForReview", a.State == "draft" && b.State == "open")
	c.add("checksFailed", !isFailed(a.ChecksState) && isFailed(b.ChecksState))
	c.add("checksPassed", a.ChecksState != "SUCCESS" && b.ChecksState == "SUCCESS")
	c.add("approved", a.ReviewDecision != "APPROVED" && b.ReviewDecision == "APPROVED")
	c.add("changesRequested", a.ReviewDecision != "CHANGES_REQUESTED" && b.ReviewDecision == "CHANGES_REQUESTED")
	c.add("newCommits", a.HeadSha != "" && b.HeadSha != "" && a.HeadSha != b.HeadSha)
	c.add("titleChanged", a.Title != b.Title)
	c.add("assigneeChanged", a.Assignees != nil && b.Assignees != nil && !sameSet(a.Assignees, b.Assignees))
	return c.Change
}

func sameSet(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

func diffIssue(a, b IssueSnap) Change {
	var c collector
	c.add("statusChanged", a.Status != b.Status)
	if d, ok := grew(a.Comments, b.Comments); ok {
		c.add("commentAdded", true)
		c.Counts = map[string]int{"commentAdded": d}
	}
	c.add("assigneeChanged", a.Assignee != b.Assignee)
	c.add("priorityChanged", a.Priority != b.Priority)
	c.add("titleChanged", a.Title != b.Title)
	c.add("linkedChangeRequests", a.Linked != nil && b.Linked != nil && *a.Linked != *b.Linked)
	return c.Change
}

func diffBranch(a, b BranchSnap) Change {
	var c collector
	c.add("remoteCommits", b.UpstreamBehind > a.UpstreamBehind)
	c.add("merged", !a.Merged && b.Merged)
	c.add("conflicts", a.ConflictsKnown && b.ConflictsKnown && len(a.Conflicts) == 0 && len(b.Conflicts) > 0)
	c.add("deploymentChanged", deploymentsDiffer(a.Deployments, b.Deployments))
	c.add("baseMoved", b.Behind > a.Behind)
	c.add("localCommits", a.Tip != "" && b.Tip != "" && a.Tip != b.Tip)
	return c.Change
}

// deploymentsDiffer compares only the environments present on both sides.
func deploymentsDiffer(a, b map[string]string) bool {
	for env, was := range a {
		if now, ok := b[env]; ok && now != was {
			return true
		}
	}
	return false
}
