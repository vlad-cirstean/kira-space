package agentmcp

// MaxReviewReplyBytes bounds one reply; it matches the review comment body bound.
const MaxReviewReplyBytes = 8 << 10

// ReviewGrant names the review comments one round run may answer.
type ReviewGrant struct {
	CommentIDs []string
}

// ReviewReplier stores an agent's answer to one review comment; *ade.TaskBoard implements it.
type ReviewReplier interface {
	Reply(runID string, id string, reply string, resolved bool) error
}
