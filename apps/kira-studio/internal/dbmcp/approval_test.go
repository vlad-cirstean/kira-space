package dbmcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func recvApprovalOrTimeout(t *testing.T, ch chan ApprovalOutcome) ApprovalOutcome {
	t.Helper()
	select {
	case out := <-ch:
		return out
	case <-time.After(2 * time.Second):
		t.Fatal("Request never returned")
		return 0
	}
}

// waitItems blocks until the broker's queue holds n items and returns them, oldest first.
func waitItems(t *testing.T, b *ApprovalBroker, n int) []approvals.Item {
	t.Helper()
	var items []approvals.Item
	testx.WaitUntil(t, 5*time.Second, func() bool {
		items = b.Queue.List()
		return len(items) == n
	})
	return items
}

func requestAsync(b *ApprovalBroker, ctx context.Context, connectionID string) chan ApprovalOutcome {
	done := make(chan ApprovalOutcome, 1)
	go func() { done <- b.Request(ctx, ApprovalRequest{ConnectionID: connectionID}) }()
	return done
}

func requestID(it approvals.Item) string { return strings.TrimPrefix(it.ID, it.Kind+":") }

// Every pending request is an item at once, and each answer reaches only its own caller.
func TestApprovalBroker_AnswersEachItemIndependently(t *testing.T) {
	t.Parallel()
	b := NewApprovalBroker(time.Now, nil)

	doneA := requestAsync(b, context.Background(), "a")
	waitItems(t, b, 1)
	doneB := requestAsync(b, context.Background(), "b")
	waitItems(t, b, 2)
	doneC := requestAsync(b, context.Background(), "c")
	items := waitItems(t, b, 3)

	res := b.Queue.Answer([]string{items[1].ID}, false, nil)
	if len(res) != 1 || res[0].Error != "" {
		t.Fatalf("deny b: %+v", res)
	}
	if out := recvApprovalOrTimeout(t, doneB); out != ApprovalDenied {
		t.Fatalf("b outcome: got %v, want denied", out)
	}
	for name, ch := range map[string]chan ApprovalOutcome{"a": doneA, "c": doneC} {
		select {
		case out := <-ch:
			t.Fatalf("%s returned %v before its own answer", name, out)
		default:
		}
	}

	b.Queue.Answer([]string{items[0].ID}, true, nil)
	if out := recvApprovalOrTimeout(t, doneA); out != ApprovalApproved {
		t.Fatalf("a outcome: got %v, want approved", out)
	}
	if rest := b.Queue.List(); len(rest) != 1 || rest[0].ID != items[2].ID {
		t.Fatalf("queue = %+v, want only c", rest)
	}
	b.Queue.Answer([]string{items[2].ID}, true, nil)
	if out := recvApprovalOrTimeout(t, doneC); out != ApprovalApproved {
		t.Fatalf("c outcome: got %v", out)
	}
}

func TestApprovalBroker_DoubleApprove_ReportsAlreadyResolved(t *testing.T) {
	t.Parallel()
	b := NewApprovalBroker(time.Now, nil)

	done := requestAsync(b, context.Background(), "a")
	id := requestID(waitItems(t, b, 1)[0])

	if got := b.Approve(id); got != ApprovalActionResolved {
		t.Fatalf("first approve: got %v", got)
	}
	if out := <-done; out != ApprovalApproved {
		t.Fatalf("outcome: got %v", out)
	}
	if got := b.Approve(id); got != ApprovalActionAlreadyResolved {
		t.Fatalf("second approve: got %v, want AlreadyResolved", got)
	}
	if got := b.Deny(id); got != ApprovalActionAlreadyResolved {
		t.Fatalf("deny after approve: got %v, want AlreadyResolved", got)
	}
	if left := b.Queue.List(); len(left) != 0 {
		t.Fatalf("queue must stay empty, got %+v", left)
	}
}

// Request selects on the caller's own ctx, so a disconnected MCP client's request resolves as
// abandoned without disturbing the others.
func TestApprovalBroker_CtxCancelLeavesOtherItems(t *testing.T) {
	t.Parallel()
	b := NewApprovalBroker(time.Now, nil)

	doneA := requestAsync(b, context.Background(), "a")
	first := waitItems(t, b, 1)[0]

	ctxB, cancelB := context.WithCancel(context.Background())
	doneB := requestAsync(b, ctxB, "b")
	waitItems(t, b, 2)

	cancelB()
	if out := recvApprovalOrTimeout(t, doneB); out != ApprovalAbandoned {
		t.Fatalf("b outcome: got %v, want ApprovalAbandoned", out)
	}
	if left := b.Queue.List(); len(left) != 1 || left[0].ID != first.ID {
		t.Fatalf("after cancelling b: %+v, want only a", left)
	}

	b.Approve(requestID(first))
	if out := recvApprovalOrTimeout(t, doneA); out != ApprovalApproved {
		t.Fatalf("a outcome: got %v", out)
	}
}

// A "now" frozen far in the past makes the minted ExpiresAt already elapsed, so the timer branch
// fires at once with no real sleep.
func TestApprovalBroker_DeadlineExpiresWithInjectedClock(t *testing.T) {
	t.Parallel()
	longAgo := time.Unix(0, 0)
	b := NewApprovalBroker(func() time.Time { return longAgo }, nil)

	done := requestAsync(b, context.Background(), "a")
	if out := recvApprovalOrTimeout(t, done); out != ApprovalTimedOut {
		t.Fatalf("got %v, want ApprovalTimedOut", out)
	}
	if left := b.Queue.List(); len(left) != 0 {
		t.Fatalf("after timeout: %+v, want empty", left)
	}
}

// An MCP client is a program and can call run_query in a loop, so the broker denies immediately past
// its cap rather than grow without bound.
func TestApprovalBroker_QueueBoundedAgainstUnlimitedEnqueue(t *testing.T) {
	t.Parallel()
	b := NewApprovalBroker(time.Now, nil)

	results := make([]chan ApprovalOutcome, 0, maxPendingApprovals)
	for i := 0; i < maxPendingApprovals; i++ {
		results = append(results, requestAsync(b, context.Background(), "c"))
		waitItems(t, b, i+1)
	}

	if out := b.Request(context.Background(), ApprovalRequest{ConnectionID: "over-cap"}); out != ApprovalQueueFull {
		t.Fatalf("over-cap outcome: got %v, want ApprovalQueueFull", out)
	}
	if n := len(b.Queue.List()); n != maxPendingApprovals {
		t.Fatalf("queue = %d, want unchanged %d", n, maxPendingApprovals)
	}
	for _, done := range results {
		select {
		case <-done:
			t.Fatal("an already-queued request resolved on its own")
		default:
		}
	}
}

// The app quitting or the server toggling off must not leave any run_query call blocked forever, and
// the broker must still work if the server is toggled back on within the same app run.
func TestApprovalBroker_AbandonAll_ReleasesAndStaysUsable(t *testing.T) {
	t.Parallel()
	b := NewApprovalBroker(time.Now, nil)

	doneA := requestAsync(b, context.Background(), "a")
	waitItems(t, b, 1)
	doneB := requestAsync(b, context.Background(), "b")
	waitItems(t, b, 2)

	b.AbandonAll()
	for name, ch := range map[string]chan ApprovalOutcome{"a": doneA, "b": doneB} {
		if out := recvApprovalOrTimeout(t, ch); out != ApprovalAbandoned {
			t.Fatalf("%s outcome: got %v, want ApprovalAbandoned", name, out)
		}
	}
	if left := b.Queue.List(); len(left) != 0 {
		t.Fatalf("after AbandonAll: %+v, want empty", left)
	}

	doneC := requestAsync(b, context.Background(), "c")
	c := waitItems(t, b, 1)[0]
	b.Approve(requestID(c))
	if out := recvApprovalOrTimeout(t, doneC); out != ApprovalApproved {
		t.Fatalf("c outcome: got %v", out)
	}
}
