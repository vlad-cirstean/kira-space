package flowharness

import (
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/metrics"
)

// WaitMetrics blocks until a kira:app:metrics sample after mark satisfies pred and returns it.
func (a *App) WaitMetrics(t testing.TB, mark int, pred func(metrics.Sample) bool, timeout time.Duration) metrics.Sample {
	t.Helper()
	decode := func(ev Event) metrics.Sample {
		var s metrics.Sample
		ev.Decode(t, &s)
		return s
	}
	ev := a.Events.WaitAfter(t, mark, appevent.ChannelAppMetrics, func(ev Event) bool { return pred(decode(ev)) }, timeout)
	return decode(ev)
}

// CheckMetricsTotals fails unless the top-level memory and process count equal App + Agents + Scripts.
func CheckMetricsTotals(t testing.TB, s metrics.Sample) {
	t.Helper()
	if s.MemoryBytes != s.App.MemoryBytes+s.Agents.MemoryBytes+s.Scripts.MemoryBytes ||
		s.ProcessCount != s.App.ProcessCount+s.Agents.ProcessCount+s.Scripts.ProcessCount {
		t.Fatalf("totals are not App + Agents + Scripts: %+v", s)
	}
}
