package sqs

import (
	"context"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func strPtr(s string) *string { return &s }

// TestResolveFIFOInsertFields_NonFIFOQueueIsUntouched is the control case: a standard queue's
// insert never needs (or gets) MessageGroupId/MessageDeduplicationId, and never pays for the extra
// GetQueueAttributes round trip content-based-dedup detection would otherwise cost (client is nil
// here — a real client would panic if this path touched it).
func TestResolveFIFOInsertFields_NonFIFOQueueIsUntouched(t *testing.T) {
	groupID, dedupID, err := resolveFIFOInsertFields(context.Background(), nil, "https://sqs/my-queue", "my-queue", model.RowValues{})
	if err != nil {
		t.Fatalf("resolveFIFOInsertFields: %v", err)
	}
	if groupID != nil || dedupID != nil {
		t.Errorf("groupID=%v dedupID=%v, want both nil for a non-FIFO queue", groupID, dedupID)
	}
}

// TestResolveFIFOInsertFields_FIFOMissingGroupIDIsQueryError is the regression: a FIFO queue
// insert with no $messageGroupId used to reach AWS's own SendMessage with neither field set,
// failing with AWS's own opaque rejection — this asserts the adapter now catches it first with a
// clear message.
func TestResolveFIFOInsertFields_FIFOMissingGroupIDIsQueryError(t *testing.T) {
	_, _, err := resolveFIFOInsertFields(context.Background(), nil, "https://sqs/my-queue.fifo", "my-queue.fifo", model.RowValues{})
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}
}

// TestResolveFIFOInsertFields_FIFOWithGroupAndDedupIDs is the happy path: both sentinels present,
// no GetQueueAttributes round trip needed (client stays nil and untouched).
func TestResolveFIFOInsertFields_FIFOWithGroupAndDedupIDs(t *testing.T) {
	values := model.RowValues{
		{Name: messageGroupIDField, Value: strPtr("group-1")},
		{Name: messageDeduplicationIDField, Value: strPtr("dedup-1")},
	}
	groupID, dedupID, err := resolveFIFOInsertFields(context.Background(), nil, "https://sqs/my-queue.fifo", "my-queue.fifo", values)
	if err != nil {
		t.Fatalf("resolveFIFOInsertFields: %v", err)
	}
	if groupID == nil || *groupID != "group-1" {
		t.Errorf("groupID = %v, want \"group-1\"", groupID)
	}
	if dedupID == nil || *dedupID != "dedup-1" {
		t.Errorf("dedupID = %v, want \"dedup-1\"", dedupID)
	}
}

// TestIsFIFOQueueName pins the exact, case-sensitive AWS naming convention this adapter keys F9's
// whole behavior off of.
func TestIsFIFOQueueName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"orders.fifo", true},
		{"orders", false},
		{"orders.FIFO", false}, // AWS's own suffix is case-sensitive
		{"", false},
	}
	for _, c := range cases {
		if got := isFIFOQueueName(c.name); got != c.want {
			t.Errorf("isFIFOQueueName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
