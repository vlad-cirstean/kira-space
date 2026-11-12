package agentmcp

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// SandboxGrant adds the sandbox git tools to a run. Push allows push_branch; a read-only step
// leaves it off.
type SandboxGrant struct {
	Push bool
}

// RemoteResult is what fetch_remote and push_branch report.
type RemoteResult struct {
	Remote string `json:"remote"`
	Branch string `json:"branch"`
	Output string `json:"output"`
}

// SandboxGit runs the remote operations in the app, outside the sandbox; *ade.TaskBoard implements
// it. Both act on the run's own repo and task branch, never on a caller-named path or branch.
type SandboxGit interface {
	FetchRemote(ctx context.Context, runID, remote string) (RemoteResult, error)
	PushBranch(ctx context.Context, runID string, force bool) (RemoteResult, error)
}

type fetchRemoteArgs struct {
	Remote string `json:"remote,omitempty"`
}

type pushBranchArgs struct {
	Force bool `json:"force,omitempty"`
}

// MaxRemoteOutput bounds the git output a tool returns.
const MaxRemoteOutput = 8 << 10

// remoteName bounds the remote argument before it reaches git.
func remoteName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "origin"
	}
	return s
}

func (s *Server) runGrant(req *mcp.CallToolRequest) (Grant, bool) {
	if req.Extra == nil {
		return Grant{}, false
	}
	g, ok := s.grant(req.Extra.TokenInfo)
	return g, ok && g.RunID != "" && g.Sandbox != nil
}

func (s *Server) addSandboxGitTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fetch_remote",
		Description: llmprompts.HeadlessToolFetchRemote.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.InputSchema[fetchRemoteArgs](s.prompts, llmprompts.HeadlessToolFetchRemote.ID(), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args fetchRemoteArgs) (*mcp.CallToolResult, RemoteResult, error) {
		g, ok := s.runGrant(req)
		if !ok {
			return toolError("fetch_remote is not available for this session."), RemoteResult{}, nil
		}
		remote := remoteName(args.Remote)
		if len(remote) > 255 || strings.HasPrefix(remote, "-") {
			return toolError("remote must be a remote name of your repo."), RemoteResult{}, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		res, err := s.sandbox.FetchRemote(ctx, g.RunID, remote)
		if err != nil {
			return fail("fetch_remote", err), RemoteResult{}, nil
		}
		res.Output = trimRemoteOutput(res.Output)
		return nil, res, nil
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "push_branch",
		Description: llmprompts.HeadlessToolPushBranch.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.InputSchema[pushBranchArgs](s.prompts, llmprompts.HeadlessToolPushBranch.ID(), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args pushBranchArgs) (*mcp.CallToolResult, RemoteResult, error) {
		g, ok := s.runGrant(req)
		if !ok {
			return toolError("push_branch is not available for this session."), RemoteResult{}, nil
		}
		if !g.Sandbox.Push {
			return toolError("push is not allowed in a read-only step"), RemoteResult{}, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		res, err := s.sandbox.PushBranch(ctx, g.RunID, args.Force)
		if err != nil {
			return fail("push_branch", err), RemoteResult{}, nil
		}
		res.Output = trimRemoteOutput(res.Output)
		return nil, res, nil
	})
}

func trimRemoteOutput(out string) string {
	out = strings.TrimSpace(out)
	if len(out) > MaxRemoteOutput {
		return out[:MaxRemoteOutput]
	}
	return out
}
