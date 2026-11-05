package reviewflow_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const wait = 20 * time.Second

type m = map[string]any

func refsChanged(gs *flowharness.GitStream) int {
	n := 0
	for _, e := range gs.Events("repo.changed") {
		var p struct {
			Kind string `json:"kind"`
		}
		if raw, ok := e.Data.(json.RawMessage); ok && json.Unmarshal(raw, &p) == nil && p.Kind == "refsChanged" {
			n++
		}
	}
	return n
}

// external runs a git change made outside the app and waits for the watcher's refsChanged to reach
// gs: ref and tip lookups read a cache the signal invalidates, so the next request would race it.
func external(t *testing.T, gs *flowharness.GitStream, fn func()) {
	t.Helper()
	before := refsChanged(gs)
	fn()
	testx.WaitUntil(t, wait, func() bool { return refsChanged(gs) > before })
}

// badRequest asserts err is E_BAD_REQUEST (user input) or E_CONTRACT (caller bug) naming field.
func badRequest(t *testing.T, gs *flowharness.GitStream, method string, params any, field string) {
	t.Helper()
	err := gs.Request(method, params, nil)
	we, ok := err.(*flowharness.WireError)
	if !ok {
		t.Fatalf("%s: err = %v, want a wire error", method, err)
	}
	if (we.Code != "E_BAD_REQUEST" && we.Code != "E_CONTRACT") || !strings.Contains(we.Message, field) {
		t.Fatalf("%s: error = %+v, want E_BAD_REQUEST or E_CONTRACT naming %q", method, we, field)
	}
}

func addComment(t *testing.T, gs *flowharness.GitStream, id, branch, path, at string, r gitreview.LineRange, body string) gitsession.CommentEntry {
	t.Helper()
	return call[gitrpc.ReviewCommentAddResult](t, gs, "review.comment.add", gitrpc.ReviewCommentAddParams{
		RepoID: id, Branch: branch, Path: path, At: at, Range: r, Body: body,
	}).Comment
}

func listComments(t *testing.T, gs *flowharness.GitStream, id, branch string) []gitsession.CommentEntry {
	t.Helper()
	return call[gitsession.CommentListResult](t, gs, "review.comment.list", gitrpc.ReviewCommentListParams{RepoID: id, Branch: branch}).Comments
}

func removeComment(t *testing.T, gs *flowharness.GitStream, id, branch string, commentID string) bool {
	t.Helper()
	return call[gitrpc.ReviewCommentRemoveResult](t, gs, "review.comment.remove", gitrpc.ReviewCommentRemoveParams{RepoID: id, Branch: branch, ID: commentID}).Removed
}

func markFile(t *testing.T, gs *flowharness.GitStream, id, branch, path string, reviewed bool, ranges []gitreview.LineRange) gitsession.ReviewFileStatus {
	t.Helper()
	return call[gitrpc.ReviewMarkResult](t, gs, "review.mark", gitrpc.ReviewMarkParams{
		RepoID: id, Branch: branch, Path: path, Reviewed: reviewed, Ranges: ranges,
	}).Review
}

func reviewFiles(t *testing.T, gs *flowharness.GitStream, id, branch, base string) []gitsession.ReviewFileEntry {
	t.Helper()
	return call[gitsession.RangeFilesResult](t, gs, "review.files", gitrpc.ReviewFilesParams{RepoID: id, Branch: branch, Base: base}).Files
}

func fileEntry(t *testing.T, files []gitsession.ReviewFileEntry, path string) gitsession.ReviewFileEntry {
	t.Helper()
	for _, f := range files {
		if f.Change.Path == path {
			return f
		}
	}
	t.Fatalf("review.files has no %s", path)
	return gitsession.ReviewFileEntry{}
}

func sinceReview(t *testing.T, gs *flowharness.GitStream, id, branch, path string) gitsession.ReviewFileDiffResult {
	t.Helper()
	return call[gitsession.ReviewFileDiffResult](t, gs, "review.fileDiff", gitrpc.ReviewFileDiffParams{
		RepoID: id, Branch: branch, Base: "main", Path: path, Mode: "sinceReview",
	})
}

// twoFileFeature is main with a.txt (5 lines) and b.txt (3 lines), then feature adding a line 6.
func twoFileFeature(app *flowharness.App) (repo *flowharness.Repo, featureSha string) {
	repo = app.NewRepo("proj")
	repo.Commit("base commit", map[string]string{"a.txt": "line1\nline2\nline3\nline4\nline5\n", "b.txt": "alpha\nbeta\ngamma\n"})
	repo.Git("checkout", "-q", "-b", "feature")
	featureSha = repo.Commit("feature commit", map[string]string{"a.txt": "line1\nline2\nline3\nline4\nline5\nline6\n"})
	return repo, featureSha
}
