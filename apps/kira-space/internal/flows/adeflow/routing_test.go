package adeflow_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/internal/testx"
)

// J-RUN-030, J-RUN-020. Routing matrix: every result id of s3 routes to its stated next step.
func TestRoutingMatrix(t *testing.T) {
	s3 := "    results:\n" +
		"      - id: ok\n        ok: true\n" +
		"      - id: redo\n        ok: false\n        next: s2\n" +
		"      - id: restart\n        ok: false\n        next: s1\n" +
		"      - id: skip\n        ok: true\n        next: s4\n" +
		"      - id: halt\n        ok: false\n        next: stop\n" +
		"      - id: finish\n        ok: true\n        next: end\n" +
		"    max_returns: 3\n"
	steps := agentStep("s1", "") + agentStep("s2", "") + agentStep("s3", s3) + agentStep("s4", "") + manualStep
	cases := []struct {
		name  string
		acts  []string
		runs  map[string]int // run rows per step when the flow settles
		route string         // s3's first run route
		last  string         // step whose run settles the flow
		state string
	}{
		{"ok goes next", []string{"done", "done", "result:ok", "done"}, map[string]int{"s1": 1, "s2": 1, "s3": 1, "s4": 1}, "next", "s4", "done"},
		{"redo goes to s2", []string{"done", "done", "result:redo", "done", "result:ok", "done"}, map[string]int{"s1": 1, "s2": 2, "s3": 2, "s4": 1}, "back:s2", "s4", "done"},
		{"restart goes to s1", []string{"done", "done", "result:restart", "done", "done", "result:ok", "done"}, map[string]int{"s1": 2, "s2": 2, "s3": 2, "s4": 1}, "back:s1", "s4", "done"},
		{"skip goes to s4", []string{"done", "done", "result:skip", "done"}, map[string]int{"s1": 1, "s2": 1, "s3": 1, "s4": 1}, "s4", "s4", "done"},
		{"halt stops", []string{"done", "done", "result:halt"}, map[string]int{"s1": 1, "s2": 1, "s3": 1, "s4": 0}, "stop", "s3", "failed"},
		{"finish ends the workflow", []string{"done", "done", "result:finish"}, map[string]int{"s1": 1, "s2": 1, "s3": 1, "s4": 0}, "end", "s3", "done"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newRunFixture(t, acts(c.acts...), steps)
			f.start(t, "feat/api-route")
			waitRun(t, f.app, f.taskID, c.last, c.state)
			testx.WaitUntil(t, waitFor, func() bool {
				for step, n := range c.runs {
					if got := len(stepRuns(t, f, step)); got < n {
						return false
					}
				}
				return true
			})
			for step, n := range c.runs {
				if got := len(stepRuns(t, f, step)); got != n {
					t.Errorf("%s run rows = %d, want %d: %+v", step, got, n, stepRuns(t, f, step))
				}
			}
			first := stepRuns(t, f, "s3")[0]
			if _, route := resultOf(first); route != c.route {
				t.Errorf("s3 first run = %+v, want route %q", first.Outcome, c.route)
			}
		})
	}
}

// J-RUN-030 step 6 and 8. An unknown result id falls back to the step's failed route; returns stop at
// max_returns with the stated message and the count stays spent after the stop.
func TestRoutingUnknownResultAndSpentLoop(t *testing.T) {
	s3 := "    results:\n      - id: ok\n        ok: true\n      - id: redo\n        ok: false\n        next: s2\n    max_returns: 2\n"
	steps := agentStep("s2", "") + agentStep("s3", s3)
	f := newRunFixture(t, acts("done", "result:redo", "done", "result:redo", "done", "result:redo"), steps)
	f.start(t, "feat/api-spent")
	stopped := waitRun(t, f.app, f.taskID, "s3", "failed")
	const want = "Stopped: s3 reached its limit of 2 returns."
	if stopped.Note != want {
		t.Fatalf("stopped = %+v, want %q", stopped, want)
	}
	if got := returnsOf(stepRuns(t, f, "s3")); got[len(got)-1] != 2 {
		t.Fatalf("returns = %v, want the counter spent at 2", got)
	}

	g := newRunFixture(t, acts("done", "result:maybe"), steps)
	g.start(t, "feat/api-unknown")
	r := waitRun(t, g.app, g.taskID, "s3", "failed", "stuck")
	if r.Outcome == nil || r.Outcome.Status == "done" {
		t.Fatalf("run with an unknown result id = %+v, want it not done", r)
	}
}
