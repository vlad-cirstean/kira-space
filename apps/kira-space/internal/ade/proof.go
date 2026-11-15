package ade

import (
	"context"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles/store"
)

// proof.go: proofs of tasks (P643 Part 4). A proof is a folder proofs/<slug>/ in the task folder; the
// proof_* rows cache it and the proof service owns the review windows and prompts.

// ProofLinks is the proof service the board reads and notifies; *proof.Service implements it.
type ProofLinks interface {
	TaskProofs(taskID string) ([]proof.TaskProof, error)
	Blobs() *store.Blobs
	LegacyProofs(taskID string) ([]repos.LegacyProof, error)
	PurgeTaskless()
	SessionOf(taskID, slug string) (string, bool, error)
	LinkOf(sessionID string) (taskID, slug string, ok bool, err error)
	RaiseReady(id string)
	RaiseReview(id string)
	WithdrawReview(id string)
	SetOnEditResolved(fn func(sessionID string, accepted bool) error)
	SetOnDelete(fn func(taskID, slug string) error)
}

func (b *TaskBoard) proofLinks() ProofLinks { return b.deps.Proofs }

// TaskProofs lists the proofs of a task. Archived tasks work too.
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
			steps[j] = adewire.TaskProofStep{Ordinal: st.Ordinal, ID: st.ID, Description: st.Description, View: st.View, Source: st.Source}
		}
		out[i] = adewire.TaskProof{ProofID: r.ID, Name: r.Name, Summary: r.Summary, ClosedAt: r.ClosedAt, LinkedAt: r.LinkedAt,
			Slug: r.Slug, SessionID: r.KiraSessionID, StepID: r.StepID, StepName: r.StepName, Rev: r.Rev, Pending: r.Pending, Steps: steps}
	}
	return out, nil
}

// proofIndex is the task's proofs as the README and the prompt list them.
func (b *TaskBoard) proofIndex(taskID string) []taskfiles.ReadmeProof {
	links := b.proofLinks()
	if links == nil {
		return nil
	}
	rows, err := links.TaskProofs(taskID)
	if err != nil {
		slog.Warn("ade: list proofs for the index", "scope", "ade", "task", taskID, "err", err)
		return nil
	}
	out := make([]taskfiles.ReadmeProof, 0, len(rows))
	for _, r := range rows {
		if r.Slug == "" {
			continue
		}
		p := taskfiles.ReadmeProof{Slug: r.Slug, Name: r.Name}
		for _, st := range r.Steps {
			p.Steps = append(p.Steps, taskfiles.ReadmeStep{ID: st.ID, View: st.View})
		}
		out = append(out, p)
	}
	return out
}

// proofEditResolved makes the files follow the user's decision on a pending edit: approve accepts the
// files at the edit's bytes, deny writes the accepted copy back.
func (b *TaskBoard) proofEditResolved(sessionID string, accepted bool) error {
	taskID, slug, ok, err := b.proofLinks().LinkOf(sessionID)
	if err != nil || !ok || b.files == nil {
		return err
	}
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	ctx := withDetailsSession(b.ctxWithTask(b.ctx, taskID), detailsSession{role: taskfiles.RoleUser})
	if accepted {
		err = b.files.svc.AcceptProofFiles(taskID, slug, string(taskfiles.RoleUser))
	} else {
		err = b.files.svc.RestoreProof(ctx, taskID, slug)
	}
	if err != nil {
		return err
	}
	b.mirrorTask(taskID, userMirror)
	b.notifyBoardNow(taskChanged(taskID))
	return nil
}

// ProofReview raises or withdraws the review of a proof's pending edit (taskfiles.ProofEvents).
func (e detailsEvents) ProofReview(_ context.Context, taskID, slug string, pending bool) error {
	links := e.b.proofLinks()
	if links == nil {
		return nil
	}
	id, ok, err := links.SessionOf(taskID, slug)
	if err != nil || !ok {
		return err
	}
	if pending {
		links.RaiseReview(id)
	} else {
		links.WithdrawReview(id)
	}
	return nil
}

// proofReady raises the ready popup of a new proof and records it for the panel forward.
func (b *TaskBoard) proofReady(taskID, rel string) {
	links := b.proofLinks()
	c := taskfiles.Classify(rel)
	if links == nil || c.Slug == "" {
		return
	}
	id, ok, err := links.SessionOf(taskID, c.Slug)
	if err != nil || !ok {
		return
	}
	links.RaiseReady(id)
	if p := b.proofOf(taskID, id); p != nil {
		b.recordPanelEdit(taskID, "proof:"+id, panelItemKindProof, "proof", "", proofText(p.Name, p.Summary))
	}
}

// proofDelete is the user's delete of a proof: the folder and every row of it go.
func (b *TaskBoard) proofDelete(taskID, slug string) error {
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	if err := b.files.svc.DeleteProof(b.ctxWithTask(b.ctx, taskID), taskID, slug); err != nil {
		return err
	}
	b.mirrorTask(taskID, userMirror)
	b.notifyBoardNow(taskChanged(taskID))
	return nil
}
