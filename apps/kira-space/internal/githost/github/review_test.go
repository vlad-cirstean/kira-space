package github

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// TestReviewArgv guards the `-F` file-read hazard: a body starting with @ must travel via `-f`.
func TestReviewArgv(t *testing.T) {
	ctx := context.Background()
	sha := strings.Repeat("a", 40)

	runner := &recordingRunner{}
	c := testClient(t, runner)
	if s := c.SubmitReview(ctx, testRepo, 7, "approve", "", sha); !s.OK() {
		t.Fatalf("approve status = %+v", s)
	}
	for _, a := range runner.lastArgs {
		if strings.HasPrefix(a, "body=") {
			t.Fatalf("approve argv has a body field: %v", runner.lastArgs)
		}
	}

	if s := c.SubmitReview(ctx, testRepo, 7, "requestChanges", "@alice fix this", sha); !s.OK() {
		t.Fatalf("request changes status = %+v", s)
	}
	args := runner.lastArgs
	if slices.Contains(args, "-F") {
		t.Fatalf("argv uses -F: %v", args)
	}
	for _, want := range []string{"body=@alice fix this", "event=REQUEST_CHANGES", "commit_id=" + sha} {
		i := slices.Index(args, want)
		if i < 1 || args[i-1] != "-f" {
			t.Fatalf("argv lacks -f %q: %v", want, args)
		}
	}

	if s := c.CommentOnPull(ctx, testRepo, 7, "@bob thanks"); !s.OK() {
		t.Fatalf("comment status = %+v", s)
	}
	args = runner.lastArgs
	n := len(args)
	if args[n-3] != "repos/o/r/issues/7/comments" || args[n-2] != "-f" || args[n-1] != "body=@bob thanks" {
		t.Fatalf("comment argv tail = %v", args)
	}
}

// TestCommentOnLinesArgv guards the line comment fields: line and start_line are typed (`-F`) so GitHub
// reads JSON integers; every user-supplied field stays behind `-f`.
func TestCommentOnLinesArgv(t *testing.T) {
	ctx := context.Background()
	sha := strings.Repeat("a", 40)
	runner := &recordingRunner{}
	c := testClient(t, runner)

	if s := c.CommentOnLines(ctx, testRepo, 7, "src/a.ts", 3, 5, sha, "@alice why?"); !s.OK() {
		t.Fatalf("range status = %+v", s)
	}
	args := runner.lastArgs
	if args[len(args)-1] == "" || !slices.Contains(args, "repos/o/r/pulls/7/comments") {
		t.Fatalf("argv lacks the endpoint: %v", args)
	}
	for _, want := range []string{"body=@alice why?", "path=src/a.ts", "commit_id=" + sha, "side=RIGHT", "start_side=RIGHT"} {
		if i := slices.Index(args, want); i < 1 || args[i-1] != "-f" {
			t.Fatalf("argv lacks -f %q: %v", want, args)
		}
	}
	for _, want := range []string{"line=5", "start_line=3"} {
		if i := slices.Index(args, want); i < 1 || args[i-1] != "-F" {
			t.Fatalf("argv lacks -F %q: %v", want, args)
		}
	}
	if n := strings.Count(strings.Join(args, " "), " -F "); n != 2 {
		t.Fatalf("argv has %d -F fields, want 2: %v", n, args)
	}

	if s := c.CommentOnLines(ctx, testRepo, 7, "src/a.ts", 4, 4, sha, "one line"); !s.OK() {
		t.Fatalf("single line status = %+v", s)
	}
	args = runner.lastArgs
	for _, a := range args {
		if strings.HasPrefix(a, "start_line=") || strings.HasPrefix(a, "start_side=") {
			t.Fatalf("single line argv has %q: %v", a, args)
		}
	}
	if i := slices.Index(args, "line=4"); i < 1 || args[i-1] != "-F" {
		t.Fatalf("argv lacks -F line=4: %v", args)
	}
}
