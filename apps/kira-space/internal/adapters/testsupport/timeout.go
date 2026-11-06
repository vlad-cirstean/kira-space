package testsupport

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// StatementTimeoutCase drives the StatementTimeout scenario. Setup and Teardown run on the main
// adapter. Slow runs with a 1 s StatementTimeout on the lane adapter (Lane) or the main one and
// must fail with the typed timeout whose cause contains WantCode. After runs on that same adapter
// next (a ROLLBACK for a user-transaction case), then Baseline must read BaselineWant (the engine
// setting was restored), then Lifted (a ~2 s statement) must succeed with no timeout.
type StatementTimeoutCase struct {
	Name         string
	Path         model.NodePath
	Lane         bool
	Setup        []string
	After        []string
	Teardown     []string
	Slow         string
	WantCode     string
	Unchanged    func(t *testing.T, main adapters.Adapter)
	Baseline     string
	BaselineWant string
	Lifted       string
}

// StatementTimeout asserts the engine-side statement timeout stops Slow, is gone again for the next
// statement on the same connection, and is not applied when StatementTimeout is zero.
func StatementTimeout(c StatementTimeoutCase) Scenario {
	return Scenario{
		Name:     "statement timeout: " + c.Name,
		Requires: func(caps adapters.Caps) bool { return caps.SQL },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			execAll(t, a, c.Path, c.Setup)
			t.Cleanup(func() { execAll(t, a, c.Path, c.Teardown) })
			target := a
			if c.Lane {
				target = laneAdapter(t, a, cfg)
			}
			const limit = time.Second
			start := time.Now()
			_, err := target.Execute(context.Background(), model.ConsoleRequest{
				Path: c.Path, Statements: []string{c.Slow}, ReadOnly: c.Lane, StatementTimeout: limit,
			}, adapters.NewOpCtx("timeout-slow"))
			var detail *adapters.TimeoutDetail
			if !errors.As(err, &detail) {
				t.Fatalf("Execute(%s) = %v, want a statement timeout", c.Slow, err)
			}
			if cause := detail.Cause; cause == nil || !strings.Contains(cause.Error(), c.WantCode) {
				t.Errorf("timeout cause = %v, want it to contain %q", detail.Cause, c.WantCode)
			}
			if took := time.Since(start); took > limit+3*time.Second {
				t.Errorf("timeout took %s, want about %s", took, limit)
			}
			if c.Unchanged != nil {
				c.Unchanged(t, a)
			}
			execAll(t, target, c.Path, c.After)
			if got := FirstCell(t, target, c.Path, c.Baseline); got != c.BaselineWant {
				t.Errorf("%s = %q after the timed-out statement, want %q (setting leaked)", c.Baseline, got, c.BaselineWant)
			}
			if _, err := target.Execute(context.Background(), model.ConsoleRequest{
				Path: c.Path, Statements: []string{c.Lifted}, ReadOnly: c.Lane,
			}, adapters.NewOpCtx("timeout-lifted")); err != nil {
				t.Errorf("Execute(%s) without a timeout: %v", c.Lifted, err)
			}
		},
	}
}
