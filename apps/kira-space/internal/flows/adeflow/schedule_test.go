package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeclock"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/scripts/scriptstest"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// A Monday, 30 s before 09:00 UTC.
var scheduleStart = time.Date(2030, 1, 7, 8, 59, 30, 0, time.UTC)

// tick waits for the scheduler to sleep on its timer, then moves the clock.
func tick(t *testing.T, clock *fakeclock.Clock, d time.Duration) {
	t.Helper()
	if !clock.BlockUntil(1, waitFor) {
		t.Fatal("the scheduler never armed a timer")
	}
	clock.Advance(d)
}

// scheduledRun waits for the n-th scheduled run of a script (0-based, oldest first) to reach state.
func scheduledRun(t *testing.T, f *autoFixture, scriptID string, n int, state string) scriptruns.Run {
	t.Helper()
	var got scriptruns.Run
	testx.WaitUntil(t, waitFor, func() bool {
		all, err := f.app.W.ScriptRuns.List(scriptruns.ListArgs{Limit: 200})
		if err != nil {
			t.Fatal(err)
		}
		var mine []scriptruns.Run
		for _, r := range all {
			if r.ScriptID == scriptID && r.Trigger == scriptruns.TriggerScheduled {
				mine = append(mine, r)
			}
		}
		if len(mine) <= n {
			return false
		}
		got = mine[len(mine)-1-n]
		return got.State == state
	})
	return got
}

func reasonOf(r scriptruns.Run) string {
	if r.Outcome == nil {
		return ""
	}
	return r.Outcome.Reason
}

func targeted(f *autoFixture) *scripts.Schedule {
	return &scripts.Schedule{Cron: "* * * * *", Timezone: "UTC", Enabled: true, TaskID: f.task, BranchID: f.apiBr.ID}
}

func TestScheduleADE(t *testing.T) {
	t.Run("smart run in the worktree holds the claim", func(t *testing.T) {
		clock := fakeclock.New(scheduleStart)
		f := newAutoFixture(t, agentStep("two", "    before: approval\n"), false, flowharness.WithClock(clock))
		fields := smartFields("audit", "Audit {branch} of {task}")
		fields.Schedule = targeted(f)
		rec := f.script(t, fields)

		tick(t, clock, 30*time.Second)
		waitCall(t, f.app, "api", 2)
		run := scheduledRun(t, f, rec.ID, 0, "running")
		if run.TaskID != f.task || run.BranchID != f.apiBr.ID || run.Cwd != f.apiBr.Worktree {
			t.Fatalf("run = %+v, want the api worktree of the task", run)
		}
		env, err := os.ReadFile(filepath.Join(f.app.FakeDir, "api-2.env"))
		if err != nil || !strings.Contains(string(env), "KIRA_BRANCH=feat/api-login") {
			t.Fatalf("env = %q, %v", env, err)
		}

		// The claim holds a step launch back until the run ends.
		step := adewire.StepArgs{TaskID: f.task, StepID: "two"}
		if err := f.app.W.AdeTask.Approve(ctx, step); err != nil {
			t.Fatal(err)
		}
		held, ok := stepRunOn(t, f.app, f.task, "two", f.apiBr.ID)
		if !ok || held.State != "pending" || held.Note != "waiting for script run audit" {
			t.Fatalf("step run = %+v, want pending behind the automation", held)
		}
		release(t, f.gateOne)
		f.run(t, run.ID, "done")

		// The step now runs on the branch: the next tick skips.
		waitCall(t, f.app, "api", 3)
		if !clock.BlockUntil(1, waitFor) {
			t.Fatal("the scheduler never re-armed")
		}
		tick(t, clock, time.Minute)
		skipped := scheduledRun(t, f, rec.ID, 1, "skipped")
		if reasonOf(skipped) != "a run is working on feat/api-login" {
			t.Fatalf("reason = %q", reasonOf(skipped))
		}
		release(t, f.gateTwo)
	})

	t.Run("task archived", func(t *testing.T) {
		clock := fakeclock.New(scheduleStart)
		f := newAutoFixture(t, "", false, flowharness.WithClock(clock))
		fields := smartFields("audit", "Audit {branch}")
		fields.Schedule = targeted(f)
		rec := f.script(t, fields)
		if err := f.app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: f.task}); err != nil {
			t.Fatal(err)
		}
		tick(t, clock, 30*time.Second)
		if r := scheduledRun(t, f, rec.ID, 0, "skipped"); reasonOf(r) != "the task no longer exists" {
			t.Fatalf("reason = %q", reasonOf(r))
		}
	})

	t.Run("normal script claims too", func(t *testing.T) {
		clock := fakeclock.New(scheduleStart)
		f := newAutoFixture(t, agentStep("two", "    before: approval\n"), false, flowharness.WithClock(clock))
		rec := f.script(t, scripts.CustomScriptFields{
			Name: "slow", Command: `echo "branch=$KIRA_BRANCH"; sleep 30`, UseAdeDir: true, Schedule: targeted(f),
		})
		tick(t, clock, 30*time.Second)
		run := scheduledRun(t, f, rec.ID, 0, "running")
		if run.TerminalID != "" || run.Cwd != f.apiBr.Worktree || run.Kind != "script" {
			t.Fatalf("run = %+v, want a headless run in the worktree", run)
		}
		step := adewire.StepArgs{TaskID: f.task, StepID: "two"}
		if err := f.app.W.AdeTask.Approve(ctx, step); err != nil {
			t.Fatal(err)
		}
		held, ok := stepRunOn(t, f.app, f.task, "two", f.apiBr.ID)
		if !ok || held.State != "pending" || held.Note != "waiting for script run slow" {
			t.Fatalf("step run = %+v, want pending behind the script", held)
		}
		if err := f.app.W.ScriptRuns.Stop(scriptruns.IDArgs{ID: run.ID}); err != nil {
			t.Fatal(err)
		}
		f.run(t, run.ID, "cancelled")
		release(t, f.gateOne)
		release(t, f.gateTwo)
	})
}

func TestSchedulePickerListsTasks(t *testing.T) {
	f := newAutoFixture(t, "", false)
	rec := f.script(t, scripts.CustomScriptFields{Name: "nightly", Kind: scripts.KindScript, Command: "echo hi"})

	listed := f.preview(t, scriptruns.RunArgs{ScriptID: rec.ID, ListTasks: true})
	if len(listed.Needs.Tasks) != 1 || listed.Needs.Tasks[0].ID != f.task {
		t.Fatalf("Needs.Tasks = %+v, want the one task", listed.Needs.Tasks)
	}
	f.app.Contract(t, "schedule-task-picker", "ScriptRunsService.Preview#list-tasks", listed, flowharness.Mask("hash"))

	fields := scripts.CustomScriptFields{
		Name: rec.Name, Kind: rec.Kind, Command: rec.Command, Color: rec.Color,
		Schedule: &scripts.Schedule{Cron: "0 9 * * 1-5", Timezone: "UTC", Enabled: true},
	}
	scriptstest.Update(t, f.app.W.CustomScripts, rec.ID, fields)
	args := bridge.CustomScriptsSetScheduleTargetArgs{ID: rec.ID, TaskID: listed.Needs.Tasks[0].ID}
	f.app.Contract(t, "schedule-task-picker", "args:CustomScriptsService.SetScheduleTarget#task", args)
	updated, err := f.app.W.CustomScripts.SetScheduleTarget(args)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Schedule == nil || updated.Schedule.TaskID != f.task {
		t.Fatalf("stored schedule = %+v, want the task kept", updated.Schedule)
	}

	// An unsaved script has no id to preview, so the picker has nothing to list.
	unsaved, err := f.app.W.ScriptRuns.Preview(scriptruns.RunArgs{ListTasks: true})
	f.app.Contract(t, "schedule-task-picker", "ScriptRunsService.Preview#unsaved", map[string]any{"error": errText(err), "tasks": unsaved.Needs.Tasks})
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
