package githost

import "context"

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
