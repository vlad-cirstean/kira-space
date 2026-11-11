package testsupport

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
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
			// Bounded so a missing timeout fails in seconds instead of waiting out the query.
			ctx, cancel := context.WithTimeout(context.Background(), limit+10*time.Second)
			defer cancel()
			_, err := target.Execute(ctx, model.ConsoleRequest{
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

// ReadStatementTimeoutCase drives ReadStatementTimeout. Setup creates the slow view and Teardown
// drops it, on Admin's adapter (nil: the scenario's own adapter), so a least-privilege matrix row
// can read a view it could not create. Path is that view, Fast a quick table read afterwards.
type ReadStatementTimeoutCase struct {
	Name     string
	Setup    []string
	Teardown []string
	Path     model.NodePath // the slow view
	Fast     model.NodePath // a table that reads at once
	SetupOn  model.NodePath // console path the Setup and Teardown statements run under
	Admin    func(t *testing.T) adapters.Adapter
	Lane     bool
	WantCode string
}

// ReadStatementTimeout asserts a 1 s StatementTimeout on the adapter Read path (the browse page)
// stops a view whose rows sleep, maps to the typed timeout carrying WantCode, and leaves the
// session clean: the next Read without a timeout succeeds.
func ReadStatementTimeout(c ReadStatementTimeoutCase) Scenario {
	return Scenario{
		Name:     "read statement timeout: " + c.Name,
		Requires: func(caps adapters.Caps) bool { return caps.SQL },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			admin := a
			if c.Admin != nil {
				admin = c.Admin(t)
			}
			execAll(t, admin, c.SetupOn, c.Setup)
			t.Cleanup(func() { execAll(t, admin, c.SetupOn, c.Teardown) })
			target := a
			if c.Lane {
				target = laneAdapter(t, a, cfg)
			}
			read := func(path model.NodePath, limit time.Duration) error {
				// Bounded so a missing timeout fails in seconds instead of waiting out the query.
				ctx, cancel := context.WithTimeout(context.Background(), limit+10*time.Second)
				defer cancel()
				_, err := target.Read(ctx, adapters.ReadRequest{
					Path: path, PageSize: 10, Cursor: model.PageCursor{Mode: "offset"}, StatementTimeout: limit,
				}, adapters.NewOpCtx("read-timeout"))
				return err
			}
			const limit = time.Second
			start := time.Now()
			err := read(c.Path, limit)
			var detail *adapters.TimeoutDetail
			if !errors.As(err, &detail) {
				t.Fatalf("Read(slow view) = %v, want a statement timeout", err)
			}
			if cause := detail.Cause; cause == nil || !strings.Contains(cause.Error(), c.WantCode) {
				t.Errorf("timeout cause = %v, want it to contain %q", detail.Cause, c.WantCode)
			}
			if took := time.Since(start); took > limit+3*time.Second {
				t.Errorf("timeout took %s, want about %s", took, limit)
			}
			if err := read(c.Fast, 0); err != nil {
				t.Errorf("Read(fast table) after the timeout: %v", err)
			}
		},
	}
}
