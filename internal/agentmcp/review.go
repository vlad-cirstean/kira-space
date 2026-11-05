package agentmcp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

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

type reviewReplyArgs struct {
	ID       string `json:"id"`
	Reply    string `json:"reply"`
	Resolved bool   `json:"resolved"`
}

func (s *Server) addReviewTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "reply_review_comment",
		Description: llmprompts.HeadlessToolReplyReviewComment.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[reviewReplyArgs](s.prompts, llmprompts.HeadlessToolReplyReviewComment.ID(), nil), "reply"),
	}, func(_ context.Context, req *mcp.CallToolRequest, args reviewReplyArgs) (*mcp.CallToolResult, any, error) {
		var g Grant
		var ok bool
		if req.Extra != nil {
			g, ok = s.grant(req.Extra.TokenInfo)
		}
		if !ok || g.RunID == "" || g.Review == nil {
			return toolError("reply_review_comment is not available for this session."), nil, nil
		}
		if !slices.Contains(g.Review.CommentIDs, args.ID) {
			return toolError(fmt.Sprintf("comment %s was not sent to you", args.ID)), nil, nil
		}
		reply := strings.TrimSpace(args.Reply)
		if reply == "" {
			return toolError("reply must not be empty."), nil, nil
		}
		if len(reply) > MaxReviewReplyBytes {
			return toolError(fmt.Sprintf("reply is too long: at most %d bytes", MaxReviewReplyBytes)), nil, nil
		}
		if err := s.review.Reply(g.RunID, args.ID, reply, args.Resolved); err != nil {
			return toolError(err.Error()), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: llmprompts.HeadlessReviewReplyAck.Text(s.prompts, llmprompts.NoVars{})}}}, nil, nil
	})
}
