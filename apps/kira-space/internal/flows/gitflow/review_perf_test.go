package gitflow_test

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// TestReviewPerf (P739) times the git calls one review window makes on a PR-sized branch: boot
// (resolveBase, files, comment list), the name-status spawn behind review.files with and without
// copy detection, marks (one, 100, half the files) and one file open (fileDiff both modes, two
// file.read). Prints `REVIEWPERF` lines; asserts nothing. KIRA_REVIEW_PERF_FILES sizes the branch
// (default 3000 changed files).
func TestReviewPerf(t *testing.T) {
	skipUnlessPerf(t)
	n := 3000
	if v := os.Getenv("KIRA_REVIEW_PERF_FILES"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
			t.Fatalf("KIRA_REVIEW_PERF_FILES=%q: %v", v, err)
		}
	}
	r := newRig(t)
	repo := r.app.NewRepo("reviewperf")

	body := func(seed, lines int) string {
		var b strings.Builder
		for i := 0; i < lines; i++ {
			fmt.Fprintf(&b, "line %d of file %d: some code here\n", i, seed)
		}
		return b.String()
	}
	base := map[string]string{"big/huge.go": body(0, 20000)}
	for i := 0; i < n; i++ {
		base[fmt.Sprintf("pkg%02d/sub%02d/file%04d.go", i%40, i%7, i)] = body(i, 200)
	}
	repo.Commit("base", base)
	repo.Git("checkout", "-q", "-b", "feat")
	// Ten commits, each editing a tenth of the files plus a slice of the huge one.
	for c := 0; c < 10; c++ {
		edits := map[string]string{}
		for i := c; i < n; i += 10 {
			edits[fmt.Sprintf("pkg%02d/sub%02d/file%04d.go", i%40, i%7, i)] = strings.Replace(body(i, 200), "line 50 ", fmt.Sprintf("line 50 edited %d ", c), 1)
		}
		huge := body(0, 20000)
		for k := 0; k <= c; k++ {
			huge = strings.Replace(huge, fmt.Sprintf("line %d of", k*1500), fmt.Sprintf("LINE %d of", k*1500), 1)
		}
		edits["big/huge.go"] = huge
		edits[fmt.Sprintf("new/added%02d.go", c)] = body(c, 300)
		repo.Commit(fmt.Sprintf("feat %d", c), edits)
	}
	tip := strings.TrimSpace(repo.Git("rev-parse", "HEAD"))
	repo.Checkout("main")
	id := r.open(repo.Dir).RepoID

	timed := func(label string, fn func() int) time.Duration {
		start := time.Now()
		bytes := fn()
		d := time.Since(start)
		t.Logf("REVIEWPERF %s ms=%.1f bytes=%d", label, float64(d.Microseconds())/1000, bytes)
		return d
	}
	size := func(v any) int { b, _ := json.Marshal(v); return len(b) }

	timed("resolveBase", func() int {
		return size(call[gitreview.BaseResolution](t, r.gs, "review.resolveBase", gitrpc.ReviewResolveBaseParams{RepoID: id, Branch: "feat"}))
	})
	var files gitsession.RangeFilesResult
	timed("files.cold", func() int {
		files = call[gitsession.RangeFilesResult](t, r.gs, "review.files", gitrpc.ReviewFilesParams{RepoID: id, Branch: "feat", Base: "main"})
		return size(files)
	})
	t.Logf("REVIEWPERF files.count=%d", len(files.Files))
	for _, args := range [][]string{
		{"diff-tree", "-r", "--no-commit-id", "--name-status", "-M", "-C", "-z", files.MergeBase, tip},
		{"diff-tree", "-r", "--no-commit-id", "--name-status", "-M", "-z", files.MergeBase, tip},
		{"diff-tree", "-r", "--no-commit-id", "--name-status", "-z", files.MergeBase, tip},
	} {
		timed("git."+strings.Join(args[4:len(args)-3], ""), func() int { return len(repo.Git(args...)) })
	}
	timed("files.warm", func() int {
		return size(call[gitsession.RangeFilesResult](t, r.gs, "review.files", gitrpc.ReviewFilesParams{RepoID: id, Branch: "feat", Base: "main"}))
	})

	// Half the files reviewed, then 40 comments spread over files.
	var half []string
	for i, f := range files.Files {
		if i%2 == 0 {
			half = append(half, f.Change.Path)
		}
	}
	timed("mark.one", func() int {
		return size(call[gitrpc.ReviewMarkResult](t, r.gs, "review.mark", gitrpc.ReviewMarkParams{RepoID: id, Branch: "feat", Path: half[1], Reviewed: true}))
	})
	timed("markMany.100", func() int {
		return size(call[gitrpc.ReviewMarkManyResult](t, r.gs, "review.markMany", gitrpc.ReviewMarkManyParams{RepoID: id, Branch: "feat", Paths: half[:100], Reviewed: true}))
	})
	timed("markMany.half", func() int {
		return size(call[gitrpc.ReviewMarkManyResult](t, r.gs, "review.markMany", gitrpc.ReviewMarkManyParams{RepoID: id, Branch: "feat", Paths: half, Reviewed: true}))
	})
	for i := 0; i < 40; i++ {
		p := files.Files[i*len(files.Files)/40].Change.Path
		call[gitrpc.ReviewCommentAddResult](t, r.gs, "review.comment.add", gitrpc.ReviewCommentAddParams{
			RepoID: id, Branch: "feat", Path: p, At: tip, Range: gitreview.LineRange{Start: 51, End: 51}, Body: "why?",
		})
	}
	timed("files.reviewedHalf", func() int {
		return size(call[gitsession.RangeFilesResult](t, r.gs, "review.files", gitrpc.ReviewFilesParams{RepoID: id, Branch: "feat", Base: "main"}))
	})
	var comments []time.Duration
	for i := 0; i < 5; i++ {
		comments = append(comments, timed("comment.list", func() int {
			return size(call[gitsession.CommentListResult](t, r.gs, "review.comment.list", gitrpc.ReviewCommentListParams{RepoID: id, Branch: "feat", At: tip}))
		}))
	}

	open := func(label, path string) {
		for _, mode := range []string{"sinceReview", "range"} {
			timed(label+".fileDiff."+mode, func() int {
				return size(call[gitsession.ReviewFileDiffResult](t, r.gs, "review.fileDiff", gitrpc.ReviewFileDiffParams{RepoID: id, Branch: "feat", Base: "main", Path: path, Mode: mode}))
			})
		}
		for _, rev := range []string{files.MergeBase, tip} {
			timed(label+".file.read", func() int {
				var out json.RawMessage
				r.gs.MustRequest("file.read", gitrpc.FileReadParams{RepoID: id, Rev: rev, Path: path}, &out)
				return len(out)
			})
		}
	}
	paths := make([]string, 0, len(files.Files))
	for _, f := range files.Files {
		paths = append(paths, f.Change.Path)
	}
	sort.Strings(paths)
	open("small.unreviewed", paths[1])
	open("small.reviewed", half[3])
	open("huge", "big/huge.go")
}
