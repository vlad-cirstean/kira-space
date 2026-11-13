package ade

import (
	"context"
	"errors"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const reviewSendStep = "  - id: review\n    name: review\n    kind: review\n    runs_on: each repo\n    timeout: 1m\n    max_returns: 2\n" +
	"    review:\n      send_to: implementer\n    results:\n      - id: approved\n        ok: true\n      - id: rework\n        ok: false\n        next: implement\n"

// SendReview gathers before the task lock and rechecks under it: a change between the phases never
// queues a round on a run that moved on.
func TestSendReview_changeBetweenPhases(t *testing.T) {
	e := newEngine(t, map[string][]string{"*": {"done"}})
	e.repo("api")
	e.workflow(flowYAML(agentStep("implement", "") + reviewSendStep))
	id := e.task("api")
	e.start(id)
	run := e.waitRun(id, "review/api", model.AdeRunReview)
	args := adewire.SendReviewArgs{RunID: run.ID, Route: adewire.ReviewSendImplementer}
	ctx := context.Background()
	roundRuns := func() int {
		rounds, err := e.repos.AdeTasks.ReviewRounds(id)
		if err != nil {
			t.Fatal(err)
		}
		return len(rounds)
	}

	g, err := e.board.gatherReviewSend(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.repos.AdeTasks.BumpReviewRound(run.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.board.applyReviewSend(ctx, args, g); !errors.Is(err, errReviewMoved) {
		t.Fatalf("round count moved: err = %v, want errReviewMoved", err)
	}

	g, err = e.board.gatherReviewSend(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	done := model.AdeRunDone
	if _, err := e.repos.AdeTasks.UpdateRun(run.ID, model.AdeRunPatch{State: &done}); err != nil {
		t.Fatal(err)
	}
	if err := e.board.applyReviewSend(ctx, args, g); err == nil || errors.Is(err, errReviewMoved) {
		t.Fatalf("finished run: err = %v, want a refusal", err)
	}
	if n := roundRuns(); n != 0 {
		t.Fatalf("%d round runs queued on a finished run", n)
	}
}
