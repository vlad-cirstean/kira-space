package dbmcp

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/llmprompts"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ProofStore is dbmcp's consumer-declared interface over *proof.Service: the two proof tools' only
// backend. They touch no connection, so the permission modes and the approval broker do not apply.
type ProofStore interface {
	Save(in proof.SaveInput) (proof.CloseResult, error)
	ListForAgent() ([]proof.AgentSession, error)
	StepsForAgent(name string) ([]proof.StepBrief, error)
}

// ProofAttacher shows a saved proof in a Kira Space task's notes; *ade.TaskBoard implements it.
type ProofAttacher interface {
	AttachProof(ctx context.Context, call agentmcp.SpaceCall, name string) (agentmcp.AttachProofResult, error)
}

type proofSaveStep struct {
	Description     string         `json:"description,omitempty"`
	Query           string         `json:"query,omitempty"`
	Language        string         `json:"language,omitempty"`
	Source          string         `json:"source,omitempty"`
	View            string         `json:"view,omitempty"`
	Data            map[string]any `json:"data,omitempty"`
	KeepOrdinal     *int           `json:"keepOrdinal,omitempty"`
	ReplacesOrdinal *int           `json:"replacesOrdinal,omitempty"`
}

type proofSaveArgs struct {
	Name    string          `json:"name"`
	Summary string          `json:"summary,omitempty"`
	Reason  string          `json:"reason,omitempty"`
	Steps   []proofSaveStep `json:"steps"`
}

type proofListArgs struct {
	Name string `json:"name,omitempty"`
}

// proofSaveResult is proof_save's answer: Status is closed or pending_review. Attached is true when the
// proof joined the call's Kira Space task; AttachNote says why it did not.
type proofSaveResult struct {
	proof.CloseResult
	Attached   bool   `json:"attached,omitempty"`
	AttachNote string `json:"attachNote,omitempty"`
}

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

// proofSave reads each step's data from the raw request rather than the decoded args: the SDK decodes
// numbers to float64 and maps to sorted keys, which would lose integer precision and key order.
func (s *Server) proofSave(ctx context.Context, req *mcp.CallToolRequest, a proofSaveArgs) (*mcp.CallToolResult, any, error) {
	var raw struct {
		Steps []struct {
			Data json.RawMessage `json:"data"`
		} `json:"steps"`
	}
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &raw); err != nil {
			return errResult("arguments are not valid JSON")
		}
	}
	in := proof.SaveInput{Name: a.Name, Summary: a.Summary, Reason: a.Reason, Steps: make([]proof.StepInput, len(a.Steps))}
	for i, st := range a.Steps {
		in.Steps[i] = proof.StepInput{
			Description: st.Description, Query: st.Query, Language: st.Language, Source: st.Source, View: st.View,
			KeepOrdinal: st.KeepOrdinal, ReplacesOrdinal: st.ReplacesOrdinal,
		}
		if i < len(raw.Steps) {
			in.Steps[i].Data = raw.Steps[i].Data
		}
	}
	res, err := s.cfg.Proof.Save(in)
	if err != nil {
		return proofResult(nil, err)
	}
	out := proofSaveResult{CloseResult: res}
	if id, ok := SessionFrom(ctx); ok && id.TaskID != "" && s.cfg.ProofAttach != nil && res.Status == "closed" {
		_, aerr := s.cfg.ProofAttach.AttachProof(ctx, agentmcp.SpaceCall{TaskID: id.TaskID, RunID: id.RunID, StepID: id.StepID, SessionID: id.SessionID}, res.Name)
		if aerr != nil {
			slog.Warn("dbmcp: attach proof to task", "scope", "dbmcp", "task", id.TaskID, "err", aerr)
			out.AttachNote = "saved, not attached to the task: " + aerr.Error()
		} else {
			out.Attached = true
		}
	}
	return jsonResult(out)
}

// proofList lists closed proofs, or with name that proof's current steps, the ordinals an edit keeps.
func (s *Server) proofList(_ context.Context, _ *mcp.CallToolRequest, a proofListArgs) (*mcp.CallToolResult, any, error) {
	if a.Name != "" {
		steps, err := s.cfg.Proof.StepsForAgent(a.Name)
		return proofResult(map[string]any{"name": a.Name, "steps": steps}, err)
	}
	r, err := s.cfg.Proof.ListForAgent()
	return proofResult(r, err)
}

// addProofTools registers proof_save and proof_list next to the database tools.
func (s *Server) addProofTools(srv *mcp.Server, src llmprompts.Source) {
	saveSch := llmprompts.RequireNonEmpty(llmprompts.InputSchema[proofSaveArgs](src, llmprompts.DbmcpToolProofSave.ID(), nil), "name", "steps")
	saveSch.Properties["steps"].MinItems = jsonschema.Ptr(1)
	step := saveSch.Properties["steps"].Items
	step.Properties["view"].Enum = []any{proof.ViewTable, proof.ViewDocument, proof.ViewKeyValue, proof.ViewChart, proof.ViewDiagram}
	step.Properties["language"].Enum = []any{"sql", "json", "shell", "text"}
	step.Properties["keepOrdinal"].Minimum = jsonschema.Ptr(1.0)
	step.Properties["replacesOrdinal"].Minimum = jsonschema.Ptr(1.0)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "proof_save", Description: llmprompts.DbmcpToolProofSave.Text(src, llmprompts.NoVars{}), InputSchema: saveSch,
	}, withPanicRecovery(s, "proof_save", s.proofSave))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "proof_list", Description: llmprompts.DbmcpToolProofList.Text(src, llmprompts.NoVars{}),
		InputSchema: llmprompts.InputSchema[proofListArgs](src, llmprompts.DbmcpToolProofList.ID(), nil),
	}, withPanicRecovery(s, "proof_list", s.proofList))
}
