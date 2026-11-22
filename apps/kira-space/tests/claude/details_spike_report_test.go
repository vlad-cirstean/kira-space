package claude

import (
	"slices"
)

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Clone(v)
	slices.Sort(s)
	if n := len(s); n%2 == 0 {
		return (s[n/2-1] + s[n/2]) / 2
	}
	return s[len(s)/2]
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var t float64
	for _, x := range v {
		t += x
	}
	return t / float64(len(v))
}

type spikeAgg struct {
	runs, pass                 int
	cost, turns, in, cr, cw, o []float64
	c1, cf, gr                 []float64
	edits, shell, failed, retr []float64
	whole, wrong, reads, rbyte []float64
	mcp                        float64
}

func spikeAggregate(recs []spikeRec, flow, arm, model string) spikeAgg {
	var a spikeAgg
	var mcp []float64
	for _, r := range recs {
		if r.Flow != flow || r.Arm != arm || r.Model != model {
			continue
		}
		a.runs++
		if r.Pass {
			a.pass++
		}
		a.cost = append(a.cost, r.Cost)
		a.turns = append(a.turns, float64(r.Turns))
		a.in = append(a.in, float64(r.In))
		a.cr = append(a.cr, float64(r.CacheRead))
		a.cw = append(a.cw, float64(r.CacheWrite))
		a.o = append(a.o, float64(r.Out))
		a.edits = append(a.edits, float64(r.EditCalls))
		a.shell = append(a.shell, float64(r.ShellWrites))
		a.failed = append(a.failed, float64(r.FailedEdits))
		a.retr = append(a.retr, float64(r.Retried))
		a.whole = append(a.whole, float64(r.WholeRewrites))
		a.wrong = append(a.wrong, float64(r.WrongFile))
		a.reads = append(a.reads, float64(r.ReadCalls))
		a.rbyte = append(a.rbyte, float64(r.ReadBytes))
		a.c1 = append(a.c1, float64(r.CtxFirst))
		a.cf = append(a.cf, float64(r.CtxFinal))
		a.gr = append(a.gr, r.CtxGrowth)
		mcp = append(mcp, float64(r.MCPTools))
	}
	a.mcp = mean(mcp)
	return a
}

func yn(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
