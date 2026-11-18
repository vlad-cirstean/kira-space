package termflow_test

import (
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/metrics"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

// Backend half of the status bar Scripts scenario; the UI half is window-chrome.spec.ts. A running
// script shows under Scripts with live values, not Agents, totals add up, Scripts empties at the end.
func TestAppMetricsCountsScripts(t *testing.T) {
	const timeout = 15 * time.Second // >= 3 metric intervals
	scriptsRunning := func(s metrics.Sample) bool { return s.Scripts.ProcessCount >= 1 && s.Scripts.MemoryBytes > 0 }
	scriptsGone := func(s metrics.Sample) bool { return s.Scripts.ProcessCount == 0 }
	check := func(t *testing.T, s metrics.Sample) {
		t.Helper()
		if s.Agents.ProcessCount != 0 {
			t.Fatalf("script filed under Agents: %+v", s)
		}
		flowharness.CheckMetricsTotals(t, s)
	}

	t.Run("terminal run", func(t *testing.T) {
		app := flowharness.New(t)
		rec := newScript(t, app, scripts.CustomScriptFields{Command: "sleep 30"})
		mark := app.Events.Mark()
		openScript(t, app, "w1", "s1", rec.ID)
		waitRun(t, app, "s1", "running")
		check(t, app.WaitMetrics(t, mark, scriptsRunning, timeout))

		if err := app.W.Terminal.Close(terminal.CloseArgs{TerminalID: "s1"}); err != nil {
			t.Fatal(err)
		}
		app.WaitMetrics(t, app.Events.Mark(), scriptsGone, timeout)
	})

	t.Run("headless run", func(t *testing.T) {
		app, clock := clockApp(t)
		rec := recurring(t, app, "* * * * *", func(f *scripts.CustomScriptFields) { f.Command = "sleep 30" })
		mark := app.Events.Mark()
		advance(t, clock, 30*time.Second)
		run := waitSched(t, app, rec.ID, 0, "running")
		check(t, app.WaitMetrics(t, mark, scriptsRunning, timeout))

		if err := app.W.ScriptRuns.Stop(scriptruns.IDArgs{ID: run.ID}); err != nil {
			t.Fatal(err)
		}
		waitSched(t, app, rec.ID, 0, "cancelled")
		app.WaitMetrics(t, app.Events.Mark(), scriptsGone, timeout)
	})
}
