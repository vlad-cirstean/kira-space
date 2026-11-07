package agentmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// Artifact limits, shared by the tool, the engine and the bridge.
const (
	MaxArtifactsPerCall = 20
	MaxArtifactsPerTask = 200
	MaxArtifactTitle    = 200
	MaxArtifactURL      = 2048
	MaxArtifactRef      = 512
	// maxArtifactsReply caps the artifacts a finishing reply lists.
	maxArtifactsReply = 100
	// artifactsReplyTimeout bounds the listing a finishing reply waits for.
	artifactsReplyTimeout = 10 * time.Second
)

// Artifact kinds an agent or the user may report. Detected kinds (branch) are never reported.
const (
	ArtifactChangeRequest = "changeRequest"
	ArtifactIssue         = "issue"
	ArtifactDeployment    = "deployment"
	ArtifactDoc           = "doc"
	ArtifactLink          = "link"
)

// ArtifactProof is the report_artifacts kind that attaches a closed proof session by name (ref). It is
// not stored as an artifact, so ReportableArtifactKind refuses it.
const ArtifactProof = "proof"

// ReportableArtifactKind reports whether a kind may be reported.
func ReportableArtifactKind(kind string) bool {
	switch kind {
	case ArtifactChangeRequest, ArtifactIssue, ArtifactDeployment, ArtifactDoc, ArtifactLink:
		return true
	}
	return false
}

// ArtifactInput is one artifact an agent reports: a url (http or https) or a ref (a file path, a
// name), plus a title.
type ArtifactInput struct {
	Kind  string `json:"kind"`
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
	Ref   string `json:"ref,omitempty"`
}

// Artifact is one artifact as an agent reads it.
type Artifact struct {
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	URL    string `json:"url,omitempty"`
	Ref    string `json:"ref,omitempty"`
	Status string `json:"status,omitempty"`
	Repo   string `json:"repo,omitempty"`
	Source string `json:"source"`
}

// ArtifactGap names a source whose artifacts are missing from the list.
type ArtifactGap struct {
	Source string `json:"source"`
	Repo   string `json:"repo,omitempty"`
	// Reason says why a tracker source is partial (unauthenticated, timeout, ...).
	Reason string `json:"reason,omitempty"`
}

// TaskArtifacts is every artifact of a task; Gaps lists the sources that failed, so the list is partial.
type TaskArtifacts struct {
	Artifacts []Artifact    `json:"artifacts"`
	Gaps      []ArtifactGap `json:"gaps"`
}

// ReportArtifactsResult is report_artifacts' answer.
type ReportArtifactsResult struct {
	Recorded int `json:"recorded"`
	// Proofs lists the proof sessions attached to the task by kind proof items.
	Proofs []AttachProofResult `json:"proofs,omitempty"`
}

// CleanArtifact trims an input and returns it, or the refusal text when it breaks a rule.
func CleanArtifact(in ArtifactInput) (ArtifactInput, string) {
	return CleanArtifactAs(in, ReportableArtifactKind)
}

// CleanArtifactAs is CleanArtifact with the caller's own set of allowed kinds.
func CleanArtifactAs(in ArtifactInput, kindOK func(string) bool) (ArtifactInput, string) {
	in.Kind, in.Title = strings.TrimSpace(in.Kind), strings.TrimSpace(in.Title)
	in.URL, in.Ref = strings.TrimSpace(in.URL), strings.TrimSpace(in.Ref)
	switch {
	case !kindOK(in.Kind):
		return in, "unknown kind; use one the tool lists."
	case in.Title == "":
		return in, "title must not be empty."
	case len(in.Title) > MaxArtifactTitle:
		return in, fmt.Sprintf("title is too long: at most %d bytes.", MaxArtifactTitle)
	case in.URL == "" && in.Ref == "":
		return in, "give a url or a ref."
	case len(in.URL) > MaxArtifactURL:
		return in, fmt.Sprintf("url is too long: at most %d bytes.", MaxArtifactURL)
	case len(in.Ref) > MaxArtifactRef:
		return in, fmt.Sprintf("ref is too long: at most %d bytes.", MaxArtifactRef)
	}
	if in.URL != "" {
		u, err := url.Parse(in.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return in, "url must be an absolute http or https address."
		}
	}
	return in, ""
}

// artifactsReply is the text block a finishing call appends: the task's artifacts as compact JSON
// under a heading. A failure becomes a one-line note; the finish itself is never failed by it.
func (s *Server) artifactsReply(ctx context.Context, req *mcp.CallToolRequest, taskID string) mcp.Content {
	ctx, done := s.callCtx(ctx, req)
	defer done()
	ctx, cancel := context.WithTimeout(ctx, artifactsReplyTimeout)
	defer cancel()
	list, err := s.space.ListArtifacts(ctx, taskID)
	if err != nil {
		slog.Warn("agentmcp: list artifacts", "scope", "ade", "task", taskID, "err", err)
		return &mcp.TextContent{Text: llmprompts.HeadlessArtifactsUnavailable.Text(s.prompts, llmprompts.NoVars{})}
	}
	more := 0
	if n := len(list.Artifacts); n > maxArtifactsReply {
		more, list.Artifacts = n-maxArtifactsReply, list.Artifacts[:maxArtifactsReply]
	}
	if list.Artifacts == nil {
		list.Artifacts = []Artifact{}
	}
	if list.Gaps == nil {
		list.Gaps = []ArtifactGap{}
	}
	body, err := json.Marshal(list)
	if err != nil {
		slog.Warn("agentmcp: encode artifacts", "scope", "ade", "task", taskID, "err", err)
		return &mcp.TextContent{Text: llmprompts.HeadlessArtifactsUnavailable.Text(s.prompts, llmprompts.NoVars{})}
	}
	text := llmprompts.HeadlessArtifactsHeader.Text(s.prompts, llmprompts.NoVars{}) + "\n" + string(body)
	if more > 0 {
		text += "\n" + llmprompts.HeadlessArtifactsMore.Text(s.prompts, llmprompts.CountVars{Count: more})
	}
	return &mcp.TextContent{Text: text}
}

// finishContent is a finishing call's reply: the ack, then the task's artifacts when the grant is a
// Kira Space task and Space tools are wired.
func (s *Server) finishContent(ctx context.Context, req *mcp.CallToolRequest, ack string, g Grant) []mcp.Content {
	out := []mcp.Content{&mcp.TextContent{Text: ack}}
	if g.Space && g.TaskID != "" && s.space != nil {
		out = append(out, s.artifactsReply(ctx, req, g.TaskID))
	}
	return out
}
