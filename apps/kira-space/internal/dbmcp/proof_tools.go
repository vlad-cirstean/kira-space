package dbmcp

import (
	"context"
	"encoding/json"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kirathecat/kira-studio/internal/llmprompts"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ProofStore is dbmcp's consumer-declared interface over *proof.Service: the five proof_* tools'
// only backend. They touch no connection, so the permission modes and the approval broker do not
// apply to them.
type ProofStore interface {
	StartSession(name, summary string) (proof.StartResult, error)
	EditSession(name, reason string) (proof.StartResult, error)
	AddStep(in proof.StepInput) (int, error)
	CloseSession(draftID string) (proof.CloseResult, error)
	ListForAgent() ([]proof.AgentSession, error)
}

type proofStartArgs struct {
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
}

type proofEditArgs struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type proofAddStepArgs struct {
	DraftID         string         `json:"draftId"`
	Description     string         `json:"description,omitempty"`
	Query           string         `json:"query,omitempty"`
	Language        string         `json:"language,omitempty"`
	Source          string         `json:"source,omitempty"`
	View            string         `json:"view,omitempty"`
	Data            map[string]any `json:"data,omitempty"`
	KeepOrdinal     *int           `json:"keepOrdinal,omitempty"`
	ReplacesOrdinal *int           `json:"replacesOrdinal,omitempty"`
}

type proofCloseArgs struct {
	DraftID string `json:"draftId"`
}

type proofListArgs struct{}

// proofResult maps a proof error to a tool error: a caller-correctable one is shown to the agent
// as is, anything else is an internal fault.
func proofResult(v any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		if proof.IsUserError(err) {
			return errResult(err.Error())
		}
		return nil, nil, err
	}
	return jsonResult(v)
}

func (s *Server) proofStart(_ context.Context, _ *mcp.CallToolRequest, a proofStartArgs) (*mcp.CallToolResult, any, error) {
	r, err := s.cfg.Proof.StartSession(a.Name, a.Summary)
	return proofResult(r, err)
}

func (s *Server) proofEdit(_ context.Context, _ *mcp.CallToolRequest, a proofEditArgs) (*mcp.CallToolResult, any, error) {
	r, err := s.cfg.Proof.EditSession(a.Name, a.Reason)
	return proofResult(r, err)
}

// proofAddStep reads data from the raw request rather than the decoded args: the SDK decodes
// numbers to float64 and maps to sorted keys, which would lose integer precision and key order.
func (s *Server) proofAddStep(_ context.Context, req *mcp.CallToolRequest, a proofAddStepArgs) (*mcp.CallToolResult, any, error) {
	var raw struct {
		Data json.RawMessage `json:"data"`
	}
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &raw); err != nil {
			return errResult("arguments are not valid JSON")
		}
	}
	ord, err := s.cfg.Proof.AddStep(proof.StepInput{
		DraftID: a.DraftID, Description: a.Description, Query: a.Query, Language: a.Language, Source: a.Source, View: a.View,
		Data: raw.Data, KeepOrdinal: a.KeepOrdinal, ReplacesOrdinal: a.ReplacesOrdinal,
	})
	return proofResult(map[string]int{"ordinal": ord}, err)
}

func (s *Server) proofClose(_ context.Context, _ *mcp.CallToolRequest, a proofCloseArgs) (*mcp.CallToolResult, any, error) {
	r, err := s.cfg.Proof.CloseSession(a.DraftID)
	return proofResult(r, err)
}

func (s *Server) proofList(_ context.Context, _ *mcp.CallToolRequest, _ proofListArgs) (*mcp.CallToolResult, any, error) {
	r, err := s.cfg.Proof.ListForAgent()
	return proofResult(r, err)
}

// addProofTools registers the five proof_* tools next to the database tools.
func (s *Server) addProofTools(srv *mcp.Server, src llmprompts.Source) {
	add := func(name string, entry llmprompts.Prompt[llmprompts.NoVars], sch *jsonschema.Schema) *mcp.Tool {
		return &mcp.Tool{Name: name, Description: entry.Text(src, llmprompts.NoVars{}), InputSchema: sch}
	}
	startSch := llmprompts.RequireNonEmpty(llmprompts.InputSchema[proofStartArgs](src, llmprompts.DbmcpToolProofStartSession.ID(), nil), "name")
	mcp.AddTool(srv, add("proof_start_session", llmprompts.DbmcpToolProofStartSession, startSch), withPanicRecovery(s, "proof_start_session", s.proofStart))

	editSch := llmprompts.RequireNonEmpty(llmprompts.InputSchema[proofEditArgs](src, llmprompts.DbmcpToolProofEditSession.ID(), nil), "name", "reason")
	mcp.AddTool(srv, add("proof_edit_session", llmprompts.DbmcpToolProofEditSession, editSch), withPanicRecovery(s, "proof_edit_session", s.proofEdit))

	stepSch := llmprompts.RequireNonEmpty(llmprompts.InputSchema[proofAddStepArgs](src, llmprompts.DbmcpToolProofAddStep.ID(), nil), "draftId")
	stepSch.Properties["view"].Enum = []any{proof.ViewTable, proof.ViewDocument, proof.ViewKeyValue, proof.ViewChart, proof.ViewDiagram}
	stepSch.Properties["language"].Enum = []any{"sql", "json", "shell", "text"}
	stepSch.Properties["keepOrdinal"].Minimum = jsonschema.Ptr(1.0)
	stepSch.Properties["replacesOrdinal"].Minimum = jsonschema.Ptr(1.0)
	mcp.AddTool(srv, add("proof_add_step", llmprompts.DbmcpToolProofAddStep, stepSch), withPanicRecovery(s, "proof_add_step", s.proofAddStep))

	closeSch := llmprompts.RequireNonEmpty(llmprompts.InputSchema[proofCloseArgs](src, llmprompts.DbmcpToolProofCloseSession.ID(), nil), "draftId")
	mcp.AddTool(srv, add("proof_close_session", llmprompts.DbmcpToolProofCloseSession, closeSch), withPanicRecovery(s, "proof_close_session", s.proofClose))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "proof_list_sessions", Description: llmprompts.DbmcpToolProofListSessions.Text(src, llmprompts.NoVars{}),
	}, withPanicRecovery(s, "proof_list_sessions", s.proofList))
}
