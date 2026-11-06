package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

func TestRunWithAbortRace_CallerCtxCancelledDoesNotReachIssue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	issueSawDone := make(chan bool, 1)
	releaseCh := make(chan struct{})

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := adapters.RunWithAbortRace(ctx, func() { close(releaseCh) }, func(issueCtx context.Context) (int, error) {
		// issueCtx must be detached from ctx's own cancellation (context.WithoutCancel) — block
		// past the caller's cancel and confirm issueCtx itself never observes it.
		select {
		case <-issueCtx.Done():
			issueSawDone <- true
		case <-time.After(200 * time.Millisecond):
			issueSawDone <- false
		}
		return 0, nil
	})
	elapsed := time.Since(start)

	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.CodeCancelled {
		t.Fatalf("got %v, want an E_CANCELLED *adapters.Error", err)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("RunWithAbortRace returned after %s, want it to return promptly on ctx.Done()", elapsed)
	}
	if <-issueSawDone {
		t.Error("issue's own context observed Done() — it must be detached from the caller's ctx")
	}

	select {
	case <-releaseCh:
	case <-time.After(1 * time.Second):
		t.Error("release was never called once issue actually settled")
	}
}
