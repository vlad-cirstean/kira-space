package githostflow_test

import (
	"fmt"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
)

// J-BC-025: past the shared caps the PR view keeps the newest conversation comments and the first
// checks, flags both truncations, and says nothing is truncated at or under a cap. (There is no paging:
// the host answers one read and the view caps it.)
func TestChangeRequestDetailCaps(t *testing.T) {
	build := func(comments, checks int) githost.ChangeRequestDetail {
		d := githost.ChangeRequestDetail{CreatedAt: 1_700_000_000_000}
		for i := 1; i <= comments; i++ {
			d.Threads = append(d.Threads, githost.Thread{ID: fmt.Sprintf("t%03d", i), Kind: githost.ThreadComment, Comments: []githost.Comment{
				{ID: fmt.Sprintf("c%03d", i), Author: githost.Person{Login: "ann"}, Body: "x", CreatedAt: 1_700_000_000_000 + int64(i)},
			}})
		}
		for i := 1; i <= checks; i++ {
			d.Checks = append(d.Checks, githost.Check{Name: fmt.Sprintf("job-%03d", i), State: githost.CheckSuccess})
		}
		return d
	}
	read := func(comments, checks int) *githost.ChangeRequestDetail {
		forge := fakehost.New()
		forge.AddChangeRequest(githost.ChangeRequest{
			Number: 12, Title: "Big", State: "open", HeadRef: "feat/big", BaseRef: "main", HeadOwner: "acme",
			URL: "https://forge.test/acme/api/-/merge_requests/12", UpdatedAt: 1_700_000_300_000,
		})
		forge.SetAuthor(12, "ann")
		forge.SetDetail(12, build(comments, checks))
		app := flowharness.New(t, flowharness.WithGitHost(forge), flowharness.WithoutGh())
		got := hvDetail(t, app, "https://forge.test/acme/api/-/merge_requests/12")
		if got.Detail == nil {
			t.Fatalf("detail = %+v", got)
		}
		return got.Detail
	}

	over := read(githost.DetailMaxComments+30, githost.DetailMaxChecks+20)
	if len(over.Threads) != githost.DetailMaxComments || !over.ThreadsTruncated {
		t.Fatalf("threads = %d truncated = %v, want %d and true", len(over.Threads), over.ThreadsTruncated, githost.DetailMaxComments)
	}
	if first, last := over.Threads[0].ID, over.Threads[len(over.Threads)-1].ID; first != "t031" || last != "t130" {
		t.Fatalf("kept threads %s..%s, want the newest 100 (t031..t130) oldest first", first, last)
	}
	if len(over.Checks) != githost.DetailMaxChecks || !over.ChecksTruncated || over.Checks[0].Name != "job-001" {
		t.Fatalf("checks = %d truncated = %v first = %q", len(over.Checks), over.ChecksTruncated, over.Checks[0].Name)
	}

	exact := read(githost.DetailMaxComments, githost.DetailMaxChecks)
	if len(exact.Threads) != githost.DetailMaxComments || exact.ThreadsTruncated || len(exact.Checks) != githost.DetailMaxChecks || exact.ChecksTruncated {
		t.Fatalf("at the cap: threads %d/%v checks %d/%v, want nothing truncated", len(exact.Threads), exact.ThreadsTruncated, len(exact.Checks), exact.ChecksTruncated)
	}
}
