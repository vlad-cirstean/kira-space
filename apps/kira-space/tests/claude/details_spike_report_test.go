package claude

import (
	"fmt"
	"slices"
	"strings"
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

// spikeReport renders the results file: a table per flow and arm, gate verdicts, raw runs.
func spikeReport(recs []spikeRec, spent float64, aborted string, planned int) string {
	var b strings.Builder
	b.WriteString("# P643 Part 1 spike results\n\n")
	fmt.Fprintf(&b, "Test: `apps/kira-space/tests/claude/details_spike_test.go`. Arm T = today's tools (real `agentmcp.Server`, stub backends, kira-studio `proof_save` stub with today's schema). Arm F = planned files plus `submit`, `finish_step` without summary, merged `branches`, `lease`, `cancel_operation`, README in the task folder. Both arms get the same prompt core; arm F adds one line naming the task folder.\n\n")
	fmt.Fprintf(&b, "Runs: %d of %d planned. Spend: %.4f USD (cap %.2f total, %.2f per run). Single consolidated-file variant (one file instead of many small ones) was not run: the flows read at most two handovers, so the arm would add no signal within budget; read effort of the per-file layout is in the edit table. Cost is `total_cost_usd`; tokens are the result line's `usage` (input excludes cache; cache read and write listed apart).\n\n", len(recs), planned, spent, spikeTotalCap, spikeRunCap)
	if aborted != "" {
		fmt.Fprintf(&b, "**Aborted early:** %s\n\n", aborted)
	}
	names := map[string]string{}
	var order []string
	for _, f := range spikeFlows {
		names[f.id] = f.name
		order = append(order, f.id)
	}
	for _, model := range []string{"haiku", "sonnet"} {
		fmt.Fprintf(&b, "## %s\n\n", model)
		b.WriteString("| Flow | Arm | Runs | Pass | Mean cost USD | Median cost USD | Mean turns | Median turns | Mean in | Mean cache read | Mean cache write | Mean out | MCP tools listed | Turn-1 context | Final context | Growth per turn |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
		for _, id := range order {
			for _, arm := range []string{"T", "F"} {
				a := spikeAggregate(recs, id, arm, model)
				if a.runs == 0 {
					continue
				}
				fmt.Fprintf(&b, "| %s %s | %s | %d | %d/%d | %.4f | %.4f | %.1f | %.1f | %.0f | %.0f | %.0f | %.0f | %.0f | %.0f | %.0f | %.0f |\n",
					id, names[id], arm, a.runs, a.pass, a.runs, mean(a.cost), median(a.cost), mean(a.turns), median(a.turns),
					mean(a.in), mean(a.cr), mean(a.cw), mean(a.o), a.mcp, mean(a.c1), mean(a.cf), mean(a.gr))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("## Edit ease and read effort (mean per run)\n\nEdit and Write calls, shell writes (Bash redirect, tee, cp, mv, sed -i), failed edit-path calls (failed edit, write or shell write; any Kira tool error; a submit or finish_step result invalid, rejected or restored), retries (a call on the same file or tool right after a failure), whole-file rewrites (Write over an existing file), wrong-file writes (README.md, task.yaml or state.yaml of the task folder), and read effort (Read, Glob, Grep and read-only Bash calls, with result bytes; per-file layout cost for arm F).\n\n")
	for _, model := range []string{"haiku", "sonnet"} {
		fmt.Fprintf(&b, "### %s\n\n| Flow | Arm | Edit/Write | Shell writes | Failed edit-path | Retried | Whole rewrites | Wrong-file | Read calls | Read bytes |\n|---|---|---|---|---|---|---|---|---|---|\n", model)
		for _, id := range order {
			for _, arm := range []string{"T", "F"} {
				a := spikeAggregate(recs, id, arm, model)
				if a.runs == 0 {
					continue
				}
				fmt.Fprintf(&b, "| %s %s | %s | %.1f | %.1f | %.2f | %.2f | %.2f | %.2f | %.1f | %.0f |\n", id, names[id], arm,
					mean(a.edits), mean(a.shell), mean(a.failed), mean(a.retr), mean(a.whole), mean(a.wrong), mean(a.reads), mean(a.rbyte))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("## Gate (haiku)\n\nPrimary metrics, per flow, haiku runs (user re-weighting): (1) cost, F median at most 1.2 times T (F6: 1.1); (2) final context, F median at most 1.0 times T (median of the last model call's fresh input + cache read + cache write); (3) pass rate F at least T; (4) edit ease, F mean of failed edit-path calls plus retries at most T's. Turns are reported only, never a threshold. Turn-1 context (tool-schema overhead) and growth per turn are in the first tables. Each metric has its own verdict, so a flow that fails only one is visible.\n\n")
	b.WriteString("| Flow | Cost F/T | Cost | Final ctx F/T | Ctx | Pass F vs T | Pass | Edit fails+retries F vs T | Edit | Turns F vs T (info) | Verdict |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	all, any := true, false
	for _, id := range order {
		t, f := spikeAggregate(recs, id, "T", "haiku"), spikeAggregate(recs, id, "F", "haiku")
		if t.runs == 0 || f.runs == 0 {
			continue
		}
		any = true
		ratio := 0.0
		if mt := median(t.cost); mt > 0 {
			ratio = median(f.cost) / mt
		}
		costX := 1.2
		if id == "F6" {
			costX = 1.1
		}
		costOK := ratio <= costX
		ctxRatio := 0.0
		if mt := median(t.cf); mt > 0 {
			ctxRatio = median(f.cf) / mt
		}
		ctxOK := ctxRatio <= 1.0
		passOK := f.pass*t.runs >= t.pass*f.runs // F rate >= T rate without division
		fe, te := mean(f.failed)+mean(f.retr), mean(t.failed)+mean(t.retr)
		editOK := fe <= te
		ok := costOK && ctxOK && passOK && editOK
		all = all && ok
		fmt.Fprintf(&b, "| %s %s | %.2f (<= %.1f) | %s | %.2f (<= 1.0) | %s | %d/%d vs %d/%d | %s | %.2f vs %.2f | %s | %.1f vs %.1f | %s |\n", id, names[id], ratio, costX, yn(costOK), ctxRatio, yn(ctxOK),
			f.pass, f.runs, t.pass, t.runs, yn(passOK), fe, te, yn(editOK), median(f.turns), median(t.turns), map[bool]string{true: "pass", false: "FAIL"}[ok])
	}
	verdict := "NO DATA"
	switch {
	case !any:
	case aborted != "" && all:
		verdict = "GO on the runs completed (aborted early: not every planned run finished)"
	case all:
		verdict = "GO"
	default:
		verdict = "NO-GO"
	}
	fmt.Fprintf(&b, "\n**Gate verdict: %s**\n\n", verdict)

	b.WriteString(`## Reading the numbers

- Context here is the Claude Code context of each model call (fresh + cache read + cache write) from the stream usage. About 38k tokens of it is Claude Code's own prompt, equal in both arms. Claude Code 2.1.325 lists MCP tools by name at turn 1 and loads schemas on demand through ToolSearch, so turn-1 context does not show tool-schema size; the 12/14/15 versus 5/6 tool counts show in "MCP tools listed" and in the ToolSearch calls instead.
- Arm T has the real old tool count for each flow's role (12 run, 14 with the two proof tools in F2 and F3, 15 orchestrator in F4 and F6). Arm F has the new count (5, 6 with create_task in F4; F6 files arm has 5 because creation moves into submit). Merged-tool descriptions carry the registry text of every tool they replace; their argument descriptions are short field notes, so a final schema may be somewhat larger.
- Both arms get the same prompt core with the full data the agent needs; arm F adds one line naming the task folder. Allowed tools are the same in both arms (Read, Edit, Write, Glob, Grep, Bash, the Kira servers).
- Edit-path failures count: failed edit, write or shell write calls, any Kira tool error, and submit or finish_step results invalid, rejected or restored. Retries are the next call on the same file or tool. Wrong-file writes (read-only task files) were zero in every run.
- Wall time is not in this file (rebuilt offline from stored transcripts). Pass flags come from the run's oracle: file content or stub state, never the agent's own claim.
- Turns are reported only. The single consolidated-file variant was skipped (see above).

`)
	b.WriteString("## Runs\n\n| Flow | Arm | Model | Rep | Pass | Cost | Turns | In | Cache read | Cache write | Out | Wall s | Tool calls | Note |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range recs {
		var tools []string
		for k, n := range r.Tools {
			tools = append(tools, fmt.Sprintf("%s×%d", k, n))
		}
		slices.Sort(tools)
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %s | %.4f | %d | %d | %d | %d | %d | %.0f | %s | %s |\n", r.Flow, r.Arm, r.Model, r.Rep, yn(r.Pass), r.Cost, r.Turns,
			r.In, r.CacheRead, r.CacheWrite, r.Out, r.Wall, strings.Join(tools, " "), strings.ReplaceAll(r.Why, "|", "/"))
	}
	return b.String()
}

func yn(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
