package ade

import (
	"context"
	"errors"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
)

// proof.go: proof sessions attached to tasks (P543). The links live in the Studio DB; the board
// guards every write with the Space DB's task (it exists and is not archived).

// ProofLinks is the proof service the board attaches sessions through; *proof.Service implements it.
type ProofLinks interface {
	AttachByName(name string, l proof.TaskLink) (proof.TaskProof, error)
	AttachByID(id string, l proof.TaskLink) (proof.TaskProof, error)
	Detach(id, taskID string) error
	TaskProofs(taskID string) ([]proof.TaskProof, error)
	Unattached() ([]proof.Summary, error)
}

type proofHolder struct{ links ProofLinks }

// SetProofs binds the proof service. The board is built before the Studio stage, so appwire calls
// this once that stage exists.
func (b *TaskBoard) SetProofs(p ProofLinks) { b.proofs.Store(&proofHolder{links: p}) }

func (b *TaskBoard) proofLinks() ProofLinks {
	if h := b.proofs.Load(); h != nil {
		return h.links
	}
	return nil
}

// proofUserError turns a proof refusal into an input error the bridge reports; other errors pass.
func proofUserError(err error) error {
	var pe *repos.ProofError
	if errors.As(err, &pe) {
		return invalid("%s", pe.Msg)
	}
	var ie *proof.InvalidError
	if errors.As(err, &ie) {
		return invalid("%s", ie.Msg)
	}
	return err
}

// toolRefusal makes an input error the agent can read; other errors pass for a generic failure.
func toolRefusal(err error) error {
	if errors.Is(err, ErrInvalidInput) {
		return agentmcp.ToolError(trimInvalid(err))
	}
	return err
}

// proofLink resolves the stage a link is written in.
func (b *TaskBoard) proofLink(taskID, source, sessionID string) (proof.TaskLink, error) {
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return proof.TaskLink{}, err
	}
	cur := b.currentStepID(tc)
	return proof.TaskLink{TaskID: taskID, Source: source, KiraSessionID: sessionID, StepID: cur, StepName: stepName(tc, cur)}, nil
}

// AttachProof is attach_proof: a closed session, by name, joins the call's task.
func (b *TaskBoard) AttachProof(_ context.Context, call agentmcp.SpaceCall, name string) (agentmcp.AttachProofResult, error) {
	links := b.proofLinks()
	if links == nil {
		return agentmcp.AttachProofResult{}, agentmcp.ToolError("Proof is not available in this session.")
	}
	mu := b.taskMu(call.TaskID)
	mu.Lock()
	defer mu.Unlock()
	l, err := b.proofLink(call.TaskID, proof.LinkSourceAgent, call.SessionID)
	if err != nil {
		return agentmcp.AttachProofResult{}, toolRefusal(err)
	}
	p, err := links.AttachByName(name, l)
	if err != nil {
		return agentmcp.AttachProofResult{}, toolRefusal(proofUserError(err))
	}
	return agentmcp.AttachProofResult{Name: p.Name, Steps: len(p.Steps), Status: "attached"}, nil
}

// LinkProof attaches a closed session to a task on the user's say.
func (b *TaskBoard) LinkProof(_ context.Context, taskID, proofID string) error {
	links := b.proofLinks()
	if links == nil {
		return invalid("proof is not available")
	}
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	l, err := b.proofLink(taskID, proof.LinkSourceUser, "")
	if err != nil {
		return err
	}
	if _, err := links.AttachByID(proofID, l); err != nil {
		return proofUserError(err)
	}
	return nil
}

// UnlinkProof removes a session from a task.
func (b *TaskBoard) UnlinkProof(_ context.Context, taskID, proofID string) error {
	links := b.proofLinks()
	if links == nil {
		return invalid("proof is not available")
	}
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	if _, err := b.loadTaskCtx(taskID); err != nil {
		return err
	}
	return proofUserError(links.Detach(proofID, taskID))
}

// TaskProofs lists the sessions attached to a task. Archived tasks work too.
func (b *TaskBoard) TaskProofs(_ context.Context, taskID string) ([]adewire.TaskProof, error) {
	if _, err := b.deps.Tasks.GetTask(taskID); err != nil {
		return nil, err
	}
	links := b.proofLinks()
	if links == nil {
		return []adewire.TaskProof{}, nil
	}
	rows, err := links.TaskProofs(taskID)
	if err != nil {
		return nil, err
	}
	out := make([]adewire.TaskProof, len(rows))
	for i, r := range rows {
		steps := make([]adewire.TaskProofStep, len(r.Steps))
		for j, st := range r.Steps {
			steps[j] = adewire.TaskProofStep{Ordinal: st.Ordinal, Description: st.Description, View: st.View, Source: st.Source}
		}
		out[i] = adewire.TaskProof{ProofID: r.ID, Name: r.Name, Summary: r.Summary, ClosedAt: r.ClosedAt, LinkedAt: r.LinkedAt,
			Source: r.Source, SessionID: r.KiraSessionID, StepID: r.StepID, StepName: r.StepName, Rev: r.Rev, Pending: r.Pending, Steps: steps}
	}
	return out, nil
}

// ProofChoices lists the closed sessions no task holds, newest first.
func (b *TaskBoard) ProofChoices(_ context.Context) ([]adewire.ProofChoice, error) {
	links := b.proofLinks()
	if links == nil {
		return []adewire.ProofChoice{}, nil
	}
	rows, err := links.Unattached()
	if err != nil {
		return nil, err
	}
	out := make([]adewire.ProofChoice, len(rows))
	for i, r := range rows {
		out[i] = adewire.ProofChoice{ProofID: r.ID, Name: r.Name, ClosedAt: r.ClosedAt, Steps: r.Steps}
	}
	return out, nil
}
