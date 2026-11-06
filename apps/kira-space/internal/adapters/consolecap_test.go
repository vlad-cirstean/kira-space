package adapters

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

func TestRemainingCapSharesBudgetAcrossBatch(t *testing.T) {
	c := page.ResultCap{Rows: 10, Bytes: 100}
	c = RemainingCap(c, 60)
	if c.Bytes != 40 || c.Rows != 10 {
		t.Fatalf("after 60 used: %+v", c)
	}
	c = RemainingCap(c, 70) // overshoot must floor at 1, never 0 (= unlimited) or negative
	if c.Bytes != 1 {
		t.Fatalf("exhausted budget = %d, want 1", c.Bytes)
	}
	if got := RemainingCap(page.ResultCap{Rows: 5}, 99); got.Bytes != 0 {
		t.Fatalf("an unbounded byte axis must stay unbounded, got %d", got.Bytes)
	}
}
