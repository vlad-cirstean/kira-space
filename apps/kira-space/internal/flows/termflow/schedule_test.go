package termflow_test

import (
	"errors"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeclock"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// A Monday, 30 s before 09:00 UTC.
var scheduleStart = time.Date(2030, 1, 7, 8, 59, 30, 0, time.UTC)

func clockApp(t *testing.T, opts ...flowharness.Opt) (*flowharness.App, *fakeclock.Clock) {
	t.Helper()
	clock := fakeclock.New(scheduleStart)
	return flowharness.New(t, append(opts, flowharness.WithClock(clock))...), clock
}

// advance waits for the scheduler loop to be asleep on its timer, then moves the clock.
func advance(t *testing.T, clock *fakeclock.Clock, d time.Duration) {
	t.Helper()
	if !clock.BlockUntil(1, wait) {
		t.Fatal("the scheduler never armed a timer")
	}
	clock.Advance(d)
}

// rearmed waits for the loop to arm again after an advance that consumed every timer.
func rearmed(t *testing.T, clock *fakeclock.Clock) {
	t.Helper()
	if !clock.BlockUntil(1, wait) {
		t.Fatal("the scheduler never re-armed")
	}
}

func recurring(t *testing.T, app *flowharness.App, cron string, mod func(*scripts.CustomScriptFields)) scripts.CustomScript {
	t.Helper()
	f := scripts.CustomScriptFields{
		Name: "tick", Kind: scripts.KindScript, Command: "echo hi", Color: "blue",
		Schedule: &scripts.Schedule{Cron: cron, Timezone: "UTC", Enabled: true},
	}
	if mod != nil {
		mod(&f)
	}
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: f})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func update(t *testing.T, app *flowharness.App, rec scripts.CustomScript, mod func(*scripts.CustomScriptFields)) scripts.CustomScript {
	t.Helper()
	f := scripts.CustomScriptFields{
		Name: rec.Name, Kind: rec.Kind, Command: rec.Command, Params: rec.Params, Smart: rec.Smart,
		Schedule: rec.Schedule, DirMode: rec.DirMode, WorkingDir: rec.WorkingDir, Color: rec.Color,
	}
	mod(&f)
	out, err := app.W.CustomScripts.Update(bridge.CustomScriptsUpdateArgs{ID: rec.ID, Fields: f})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func scheduledRuns(t *testing.T, app *flowharness.App, scriptID string) []scriptruns.Run {
	t.Helper()
	all, err := app.W.ScriptRuns.List(scriptruns.ListArgs{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	var out []scriptruns.Run
	for _, r := range all {
		if r.ScriptID == scriptID && r.Trigger == scriptruns.TriggerScheduled {
			out = append(out, r)
		}
	}
	return out
}

// waitRun waits for the oldest scheduled run of a script in state, counting from the n-th (0-based, oldest first).
func waitSched(t *testing.T, app *flowharness.App, scriptID string, n int, state string) scriptruns.Run {
	t.Helper()
	var got scriptruns.Run
	testx.WaitUntil(t, wait, func() bool {
		runs := scheduledRuns(t, app, scriptID)
		if len(runs) <= n {
			return false
		}
		got = runs[len(runs)-1-n]
		return got.State == state
	})
	return got
}

func logText(t *testing.T, app *flowharness.App, runID string) string {
	t.Helper()
	page, err := app.W.ScriptRuns.ReadLog(scriptruns.ReadLogArgs{ID: runID})
	if err != nil {
		t.Fatal(err)
	}
	texts := make([]string, 0, len(page.Chunks))
	for _, c := range page.Chunks {
		texts = append(texts, c.Text)
	}
	return strings.Join(texts, "\n")
}

func reasonOf(r scriptruns.Run) string {
	if r.Outcome == nil {
		return ""
	}
	return r.Outcome.Reason
}

func TestScheduleValidation(t *testing.T) {
	app := flowharness.New(t)
	for _, c := range []struct {
		name, want string
		mod        func(*scripts.CustomScriptFields)
	}{
		{"four fields", "cron needs 5 fields", func(f *scripts.CustomScriptFields) { f.Schedule.Cron = "* * * *" }},
		{"six fields", "cron needs 5 fields", func(f *scripts.CustomScriptFields) { f.Schedule.Cron = "0 * * * * *" }},
		{"at tag", "cron needs 5 fields", func(f *scripts.CustomScriptFields) { f.Schedule.Cron = "@daily" }},
		{"bad cron", "is not a valid expression", func(f *scripts.CustomScriptFields) { f.Schedule.Cron = "99 * * * *" }},
		{"bad zone", "unknown timezone", func(f *scripts.CustomScriptFields) { f.Schedule.Timezone = "Nope/Zone" }},
		{"timeout low", "between 1m and 24h", func(f *scripts.CustomScriptFields) { f.Schedule.Timeout = "10s" }},
		{"timeout high", "between 1m and 24h", func(f *scripts.CustomScriptFields) { f.Schedule.Timeout = "25h" }},
		{"required secret without a saved value", "needs a saved value for secret param token", func(f *scripts.CustomScriptFields) {
			f.Params = []scripts.Param{{Name: "token", Type: scripts.ParamText, Secret: true, Required: true}}
			f.Schedule.Confirm = false
		}},
		{"task variable without task", "without a task cannot use {branch}", func(f *scripts.CustomScriptFields) { f.Command = "echo {branch}" }},
		{"unknown param", "schedule params", func(f *scripts.CustomScriptFields) { f.Schedule.Params = map[string][]string{"ghost": {"x"}} }},
		{"stored secret", "save its value in the Parameters tab", func(f *scripts.CustomScriptFields) {
			f.Params = []scripts.Param{{Name: "token", Type: scripts.ParamText, Secret: true}}
			f.Schedule.Confirm = true
			f.Schedule.Params = map[string][]string{"token": {"x"}}
		}},
	} {
		f := scripts.CustomScriptFields{
			Name: "bad", Kind: scripts.KindScript, Command: "echo hi", Color: "blue",
			Schedule: &scripts.Schedule{Cron: "0 9 * * *", Timezone: "UTC", Enabled: true, Confirm: true},
		}
		c.mod(&f)
		if _, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: f}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}

	fires, err := app.W.ScriptRuns.NextFires(scriptruns.NextFiresArgs{Cron: "0 9 * * 1-5", Timezone: "UTC", Count: 3})
	if err != nil || len(fires) != 3 {
		t.Fatalf("NextFires = %v, %v", fires, err)
	}
	for _, ms := range fires {
		if wd := time.UnixMilli(ms).UTC().Weekday(); wd == time.Saturday || wd == time.Sunday {
			t.Fatalf("NextFires has a weekend fire: %v", fires)
		}
	}
	if _, err := app.W.ScriptRuns.NextFires(scriptruns.NextFiresArgs{Cron: "* * *", Timezone: "UTC"}); errCode(err) != "E_BAD_REQUEST" ||
		!strings.Contains(err.Error(), "cron needs 5 fields") {
		t.Fatalf("NextFires with a bad cron = %v", err)
	}
}

func TestScheduleHeadless(t *testing.T) {
	t.Run("output and exit", func(t *testing.T) {
		app, clock := clockApp(t)
		rec := recurring(t, app, "* * * * *", func(f *scripts.CustomScriptFields) { f.Command = "echo hi; echo err >&2" })
		advance(t, clock, 30*time.Second)
		run := waitSched(t, app, rec.ID, 0, "done")
		if run.TerminalID != "" || run.Kind != "script" || run.Outcome.Status != "done" {
			t.Fatalf("run = %+v", run)
		}
		if want := filepath.Join(app.SpaceHome, "automations", rec.ID); run.Cwd != want {
			t.Fatalf("cwd = %q, want %q", run.Cwd, want)
		}
		if log := logText(t, app, run.ID); !strings.Contains(log, "hi") || !strings.Contains(log, "err") {
			t.Fatalf("log = %q", log)
		}

		update(t, app, rec, func(f *scripts.CustomScriptFields) { f.Command = "echo bad >&2; exit 3" })
		rearmed(t, clock)
		advance(t, clock, time.Minute)
		failed := waitSched(t, app, rec.ID, 1, "failed")
		if reasonOf(failed) != "exited with status 3" {
			t.Fatalf("reason = %q", reasonOf(failed))
		}
		if failed.Outcome.OutputTail != "bad" {
			t.Fatalf("outputTail = %q, want bad", failed.Outcome.OutputTail)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		app, clock := clockApp(t, flowharness.WithScheduleTimeout(2*time.Second))
		rec := recurring(t, app, "* * * * *", func(f *scripts.CustomScriptFields) { f.Command = "sleep 30" })
		advance(t, clock, 30*time.Second)
		if r := waitSched(t, app, rec.ID, 0, "failed"); reasonOf(r) != "timed out after 2s" {
			t.Fatalf("reason = %q", reasonOf(r))
		}
	})

	t.Run("stop", func(t *testing.T) {
		app, clock := clockApp(t)
		rec := recurring(t, app, "* * * * *", func(f *scripts.CustomScriptFields) { f.Command = "sleep 30" })
		advance(t, clock, 30*time.Second)
		run := waitSched(t, app, rec.ID, 0, "running")
		if err := app.W.ScriptRuns.Stop(scriptruns.IDArgs{ID: run.ID}); err != nil {
			t.Fatal(err)
		}
		if r := waitSched(t, app, rec.ID, 0, "cancelled"); reasonOf(r) != "stopped by you" {
			t.Fatalf("reason = %q", reasonOf(r))
		}
	})
}

func TestScheduleOverlap(t *testing.T) {
	t.Run("script", func(t *testing.T) {
		app, clock := clockApp(t)
		rec := recurring(t, app, "* * * * *", func(f *scripts.CustomScriptFields) { f.Command = "sleep 30" })
		advance(t, clock, 30*time.Second)
		first := waitSched(t, app, rec.ID, 0, "running")
		rearmed(t, clock)
		advance(t, clock, time.Minute)
		skipped := waitSched(t, app, rec.ID, 1, "skipped")
		if !strings.Contains(reasonOf(skipped), "the previous run is still running (started 09:00)") || skipped.Outcome.Source != "schedule" {
			t.Fatalf("skipped = %+v", skipped.Outcome)
		}
		app.Contract(t, "schedule-overlap", "ScriptRunsService.Get#skipped", skipped, flowharness.Mask("createdAt", "startedAt", "finishedAt", "hash", "sessionId"))
		if err := app.W.ScriptRuns.Stop(scriptruns.IDArgs{ID: first.ID}); err != nil {
			t.Fatal(err)
		}
		waitSched(t, app, rec.ID, 0, "cancelled")
		rearmed(t, clock)
		advance(t, clock, time.Minute)
		waitSched(t, app, rec.ID, 2, "running")
	})

	t.Run("smart", func(t *testing.T) {
		app, clock := clockApp(t)
		app.Scenario(scenario(fakeagent.Action{Name: "done", WaitFile: filepath.Join(t.TempDir(), "never")}))
		rec := newSmart(t, app, "wait", func(f *scripts.CustomScriptFields) {
			f.Schedule = &scripts.Schedule{Cron: "* * * * *", Timezone: "UTC", Enabled: true}
		})
		advance(t, clock, 30*time.Second)
		waitSched(t, app, rec.ID, 0, "running")
		rearmed(t, clock)
		advance(t, clock, time.Minute)
		if r := waitSched(t, app, rec.ID, 1, "skipped"); !strings.Contains(reasonOf(r), "still running") {
			t.Fatalf("reason = %q", reasonOf(r))
		}
	})
}

func TestScheduleConfirm(t *testing.T) {
	const window = "w-main"
	app, clock := clockApp(t)
	app.W.Windows.Add(window, 0, nil, func() {})
	if key, err := app.W.PromptsSvc.MainWindow(); err != nil || key != window {
		t.Fatalf("MainWindow = %q, %v", key, err)
	}
	rec := recurring(t, app, "* * * * *", func(f *scripts.CustomScriptFields) {
		f.Command = "echo hello"
		f.Schedule.Confirm = true
	})
	// Contract schedule-confirm: tests/ui/automations-recurring.spec.ts "contract: a waiting run asks,
	// Run starts it headless" reads the same fixture.
	const sc = "schedule-confirm"
	app.Contract(t, sc, "CustomScriptsService.Create", rec)
	advance(t, clock, 30*time.Second)
	waiting := waitSched(t, app, rec.ID, 0, "waiting")
	app.Contract(t, sc, "ScriptRunsService.Get#waiting", waiting, flowharness.Mask("createdAt", "startedAt", "finishedAt", "hash"))
	if waiting.TerminalID != "" || waiting.StartedAt != nil || waiting.Outcome != nil {
		t.Fatalf("waiting = %+v", waiting)
	}
	pv, err := app.W.ScriptRuns.SchedulePreview(scriptruns.ScheduleArgs{ScriptID: rec.ID})
	if err != nil {
		t.Fatal(err)
	}
	if pv.Dir.Path != waiting.Cwd || pv.Command != waiting.Command || pv.Command != "echo hello" {
		t.Fatalf("preview dir %q command %q, waiting cwd %q command %q", pv.Dir.Path, pv.Command, waiting.Cwd, waiting.Command)
	}
	app.Contract(t, sc, "ScriptRunsService.SchedulePreview", pv, flowharness.Mask("hash"))

	rearmed(t, clock)
	advance(t, clock, time.Minute)
	if r := waitSched(t, app, rec.ID, 1, "skipped"); !strings.Contains(reasonOf(r), "still waiting for your answer (due 09:00)") {
		t.Fatalf("reason = %q", reasonOf(r))
	}

	accept := scriptruns.ConfirmArgs{RunID: waiting.ID, Hash: pv.Hash}
	app.Contract(t, sc, "args:ScriptRunsService.ConfirmAccept", accept, flowharness.Mask("hash"))
	started, err := app.W.ScriptRuns.ConfirmAccept(accept)
	if err != nil || started.RunID != waiting.ID {
		t.Fatalf("ConfirmAccept = %+v, %v", started, err)
	}
	app.Contract(t, sc, "ScriptRunsService.ConfirmAccept", started)
	if r := waitSmart(t, app, waiting.ID, "done"); r.Trigger != scriptruns.TriggerScheduled || !strings.Contains(logText(t, app, r.ID), "hello") {
		t.Fatalf("accepted run = %+v", r)
	} else {
		app.Contract(t, sc, "ScriptRunsService.Get#done", r, flowharness.Mask("createdAt", "startedAt", "finishedAt", "hash"))
		page, err := app.W.ScriptRuns.ReadLog(scriptruns.ReadLogArgs{ID: r.ID})
		if err != nil {
			t.Fatal(err)
		}
		app.Contract(t, sc, "ScriptRunsService.ReadLog", page)
	}
	if _, err := app.W.ScriptRuns.ConfirmAccept(scriptruns.ConfirmArgs{RunID: waiting.ID, Hash: pv.Hash}); errCode(err) != "E_CONFLICT" ||
		!strings.Contains(err.Error(), "already answered") {
		t.Fatalf("second accept = %v", err)
	}

	// An edit since the preview changes the hash.
	rearmed(t, clock)
	advance(t, clock, time.Minute)
	next := waitSched(t, app, rec.ID, 2, "waiting")
	update(t, app, rec, func(f *scripts.CustomScriptFields) { f.Command = "echo changed" })
	if _, err := app.W.ScriptRuns.ConfirmAccept(scriptruns.ConfirmArgs{RunID: next.ID, Hash: pv.Hash}); errCode(err) != "E_CONFLICT" {
		t.Fatalf("accept with an old hash = %v, want E_CONFLICT", err)
	}
	if err := app.W.ScriptRuns.ConfirmDecline(scriptruns.IDArgs{ID: next.ID}); err != nil {
		t.Fatal(err)
	}
	if r := waitSmart(t, app, next.ID, "skipped"); reasonOf(r) != "you declined it" || r.Outcome.Source != "user" {
		t.Fatalf("declined = %+v", r.Outcome)
	}

	// Turning the schedule off ends the run that waits.
	rearmed(t, clock)
	advance(t, clock, time.Minute)
	last := waitSched(t, app, rec.ID, 3, "waiting")
	update(t, app, rec, func(f *scripts.CustomScriptFields) {
		off := *f.Schedule
		off.Enabled = false
		f.Schedule = &off
	})
	if r := waitSmart(t, app, last.ID, "skipped"); reasonOf(r) != "the schedule was turned off" {
		t.Fatalf("reason = %q", reasonOf(r))
	}
}

func TestScheduleConfirmSecret(t *testing.T) {
	app, clock := clockApp(t)
	rec := recurring(t, app, "* * * * *", func(f *scripts.CustomScriptFields) {
		f.Command = `test "$KIRA_PARAM_TOKEN" = s3cret`
		f.Params = []scripts.Param{{Name: "token", Type: scripts.ParamText, Secret: true, Required: true}}
		f.Schedule.Confirm = true
	})
	advance(t, clock, 30*time.Second)
	waiting := waitSched(t, app, rec.ID, 0, "waiting")
	bare, err := app.W.ScriptRuns.SchedulePreview(scriptruns.ScheduleArgs{ScriptID: rec.ID})
	if err != nil || len(bare.Missing) != 1 || bare.Missing[0] != "token" {
		t.Fatalf("preview without the secret = %+v, %v", bare.Missing, err)
	}
	secrets := map[string][]string{"token": {"s3cret"}}
	pv, err := app.W.ScriptRuns.SchedulePreview(scriptruns.ScheduleArgs{ScriptID: rec.ID, Secrets: secrets})
	if err != nil || len(pv.Missing) != 0 {
		t.Fatalf("preview with the secret = %+v, %v", pv.Missing, err)
	}
	if _, err := app.W.ScriptRuns.ConfirmAccept(scriptruns.ConfirmArgs{RunID: waiting.ID, Hash: pv.Hash, Secrets: secrets}); err != nil {
		t.Fatal(err)
	}
	done := waitSmart(t, app, waiting.ID, "done")
	for _, p := range done.Params {
		if strings.Contains(p.Value, "s3cret") {
			t.Fatalf("stored param %+v carries the secret", p)
		}
	}
}

func TestScheduleRestart(t *testing.T) {
	app, clock := clockApp(t)
	rec := recurring(t, app, "0 * * * *", func(f *scripts.CustomScriptFields) { f.Schedule.Confirm = true })
	advance(t, clock, 30*time.Second)
	waiting := waitSched(t, app, rec.ID, 0, "waiting")
	rearmed(t, clock)

	app.Restart()
	if r := waitSmart(t, app, waiting.ID, "skipped"); reasonOf(r) != "Kira Space closed before you answered" {
		t.Fatalf("reason = %q", reasonOf(r))
	}
	// 10:00 passed while closed: missed, no catch-up row, and the next fire still comes.
	advance(t, clock, 2*time.Hour+30*time.Minute)
	rearmed(t, clock)
	if n := len(scheduledRuns(t, app, rec.ID)); n != 1 {
		t.Fatalf("scheduled runs after the jump = %d, want the one skipped row", n)
	}
	advance(t, clock, 30*time.Minute)
	waitSched(t, app, rec.ID, 1, "waiting")
	got, err := app.W.CustomScripts.List()
	if err != nil || len(got.Scripts) != 1 || got.Scripts[0].Schedule == nil || !got.Scripts[0].Schedule.Enabled {
		t.Fatalf("scripts = %+v, %v", got, err)
	}
}

func TestRunScheduleNow(t *testing.T) {
	app := flowharness.New(t)
	rec := recurring(t, app, "0 9 * * *", func(f *scripts.CustomScriptFields) {
		f.Command = "sleep 30"
		f.Schedule.Enabled = false
	})
	pv, err := app.W.ScriptRuns.SchedulePreview(scriptruns.ScheduleArgs{ScriptID: rec.ID})
	if err != nil {
		t.Fatal(err)
	}
	args := scriptruns.ScheduleStartArgs{ScheduleArgs: scriptruns.ScheduleArgs{ScriptID: rec.ID}, Hash: pv.Hash}
	// Contract schedule-now, read by tests/ui/automations-recurring.spec.ts "contract: Run now starts
	// the schedule run at once".
	app.Contract(t, "schedule-now", "CustomScriptsService.Create", rec)
	app.Contract(t, "schedule-now", "ScriptRunsService.SchedulePreview", pv, flowharness.Mask("hash"))
	app.Contract(t, "schedule-now", "args:ScriptRunsService.RunScheduleNow", args, flowharness.Mask("hash"))
	started, err := app.W.ScriptRuns.RunScheduleNow(args)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, "schedule-now", "ScriptRunsService.RunScheduleNow", started)
	run := waitSmart(t, app, started.RunID, "running")
	app.Contract(t, "schedule-now", "ScriptRunsService.Get#running", run, flowharness.Mask("createdAt", "startedAt", "finishedAt", "hash"))
	if run.Trigger != scriptruns.TriggerScheduled || run.TerminalID != "" {
		t.Fatalf("run = %+v", run)
	}
	if _, err := app.W.ScriptRuns.RunScheduleNow(args); errCode(err) != "E_CONFLICT" || !strings.Contains(err.Error(), "still running") {
		t.Fatalf("second Run now = %v", err)
	}
	if n := len(scheduledRuns(t, app, rec.ID)); n != 1 {
		t.Fatalf("scheduled runs = %d, want 1 (no skipped row beside a person)", n)
	}
	if err := app.W.ScriptRuns.Stop(scriptruns.IDArgs{ID: run.ID}); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleMissingTask(t *testing.T) {
	app, clock := clockApp(t)
	rec := recurring(t, app, "* * * * *", func(f *scripts.CustomScriptFields) { f.Schedule.TaskID = "task-1" })
	advance(t, clock, 30*time.Second)
	r := waitSched(t, app, rec.ID, 0, "skipped")
	if !strings.Contains(reasonOf(r), "the task no longer exists") {
		t.Fatalf("reason = %q", reasonOf(r))
	}
}

const wait = 20 * time.Second

func errCode(err error) string {
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		return ie.Code
	}
	return ""
}

func scenario(actions ...fakeagent.Action) fakeagent.Scenario {
	return fakeagent.Scenario{Claude: map[string][]fakeagent.Action{"*": actions}}
}

func newSmart(t *testing.T, app *flowharness.App, prompt string, mod func(*scripts.CustomScriptFields)) scripts.CustomScript {
	t.Helper()
	f := scripts.CustomScriptFields{Name: "ask", Kind: scripts.KindSmart, Command: prompt, Color: "blue"}
	if mod != nil {
		mod(&f)
	}
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: f})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func waitSmart(t *testing.T, app *flowharness.App, id, state string) scriptruns.Run {
	t.Helper()
	var r scriptruns.Run
	testx.WaitUntil(t, wait, func() bool {
		got, err := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: id})
		r = got
		return err == nil && got.State == state
	})
	return r
}
