package claude

// P643 Part 5 step 0 (plan 14.2): does the agent call `state` right more often with nested
// {action, args} (arm N) than with the flat union (arm FLAT)? Real claude on sonnet, stub tool, no Kira
// backend. Opt-in, spends real tokens (18 runs, about 1.35 USD), never in hooks or CI:
//
//	KIRA_SPIKE=state KIRA_ACC_OUT=<abs>/docs/v0.3.14/plans/P643-state-accuracy.md \
//	KIRA_SPIKE_DUMP=<scratch>/acc-dump CGO_ENABLED=1 go test -count=1 -v -timeout 2h -run 'TestStateAccuracy$' .
//
// Knobs: KIRA_ACC_REPS (default 3), KIRA_ACC_FLOWS (default A1,A2,A3), KIRA_ACC_ARMS (default N,FLAT),
// KIRA_ACC_BUDGET (USD, default 1.9: estimate plus one run's cap). Free tests: TestStateAccuracyScoring,
// TestStateAccuracyShapes.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

const accRunCap = 0.5

// accRec is a spikeRec plus the call-log scores of one run.
type accRec struct {
	spikeRec
	Calls, ClientCalls int
	Attempts, FTC      int
	Retries            int
	Invalid            map[string]int
}

// accScore walks a call log in order: a call opens an attempt when none is open for its action, an
// invalid reply keeps it open, a valid reply closes it. First-try-correct = closed by its first call.
func accScore(log []accCall) (attempts, ftc, retries int, invalid map[string]int) {
	invalid = map[string]int{}
	type st struct{ open, tainted bool }
	open := map[string]*st{}
	for _, c := range log {
		s := open[c.Action]
		if s == nil {
			s = &st{}
			open[c.Action] = s
		}
		if s.open {
			retries++
		} else {
			attempts++
			s.open, s.tainted = true, false
		}
		if !c.Valid {
			invalid[c.Class]++
			s.tainted = true
			continue
		}
		if !s.tainted {
			ftc++
		}
		s.open = false
	}
	return attempts, ftc, retries, invalid
}

func accFlowPrompt(core string) func(r *spikeRun) string {
	return func(r *spikeRun) string {
		return core + "\n\nTask folder: " + r.taskDir + " (README.md there).\nKira tool: state. Load it first with ToolSearch select:mcp__kira-space__state"
	}
}

var accFlows = []spikeFlow{
	{
		id: "A1", name: "branch wait",
		prompt: accFlowPrompt("Branch feature/x of repo app in this task is being set up. Wait until its setup is done, then reply with the branch's worktree path and nothing else."),
		oracle: func(r *spikeRun, result string) (bool, string) {
			if !strings.Contains(result, r.wt) {
				return false, "result lacks worktree path"
			}
			if time.Now().Before(r.ready) {
				return false, "answered before setup ended"
			}
			for _, c := range accCur.callLog() {
				if c.Valid && c.Action == "branch.request" && c.Subject == "app/feature/x" {
					return true, ""
				}
			}
			return false, "no valid branch.request for app/feature/x"
		},
	},
	{
		id: "A2", name: "two repos",
		prompt: accFlowPrompt("This task needs work in two repos. Create branch feat/billing in repo api from main, and branch feat/billing-ui in repo web from release/2.4. " +
			"Then rename the api branch to feat/billing-api. Then take the lease on queue deploy-staging and release it once you hold it. Reply with the two final branch names, one per line."),
		oracle: func(_ *spikeRun, result string) (bool, string) {
			s := accCur
			s.mu.Lock()
			defer s.mu.Unlock()
			if base, ok := s.task["api"]["feat/billing-api"]; !ok || base != "main" {
				return false, "api feat/billing-api missing or wrong base"
			}
			if _, ok := s.task["api"]["feat/billing"]; ok {
				return false, "api feat/billing not renamed"
			}
			if base, ok := s.task["web"]["feat/billing-ui"]; !ok || base != "release/2.4" {
				return false, "web feat/billing-ui missing or wrong base"
			}
			if len(s.leaseEv) != 2 || s.leaseEv[0].what != "acquired" || s.leaseEv[1].what != "released" || s.held {
				return false, "lease not acquired then released"
			}
			if !strings.Contains(result, "feat/billing-api") || !strings.Contains(result, "feat/billing-ui") {
				return false, "reply lacks the two branch names"
			}
			return true, ""
		},
	},
	{
		id: "A3", name: "change and review reply",
		prompt: accFlowPrompt("Reviewer comment c-17 asks you to switch branches. Point the task's branch feature/x of repo app at the existing local branch feature/x-alt. " +
			"Then reply to comment c-17 with \"Switched to feature/x-alt.\", mark it resolved, and finish the step with status done. Reply with the word done."),
		oracle: func(_ *spikeRun, _ string) (bool, string) {
			s := accCur
			s.mu.Lock()
			defer s.mu.Unlock()
			ci := slices.IndexFunc(s.changes, func(c accChange) bool { return c == accChange{"app", "feature/x", "feature/x-alt"} })
			if ci < 0 {
				return false, "branch change app feature/x to feature/x-alt not recorded"
			}
			if !strings.Contains(s.replies["c-17"], "Switched to feature/x-alt") || !s.resolved["c-17"] {
				return false, "c-17 reply missing or not resolved"
			}
			last := slices.IndexFunc(s.finishes, func(f accFinish) bool {
				return f.status == "done" && f.seq > s.changeAt[ci] && f.seq > s.replySeq["c-17"]
			})
			if last < 0 {
				return false, "step.finish done not recorded after change and reply"
			}
			return true, ""
		},
	},
}

func accList(env, def string) []string {
	v := os.Getenv(env)
	if v == "" {
		v = def
	}
	var s []string
	for _, f := range strings.Split(v, ",") {
		if f = strings.TrimSpace(f); f != "" {
			s = append(s, f)
		}
	}
	return s
}

func TestStateAccuracy(t *testing.T) {
	out := os.Getenv("KIRA_ACC_OUT")
	if os.Getenv("KIRA_SPIKE") != "state" || out == "" {
		t.Skip("P643 state accuracy test spends real tokens; set KIRA_SPIKE=state and KIRA_ACC_OUT (see the file comment)")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("no claude on PATH")
	}
	flows := accList("KIRA_ACC_FLOWS", "A1,A2,A3")
	arms := accList("KIRA_ACC_ARMS", accArmN+","+accArmFlat)
	reps := 3
	if v, err := strconv.Atoi(os.Getenv("KIRA_ACC_REPS")); err == nil && v > 0 {
		reps = v
	}
	budget := 1.9
	if v, err := strconv.ParseFloat(os.Getenv("KIRA_ACC_BUDGET"), 64); err == nil && v > 0 {
		budget = v
	}
	for _, f := range flows {
		if flowByIDs(accFlows, f).oracle == nil {
			t.Fatalf("unknown flow %s", f)
		}
	}
	for _, a := range arms {
		if a != accArmN && a != accArmFlat {
			t.Fatalf("unknown arm %s", a)
		}
	}
	tokens := map[string]int{}
	for _, a := range arms {
		tokens[a] = accSchemaBytes(t, a)
	}

	prevCap := benchRunCapUSD
	benchRunCapUSD = accRunCap
	defer func() { benchRunCapUSD, benchSetup, accCur = prevCap, nil, nil }()
	home := spikeHome(t)

	var recs []accRec
	var spent float64
	note := ""
	planned := len(flows) * len(arms) * reps
loop:
	for rep := 1; rep <= reps; rep++ {
		for _, id := range flows {
			fl := flowByIDs(accFlows, id)
			for _, arm := range arms {
				next := accRunCap // later runs are bounded by the dearest run seen
				if len(recs) > 0 {
					next = 0
					for _, r := range recs {
						next = max(next, r.Cost)
					}
				}
				if spent+next > budget {
					note = fmt.Sprintf("Budget guard: spent %.4f USD, next run could add %.2f, budget %.2f. Remaining runs skipped.", spent, next, budget)
					break loop
				}
				benchSetup = accSetup(arm)
				rec := execute(t, home, fl, arm, "sonnet", rep)
				benchSetup = nil
				log := accCur.callLog()
				ar := accRec{spikeRec: rec, Calls: len(log), ClientCalls: rec.Tools["state"]}
				ar.Attempts, ar.FTC, ar.Retries, ar.Invalid = accScore(log)
				spent += rec.Cost
				recs = append(recs, ar)
				if d := os.Getenv("KIRA_SPIKE_DUMP"); d != "" {
					raw, _ := json.MarshalIndent(log, "", " ")
					_ = os.WriteFile(filepath.Join(d, fmt.Sprintf("%s-%s-sonnet-%d-calls.json", fl.id, arm, rep)), raw, 0o644)
				}
				t.Logf("%s %s #%d pass=%v cost=%.4f turns=%d calls=%d ftc=%d/%d spent=%.4f %s", rec.Flow, rec.Arm, rec.Rep, rec.Pass, rec.Cost, rec.Turns, ar.Calls, ar.FTC, ar.Attempts, spent, rec.Why)
				if err := os.WriteFile(out, []byte(accReport(recs, flows, arms, spent, planned, note, tokens)), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := os.WriteFile(out, []byte(accReport(recs, flows, arms, spent, planned, note, tokens)), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("state accuracy spend %.4f USD over %d runs; results in %s", spent, len(recs), out)
}

func accLabel(arm string) string {
	if arm == accArmFlat {
		return "F"
	}
	return arm
}

type accTotals struct {
	runs, pass, attempts, ftc, retries int
	invalid                            map[string]int
}

func accTotal(recs []accRec, flow, arm string) accTotals {
	g := accTotals{invalid: map[string]int{}}
	for _, r := range recs {
		if r.Arm != arm || (flow != "" && r.Flow != flow) {
			continue
		}
		g.runs++
		if r.Pass {
			g.pass++
		}
		g.attempts += r.Attempts
		g.ftc += r.FTC
		g.retries += r.Retries
		for k, v := range r.Invalid {
			g.invalid[k] += v
		}
	}
	return g
}

func (g accTotals) rate() float64 {
	if g.attempts == 0 {
		return 0
	}
	return float64(g.ftc) / float64(g.attempts)
}

func accInvalidText(m map[string]int) string {
	var parts []string
	for _, k := range []string{"action", "shape", "args-string", "missing", "stray", "value"} {
		if m[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

// accVerdict applies the gate of plan 14.2, never relaxed: N wins when its first-try-correct rate is at
// least FLAT's and it fails no more runs; under 90 percent for the winner (or both) is STOP; a partial
// round has no verdict.
func accVerdict(recs []accRec, planned int) string {
	if len(recs) < planned {
		return fmt.Sprintf("NO VERDICT: partial round, %d of %d runs. The user decides on more spend.", len(recs), planned)
	}
	n, f := accTotal(recs, "", accArmN), accTotal(recs, "", accArmFlat)
	win, w := "N (nested)", n
	if n.rate() < f.rate() || n.runs-n.pass > f.runs-f.pass {
		win, w = "F (flat)", f
	}
	switch {
	case n.rate() < 0.9 && f.rate() < 0.9:
		return fmt.Sprintf("STOP: both arms under 90 percent first-try-correct (N %.1f, F %.1f). Build nothing, report to the user. Gate pick would have been %s.", 100*n.rate(), 100*f.rate(), win)
	case w.rate() < 0.9:
		return fmt.Sprintf("STOP: winner %s under 90 percent first-try-correct (%.1f). Build nothing, report to the user.", win, 100*w.rate())
	}
	return fmt.Sprintf("WINNER: %s. First-try-correct N %.1f percent, F %.1f percent; failed runs N %d, F %d.", win, 100*n.rate(), 100*f.rate(), n.runs-n.pass, f.runs-f.pass)
}

func accReport(recs []accRec, flows, arms []string, spent float64, planned int, note string, tokens map[string]int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# P643 state accuracy\n\nRuns: %d of %d planned, sonnet. Spend: %.4f USD. %s\n\n", len(recs), planned, spent, note)
	b.WriteString("Schema size (`tools/list` JSON of the `state` tool):\n\n| Arm | Bytes | Tokens (bytes/4) |\n|---|---|---|\n")
	for _, a := range arms {
		fmt.Fprintf(&b, "| %s | %d | %d |\n", accLabel(a), tokens[a], tokens[a]/4)
	}
	b.WriteString("\n### Per flow and arm\n\n| Flow | Arm | Runs | Pass | Calls (mean) | Client calls (mean) | Attempts | FTC | FTC rate | Invalid by class | Retries | Cost USD (median) | Final context (median) | Turns (median) | ToolSearch ctx (median) |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	col := func(recs []accRec, flow, arm string, f func(accRec) float64) []float64 {
		var v []float64
		for _, r := range recs {
			if r.Flow == flow && r.Arm == arm {
				v = append(v, f(r))
			}
		}
		return v
	}
	for _, fl := range flows {
		for _, a := range arms {
			g := accTotal(recs, fl, a)
			if g.runs == 0 {
				continue
			}
			fmt.Fprintf(&b, "| %s | %s | %d | %d | %.1f | %.1f | %d | %d | %.1f%% | %s | %d | %.4f | %.0f | %.0f | %.0f |\n", fl, accLabel(a), g.runs, g.pass,
				mean(col(recs, fl, a, func(r accRec) float64 { return float64(r.Calls) })),
				mean(col(recs, fl, a, func(r accRec) float64 { return float64(r.ClientCalls) })),
				g.attempts, g.ftc, 100*g.rate(), accInvalidText(g.invalid), g.retries,
				median(col(recs, fl, a, func(r accRec) float64 { return r.Cost })),
				median(col(recs, fl, a, func(r accRec) float64 { return float64(r.CtxFinal) })),
				median(col(recs, fl, a, func(r accRec) float64 { return float64(r.Turns) })),
				median(col(recs, fl, a, func(r accRec) float64 { return float64(r.SearchCtx) })))
		}
	}
	b.WriteString("\n### Overall per arm\n\n| Arm | Runs | Pass | Failed runs | Attempts | FTC | FTC rate | Invalid by class | Retries | Cost USD (sum) |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, a := range arms {
		g := accTotal(recs, "", a)
		var cost float64
		for _, r := range recs {
			if r.Arm == a {
				cost += r.Cost
			}
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %.1f%% | %s | %d | %.4f |\n", accLabel(a), g.runs, g.pass, g.runs-g.pass, g.attempts, g.ftc, 100*g.rate(), accInvalidText(g.invalid), g.retries, cost)
	}
	fmt.Fprintf(&b, "\n### Verdict\n\n%s\n", accVerdict(recs, planned))
	b.WriteString("\n### Runs\n\n| Flow | Arm | Rep | Pass | Calls | Client | Attempts | FTC | Invalid | Retries | Cost | Final ctx | Turns | Why |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range recs {
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %d | %d | %d | %d | %s | %d | %.4f | %d | %d | %s |\n", r.Flow, accLabel(r.Arm), r.Rep, yn(r.Pass), r.Calls, r.ClientCalls,
			r.Attempts, r.FTC, accInvalidText(r.Invalid), r.Retries, r.Cost, r.CtxFinal, r.Turns, strings.ReplaceAll(r.Why, "|", "/"))
	}
	return b.String()
}

func TestStateAccuracyScoring(t *testing.T) {
	v := func(a string) accCall { return accCall{Action: a, Valid: true} }
	bad := func(a, class string) accCall { return accCall{Action: a, Class: class} }
	tests := []struct {
		name                   string
		log                    []accCall
		attempts, ftc, retries int
		invalid                map[string]int
	}{
		{"two valid calls of one action", []accCall{v("a"), v("a")}, 2, 2, 0, map[string]int{}},
		{"invalid then valid", []accCall{bad("a", "missing"), v("a")}, 1, 0, 1, map[string]int{"missing": 1}},
		{"invalid twice then valid", []accCall{bad("a", "shape"), bad("a", "stray"), v("a")}, 1, 0, 2, map[string]int{"shape": 1, "stray": 1}},
		{"invalid, other action, valid", []accCall{bad("a", "value"), v("b"), v("a")}, 2, 1, 1, map[string]int{"value": 1}},
		{"never closed", []accCall{bad("a", "action")}, 1, 0, 0, map[string]int{"action": 1}},
		{"valid after a closed invalid attempt is a fresh attempt", []accCall{bad("a", "missing"), v("a"), v("a")}, 2, 1, 1, map[string]int{"missing": 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, f, r, inv := accScore(tc.log)
			if a != tc.attempts || f != tc.ftc || r != tc.retries || !mapsEqual(inv, tc.invalid) {
				t.Fatalf("got attempts %d ftc %d retries %d invalid %v, want %d %d %d %v", a, f, r, inv, tc.attempts, tc.ftc, tc.retries, tc.invalid)
			}
		})
	}
}

func mapsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// accCallJSON builds a call from an action and its fields in the arm's shape.
func accCallJSON(arm, action string, fields map[string]any) json.RawMessage {
	m := map[string]any{"action": action}
	if arm == accArmN {
		m["args"] = fields
	} else {
		for k, v := range fields {
			m[k] = v
		}
	}
	raw, _ := json.Marshal(m)
	return raw
}

func TestStateAccuracyShapes(t *testing.T) {
	valid := map[string]map[string]any{
		"repo.declare":     {"repos": []string{"api", "app"}},
		"branch.request":   {"repo": "app", "branch": "feature/x", "base": "main", "existing": false},
		"branch.rename":    {"repo": "app", "from": "feature/x", "to": "feature/y"},
		"branch.change":    {"repo": "app", "from": "feature/x", "to": "feature/x-alt"},
		"lease.acquire":    {"queue": "deploy-staging"},
		"lease.release":    {},
		"operation.cancel": {"opId": "op-7"},
		"remote.fetch":     {"remote": "origin"},
		"branch.push":      {"force": true},
		"review.reply":     {"id": "c-17", "reply": "ok", "resolved": true},
		"step.finish":      {"status": "failed", "reason": "x", "conflictedFiles": []string{"a"}, "lastGitError": "e", "tried": "t"},
	}
	if len(valid) != len(accActions) {
		t.Fatalf("%d valid calls for %d actions", len(valid), len(accActions))
	}
	for _, arm := range []string{accArmN, accArmFlat} {
		t.Run(arm, func(t *testing.T) {
			var want map[string]map[string]json.RawMessage
			for _, a := range accActions {
				d, inv := accDecode(arm, accCallJSON(arm, a.name, valid[a.name]))
				if inv != nil || d.action != a.name {
					t.Fatalf("%s: %+v", a.name, inv)
				}
				if want == nil {
					want = map[string]map[string]json.RawMessage{}
				}
				want[a.name] = d.fields
				if len(d.fields) != len(valid[a.name]) {
					t.Fatalf("%s: decoded %d fields, sent %d", a.name, len(d.fields), len(valid[a.name]))
				}
			}
			bad := map[string]struct {
				raw   string
				class string
			}{
				"unknown action": {`{"action":"branch.make"}`, "action"},
				"no action":      {`{}`, "action"},
				"missing":        {string(accCallJSON(arm, "branch.request", map[string]any{"repo": "app"})), "missing"},
				"stray":          {string(accCallJSON(arm, "lease.acquire", map[string]any{"queue": "q", "wait": 5})), "stray"},
				"wrong type":     {string(accCallJSON(arm, "branch.push", map[string]any{"force": "yes"})), "value"},
			}
			if arm == accArmN {
				bad["flat call"] = struct{ raw, class string }{`{"action":"lease.acquire","queue":"q"}`, "shape"}
				bad["args string"] = struct{ raw, class string }{`{"action":"lease.acquire","args":"{\"queue\":\"q\"}"}`, "args-string"}
				bad["args not object"] = struct{ raw, class string }{`{"action":"lease.acquire","args":[1]}`, "shape"}
			} else {
				bad["nested call"] = struct{ raw, class string }{`{"action":"lease.acquire","args":{"queue":"q"}}`, "shape"}
			}
			for name, tc := range bad {
				_, inv := accDecode(arm, json.RawMessage(tc.raw))
				if inv == nil || inv.class != tc.class {
					t.Fatalf("%s: got %+v, want class %s", name, inv, tc.class)
				}
				if inv.fix == "" || strings.Contains(inv.fix, `"args"`) != (arm == accArmN) {
					t.Fatalf("%s: fix not in the %s shape: %q", name, arm, inv.fix)
				}
			}
			if n := accSchemaBytes(t, arm); n == 0 {
				t.Fatal("empty tools/list")
			}
		})
	}
	st := newAccStub(accArmN, t.TempDir(), time.Now())
	st.approve, st.setup, st.lease = 0, 0, 0
	d, _ := accDecode(accArmN, accCallJSON(accArmN, "branch.rename", map[string]any{"repo": "app", "from": "nope", "to": "y"}))
	st.mu.Lock()
	inv, _ := st.checkLocked(d)
	st.mu.Unlock()
	if inv == nil || inv.class != "value" {
		t.Fatalf("rename of a branch not on the task: %+v", inv)
	}
}
