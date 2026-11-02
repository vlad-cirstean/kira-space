package gitsession

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"unicode/utf16"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
)

// ChangeRequestReviewState is changeRequest.reviewState's result. Kind is "ok" | "disabled" |
// "none" | "unavailable".
type ChangeRequestReviewState struct {
	Kind                                                string
	Status                                              *githost.Status
	Host                                                githost.Info
	Number                                              int
	Title, State, HeadSha, LocalHeadSha, Author, Viewer string
	Own                                                 bool
}

// ChangeRequestPost is the result of a review or comment post. Kind is "ok" | "disabled" | "unavailable".
type ChangeRequestPost struct {
	Kind   string
	Status *githost.Status
}

func (e *RepoEntry) reviewHost(ctx context.Context) (githost.Provider, githost.ChangeRequestReviews, githost.Repo, bool) {
	p, repo, ok := e.hostRepo(ctx)
	if !ok {
		return nil, nil, githost.Repo{}, false
	}
	rv, ok := p.(githost.ChangeRequestReviews)
	if !ok {
		return nil, nil, githost.Repo{}, false
	}
	return p, rv, repo, true
}

func reviewUnavailable(status githost.Status) ChangeRequestReviewState {
	s := status
	return ChangeRequestReviewState{Kind: "unavailable", Status: &s}
}

// hostHeadName is the head name the host knows branch by: an origin remote-tracking spelling
// ("origin/feat/x") loses its remote prefix; any other spelling is returned as is.
func (e *RepoEntry) hostHeadName(ctx context.Context, branch string) (string, error) {
	snapshot, err := e.Refs(ctx)
	if err != nil {
		return "", err
	}
	for _, r := range snapshot.Branches {
		if r.ShortName == branch {
			return branch, nil
		}
	}
	for _, r := range snapshot.RemoteBranches {
		if r.ShortName == branch {
			if head, ok := strings.CutPrefix(r.Refname, "refs/remotes/origin/"); ok {
				return head, nil
			}
			break
		}
	}
	return branch, nil
}

// ChangeRequestReviewState reads the review facts of a change request, fresh from the host.
// number > 0 names it; otherwise it is the one found for branch by its host head name.
func (e *RepoEntry) ChangeRequestReviewState(ctx context.Context, branch string, number int) (ChangeRequestReviewState, error) {
	p, rv, repo, ok := e.reviewHost(ctx)
	if !ok {
		return ChangeRequestReviewState{Kind: "disabled"}, nil
	}
	title := ""
	if number <= 0 {
		head, err := e.hostHeadName(ctx, branch)
		if err != nil {
			return ChangeRequestReviewState{}, err
		}
		lookup := e.ResolveBranchChangeRequest(ctx, head)
		switch lookup.Kind {
		case "disabled":
			return ChangeRequestReviewState{Kind: "disabled"}, nil
		case "unavailable":
			return ChangeRequestReviewState{Kind: "unavailable", Status: lookup.Status}, nil
		}
		if len(lookup.ChangeRequests) == 0 {
			return ChangeRequestReviewState{Kind: "none"}, nil
		}
		number, title = lookup.ChangeRequests[0].Number, lookup.ChangeRequests[0].Title
	}

	if status, armed := e.host.breakerStatus(); armed {
		return reviewUnavailable(status), nil
	}
	t, status := rv.ReviewTarget(ctx, repo, number)
	if !status.OK() {
		if status.RateLimited {
			e.host.armBreaker(status)
		}
		return reviewUnavailable(status), nil
	}
	if t.Title != "" {
		title = t.Title
	}

	local, err := e.branchTip(ctx, branch)
	if err != nil && !errors.Is(err, ErrBranchNotFound) {
		return ChangeRequestReviewState{}, err
	}
	if err != nil {
		local = ""
	}
	viewer := p.Account(ctx, repo)
	return ChangeRequestReviewState{
		Kind: "ok", Host: githost.Describe(p), Number: number, Title: title, State: t.State,
		HeadSha: t.HeadSha, LocalHeadSha: local, Author: t.Author, Viewer: viewer,
		Own: t.Author != "" && strings.EqualFold(t.Author, viewer),
	}, nil
}

// SubmitChangeRequestReview posts a review on change request number.
func (e *RepoEntry) SubmitChangeRequestReview(ctx context.Context, number int, review githost.Review) ChangeRequestPost {
	return e.post(ctx, func(rv githost.ChangeRequestReviews, repo githost.Repo) githost.Status {
		return rv.SubmitReview(ctx, repo, number, review)
	})
}

// CommentOnChangeRequest posts a conversation comment on change request number.
func (e *RepoEntry) CommentOnChangeRequest(ctx context.Context, number int, body string) ChangeRequestPost {
	return e.post(ctx, func(rv githost.ChangeRequestReviews, repo githost.Repo) githost.Status {
		return rv.Comment(ctx, repo, number, body)
	})
}

func (e *RepoEntry) post(ctx context.Context, fn func(githost.ChangeRequestReviews, githost.Repo) githost.Status) ChangeRequestPost {
	_, rv, repo, ok := e.reviewHost(ctx)
	if !ok {
		return ChangeRequestPost{Kind: "disabled"}
	}
	if status, armed := e.host.breakerStatus(); armed {
		return ChangeRequestPost{Kind: "unavailable", Status: &status}
	}
	return e.postResult(ctx, fn(rv, repo))
}

// postResult maps a host write's status: success syncs the board, a rate limit arms the breaker.
func (e *RepoEntry) postResult(ctx context.Context, status githost.Status) ChangeRequestPost {
	if status.OK() {
		// Non-waiting: board badges pick up the new review decision within the sync gates.
		e.SyncChangeRequests(ctx, nil, false)
		return ChangeRequestPost{Kind: "ok"}
	}
	if status.RateLimited {
		e.host.armBreaker(status)
	}
	return ChangeRequestPost{Kind: "unavailable", Status: &status}
}

// ChangeRequestLinePost is a line comment post's result: ChangeRequestPost's kinds plus
// "headMoved" (the host head is not headSha; HeadSha then holds the host head).
type ChangeRequestLinePost struct {
	Kind    string
	Status  *githost.Status
	HeadSha string
}

// CommentOnChangeRequestLines posts a comment on lines of path at the change request's head. The
// host head is read fresh and must equal headSha, the commit the caller's line numbers refer to.
func (e *RepoEntry) CommentOnChangeRequestLines(ctx context.Context, number int, headSha, path string, r gitreview.LineRange, body string) (ChangeRequestLinePost, error) {
	_, rv, repo, ok := e.reviewHost(ctx)
	if !ok {
		return ChangeRequestLinePost{Kind: "disabled"}, nil
	}
	lc, ok := rv.(githost.ChangeRequestLineComments)
	if !ok {
		return ChangeRequestLinePost{Kind: "disabled"}, nil
	}
	if status, armed := e.host.breakerStatus(); armed {
		return ChangeRequestLinePost{Kind: "unavailable", Status: &status}, nil
	}
	t, status := rv.ReviewTarget(ctx, repo, number)
	if !status.OK() {
		if status.RateLimited {
			e.host.armBreaker(status)
		}
		return ChangeRequestLinePost{Kind: "unavailable", Status: &status}, nil
	}
	if t.HeadSha != headSha {
		return ChangeRequestLinePost{Kind: "headMoved", HeadSha: t.HeadSha}, nil
	}
	kind, content, lineCount, _, err := e.readSnapshotSource(ctx, headSha, path)
	if err != nil {
		return ChangeRequestLinePost{}, err
	}
	if kind != gitreview.ContentText {
		return ChangeRequestLinePost{}, ErrCommentNotText
	}
	if r.End > lineCount {
		return ChangeRequestLinePost{}, ErrCommentRangeOutOfFile
	}
	post := e.postResult(ctx, lc.CommentOnLines(ctx, repo, number, githost.LineComment{
		Path: path, Start: r.Start, End: r.End, EndColumn: endColumn(content, r.End), CommitSHA: headSha, Body: body,
	}))
	return ChangeRequestLinePost{Kind: post.Kind, Status: post.Status}, nil
}

// endColumn is the 1-based UTF-16 column after the last character of 1-based line n of content.
func endColumn(content []byte, n int) int {
	line := content
	for i := 1; i < n; i++ {
		_, rest, found := bytes.Cut(line, []byte("\n"))
		if !found {
			break
		}
		line = rest
	}
	line, _, _ = bytes.Cut(line, []byte("\n"))
	line = bytes.TrimSuffix(line, []byte("\r"))
	return len(utf16.Encode([]rune(string(line)))) + 1
}
