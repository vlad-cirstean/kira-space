package githost

import "context"

// NewComment is one comment to post.
type NewComment struct {
	Body    string // Markdown, 1..MaxHostCommentBytes
	ReplyTo string // a Thread.ID; "" posts a new top-level comment
}

// ChangeRequestComments is optional: a provider the PR view can post to. Separate from
// ChangeRequestDetails (read) and ChangeRequestReviews (review window). Failure is in Status, never an error.
type ChangeRequestComments interface {
	ReplyKinds(r Repo) []string // Thread kinds a reply may target
	PostChangeRequestComment(ctx context.Context, r Repo, number int, c NewComment) Status
}

// LineComment is a new comment on lines of one file at a change request's head.
type LineComment struct {
	Path      string // repo-relative, forward slashes
	Start     int    // 1-based, inclusive; lines of the file at CommitSHA
	End       int
	EndColumn int    // 1-based UTF-16 column after End's last character (hosts that anchor by column)
	CommitSHA string // the host's current head
	Body      string // Markdown, 1..MaxHostCommentBytes
}

// ChangeRequestLineComments is optional: a provider that comments on file lines of a change request
// (right side). Failure is in Status, never an error.
type ChangeRequestLineComments interface {
	CommentOnLines(ctx context.Context, r Repo, number int, c LineComment) Status
}
