package ade

import (
	"fmt"
	"time"

	"github.com/kirathecat/kira-studio/internal/prompts"
)

// defaultCallWait is how long a state call blocks on work that is not finished yet: the user's
// approval, a worktree setup, a lease queue. When it ends nothing is cancelled: the call answers
// pending or waiting and the work goes on.
const defaultCallWait = prompts.AnswerWait

func (b *TaskBoard) callWait() time.Duration {
	if b.deps.CallWait > 0 {
		return b.deps.CallWait
	}
	return defaultCallWait
}

// waitText is d in whole minutes, or seconds under a minute.
func waitText(d time.Duration) string {
	if d >= time.Minute {
		return fmt.Sprintf("%d minutes", int(d/time.Minute))
	}
	return fmt.Sprintf("%d seconds", int(d/time.Second))
}
