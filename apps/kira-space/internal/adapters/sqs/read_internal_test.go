package sqs

import (
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// TestReceiptHandles_ForDelete_ExpiredHandleRefused is the regression: a cached receipt
// handle past its own recorded visibility timeout must be refused outright, rather than letting
// AWS silently accept a delete with a stale handle and report a false success. White-box (package
// sqs), no live queue needed — receivedAt is set directly, simulating time having passed rather
// than sleeping in the test.
func TestReceiptHandles_ForDelete_ExpiredHandleRefused(t *testing.T) {
	h := newReceiptHandles()
	h.mu.Lock()
	h.entries["m1"] = receiptHandleEntry{
		handle:            "rh1",
		receivedAt:        time.Now().Add(-2 * time.Second),
		visibilityTimeout: time.Second,
	}
	h.order = []string{"m1"}
	h.mu.Unlock()

	_, err := h.forDelete("m1")
	if err == nil {
		t.Fatal("want an error for an expired receipt handle, got nil")
	}
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}
}

// TestReceiptHandles_ForDelete_StillValid is the control case: a handle well inside its own
// visibility timeout is returned normally.
func TestReceiptHandles_ForDelete_StillValid(t *testing.T) {
	h := newReceiptHandles()
	h.set("m1", "rh1", time.Minute)

	handle, err := h.forDelete("m1")
	if err != nil {
		t.Fatalf("forDelete: %v", err)
	}
	if handle != "rh1" {
		t.Errorf("handle = %q, want %q", handle, "rh1")
	}
}

// TestReceiptHandles_ForDelete_UnknownMessageIsQueryError covers the pre-existing "never received"
// case, kept correct alongside the new expiry check.
func TestReceiptHandles_ForDelete_UnknownMessageIsQueryError(t *testing.T) {
	h := newReceiptHandles()
	_, err := h.forDelete("missing")
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}
}

// TestReceiptHandles_ForDelete_ExactlyAtBoundary confirms the boundary itself (time.Since ==
// visibilityTimeout) is treated as expired, not valid — CLAUDE.md's own boundary-arithmetic
// exception is exactly this off-by-one class of case.
func TestReceiptHandles_ForDelete_ExactlyAtBoundary(t *testing.T) {
	h := newReceiptHandles()
	h.mu.Lock()
	h.entries["m1"] = receiptHandleEntry{
		handle: "rh1",
		// A hair before "now - timeout" so time.Since(receivedAt) >= visibilityTimeout is
		// deterministically true regardless of scheduling jitter between here and the assertion.
		receivedAt:        time.Now().Add(-time.Second - time.Millisecond),
		visibilityTimeout: time.Second,
	}
	h.order = []string{"m1"}
	h.mu.Unlock()

	if _, err := h.forDelete("m1"); err == nil {
		t.Fatal("want the exact-boundary case treated as expired")
	}
}
