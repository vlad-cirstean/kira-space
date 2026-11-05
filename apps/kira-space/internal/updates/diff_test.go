package updates

import (
	"slices"
	"testing"
)

func n(v int) *int { return &v }

func TestDiff(t *testing.T) {
	open := CRSnap{Present: true, State: "open", CommentCount: n(1), Assignees: []string{"a"}}
	with := func(s CRSnap, f func(*CRSnap)) CRSnap { f(&s); return s }
	cases := []struct {
		name      string
		kind      string
		seen, cur string
		want      []string
		counts    map[string]int
		wantReset bool
	}{
		{"cr unchanged", KindChangeRequest, open.Encode(), open.Encode(), nil, nil, false},
		{"cr opened", KindChangeRequest, CRSnap{}.Encode(), open.Encode(), []string{"opened"}, nil, false},
		{"cr replaced", KindChangeRequest, with(open, func(s *CRSnap) { s.Number = 1; s.State = "closed" }).Encode(),
			with(open, func(s *CRSnap) { s.Number = 2 }).Encode(), []string{"opened"}, nil, false},
		{"cr vanished", KindChangeRequest, open.Encode(), CRSnap{}.Encode(), nil, nil, false},
		{"comments grow", KindChangeRequest, open.Encode(), with(open, func(s *CRSnap) { s.CommentCount = n(3) }).Encode(),
			[]string{"commentAdded"}, map[string]int{"commentAdded": 2}, false},
		{"comments shrink", KindChangeRequest, open.Encode(), with(open, func(s *CRSnap) { s.CommentCount = n(0) }).Encode(), nil, nil, false},
		{"comments unknown to known", KindChangeRequest, with(open, func(s *CRSnap) { s.CommentCount = nil }).Encode(),
			with(open, func(s *CRSnap) { s.CommentCount = n(5) }).Encode(), nil, nil, false},
		{"open to merged", KindChangeRequest, open.Encode(), with(open, func(s *CRSnap) { s.State = "merged" }).Encode(), []string{"merged"}, nil, false},
		{"open to closed", KindChangeRequest, open.Encode(), with(open, func(s *CRSnap) { s.State = "closed" }).Encode(), []string{"closed"}, nil, false},
		{"merged to open", KindChangeRequest, with(open, func(s *CRSnap) { s.State = "merged" }).Encode(), open.Encode(), []string{"reopened"}, nil, false},
		{"draft to open", KindChangeRequest, with(open, func(s *CRSnap) { s.State = "draft" }).Encode(), open.Encode(), []string{"readyForReview"}, nil, false},
		{"open to draft", KindChangeRequest, open.Encode(), with(open, func(s *CRSnap) { s.State = "draft" }).Encode(), nil, nil, false},
		{"checks failed", KindChangeRequest, with(open, func(s *CRSnap) { s.ChecksState = "PENDING" }).Encode(),
			with(open, func(s *CRSnap) { s.ChecksState = "FAILURE" }).Encode(), []string{"checksFailed"}, nil, false},
		{"failure to error", KindChangeRequest, with(open, func(s *CRSnap) { s.ChecksState = "FAILURE" }).Encode(),
			with(open, func(s *CRSnap) { s.ChecksState = "ERROR" }).Encode(), nil, nil, false},
		{"failed then passed reads passed only", KindChangeRequest, open.Encode(),
			with(open, func(s *CRSnap) { s.ChecksState = "SUCCESS" }).Encode(), []string{"checksPassed"}, nil, false},
		{"review decisions", KindChangeRequest, with(open, func(s *CRSnap) { s.ReviewDecision = "REVIEW_REQUIRED" }).Encode(),
			with(open, func(s *CRSnap) { s.ReviewDecision = "APPROVED" }).Encode(), []string{"approved"}, nil, false},
		{"changes requested", KindChangeRequest, open.Encode(),
			with(open, func(s *CRSnap) { s.ReviewDecision = "CHANGES_REQUESTED" }).Encode(), []string{"changesRequested"}, nil, false},
		{"assignees reordered", KindChangeRequest, with(open, func(s *CRSnap) { s.Assignees = []string{"a", "b"} }).Encode(),
			with(open, func(s *CRSnap) { s.Assignees = []string{"b", "a"} }).Encode(), nil, nil, false},
		{"assignee changed", KindChangeRequest, open.Encode(), with(open, func(s *CRSnap) { s.Assignees = []string{"b"} }).Encode(), []string{"assigneeChanged"}, nil, false},
		{"catalog order", KindChangeRequest, open.Encode(),
			with(open, func(s *CRSnap) { s.State, s.ChecksState, s.CommentCount = "merged", "SUCCESS", n(2) }).Encode(),
			[]string{"commentAdded", "merged", "checksPassed"}, map[string]int{"commentAdded": 1}, false},
		{"issue status and unknown comments", KindIssue, IssueSnap{Status: "To Do"}.Encode(),
			IssueSnap{Status: "Done", Comments: n(4)}.Encode(), []string{"statusChanged"}, nil, false},
		{"issue comments", KindIssue, IssueSnap{Comments: n(1)}.Encode(), IssueSnap{Comments: n(2)}.Encode(),
			[]string{"commentAdded"}, map[string]int{"commentAdded": 1}, false},
		{"issue linked unknown", KindIssue, IssueSnap{}.Encode(), IssueSnap{Linked: n(1)}.Encode(), nil, nil, false},
		{"issue linked known", KindIssue, IssueSnap{Linked: n(0)}.Encode(), IssueSnap{Linked: n(1)}.Encode(), []string{"linkedChangeRequests"}, nil, false},
		{"branch remote commits", KindBranch, BranchSnap{UpstreamBehind: 1}.Encode(), BranchSnap{UpstreamBehind: 3}.Encode(), []string{"remoteCommits"}, nil, false},
		{"branch merged", KindBranch, BranchSnap{}.Encode(), BranchSnap{Merged: true}.Encode(), []string{"merged"}, nil, false},
		{"conflicts both known", KindBranch, BranchSnap{ConflictsKnown: true}.Encode(),
			BranchSnap{ConflictsKnown: true, Conflicts: []string{"a.go"}}.Encode(), []string{"conflicts"}, nil, false},
		{"conflicts check pending", KindBranch, BranchSnap{}.Encode(),
			BranchSnap{ConflictsKnown: true, Conflicts: []string{"a.go"}}.Encode(), nil, nil, false},
		{"conflicts already present", KindBranch, BranchSnap{ConflictsKnown: true, Conflicts: []string{"a.go"}}.Encode(),
			BranchSnap{ConflictsKnown: true, Conflicts: []string{"a.go", "b.go"}}.Encode(), nil, nil, false},
		{"deployment status", KindBranch, BranchSnap{Deployments: map[string]string{"prod": "stale"}}.Encode(),
			BranchSnap{Deployments: map[string]string{"prod": "deployed"}}.Encode(), []string{"deploymentChanged"}, nil, false},
		{"new environment", KindBranch, BranchSnap{Deployments: map[string]string{"prod": "stale"}}.Encode(),
			BranchSnap{Deployments: map[string]string{"prod": "stale", "dev": "deployed"}}.Encode(), nil, nil, false},
		{"base moved and local commits", KindBranch, BranchSnap{Tip: "a", Behind: 1}.Encode(), BranchSnap{Tip: "b", Behind: 2}.Encode(),
			[]string{"baseMoved", "localCommits"}, nil, false},
		{"version mismatch", KindChangeRequest, `{"v":0,"present":true}`, open.Encode(), nil, nil, true},
		{"unreadable", KindIssue, `not json`, IssueSnap{}.Encode(), nil, nil, true},
		{"unknown kind", "other", `{}`, `{}`, nil, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Diff(tc.kind, tc.seen, tc.cur)
			if got.Reset != tc.wantReset {
				t.Fatalf("reset = %v, want %v", got.Reset, tc.wantReset)
			}
			if !slices.Equal(got.Concepts, tc.want) {
				t.Fatalf("concepts = %v, want %v", got.Concepts, tc.want)
			}
			for k, v := range tc.counts {
				if got.Counts[k] != v {
					t.Fatalf("count %s = %d, want %d", k, got.Counts[k], v)
				}
			}
		})
	}
}
