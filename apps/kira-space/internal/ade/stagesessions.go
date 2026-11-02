package ade

import (
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// stageSessions lists the running interactive sessions a move from `from` to `to` leaves behind:
// those of any other step, plus the review agent when the move leaves the review step. Branch
// sessions (no step) stay. Mirrors frontend board/stageSessions.ts.
func (b *TaskBoard) stageSessions(taskID, from, to string) ([]model.AdeSession, error) {
	rows, err := b.deps.Sessions.ListRunningTUI()
	if err != nil {
		return nil, err
	}
	var out []model.AdeSession
	for _, r := range rows {
		if r.TaskID != taskID {
			continue
		}
		review := r.Purpose == "review" && from == reviewStepID && to != reviewStepID
		if review || (r.Purpose != "review" && r.StepID != "" && r.StepID != to) {
			out = append(out, r)
		}
	}
	return out, nil
}

// closeStageSessionsLocked closes the stage sessions of a move. A session that exits meanwhile is
// skipped. taskMu is held by the caller.
func (b *TaskBoard) closeStageSessionsLocked(taskID, from, to string) error {
	rows, err := b.stageSessions(taskID, from, to)
	if err != nil || len(rows) == 0 {
		return err
	}
	tr, err := b.tracker()
	if err != nil {
		return err
	}
	for i := range rows {
		if err := b.closeTUILocked(tr, &rows[i]); err != nil && !errors.Is(err, errSessionGone) {
			return fmt.Errorf("ade: close stage session %s: %w", rows[i].ID, err)
		}
	}
	return nil
}
