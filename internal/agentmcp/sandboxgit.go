package agentmcp

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SandboxGrant adds the sandbox git actions to a run. Push allows branch.push; a read-only step
// leaves it off.
type SandboxGrant struct {
	Push bool
}

// RemoteResult is what remote.fetch and branch.push report.
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

func trimRemoteOutput(out string) string {
	out = strings.TrimSpace(out)
	if len(out) > MaxRemoteOutput {
		return out[:MaxRemoteOutput]
	}
	return out
}
