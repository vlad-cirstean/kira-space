package promptflow

import (
	"errors"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeclock"
	"github.com/kirathecat/kira-studio/internal/flowtest/notifysink"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const wait = 20 * time.Second

// A Monday, 30 s before 09:00 UTC.
var start = time.Date(2030, 1, 7, 8, 59, 30, 0, time.UTC)

func newApp(t *testing.T) (*studioflowharness.App, *notifysink.Sink, *fakeclock.Clock) {
	t.Helper()
	clock := fakeclock.New(start)
	app := studioflowharness.New(t, studioflowharness.WithClock(clock))
	sink := notifysink.New()
	app.W.Prompts.SetSink(sink)
	return app, sink, clock
}

// advance waits for the scheduler loop to be asleep on its timer, then moves the clock.
func advance(t *testing.T, clock *fakeclock.Clock, d time.Duration) {
	t.Helper()
	if !clock.BlockUntil(1, wait) {
		t.Fatal("the scheduler never armed a timer")
	}
	clock.Advance(d)
}

func addWindow(app *studioflowharness.App, key string, order int) {
	app.W.Windows.Add(key, order, nil, func() {})
}

func code(err error) string {
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		return ie.Code
	}
	return ""
}

// listed waits until the router lists exactly n prompts of kind and returns them.
func listed(t *testing.T, app *studioflowharness.App, kind prompts.Kind, n int) []prompts.Routed {
	t.Helper()
	var got []prompts.Routed
	testx.WaitUntil(t, wait, func() bool {
		got = got[:0]
		all, err := app.W.PromptsSvc.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range all {
			if p.Kind == kind {
				got = append(got, p)
			}
		}
		return len(got) == n
	})
	return got
}

func count(t *testing.T, app *studioflowharness.App, kind prompts.Kind) int {
	t.Helper()
	all, err := app.W.PromptsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, p := range all {
		if p.Kind == kind {
			n++
		}
	}
	return n
}

func target(t *testing.T, app *studioflowharness.App, id string) string {
	t.Helper()
	all, err := app.W.PromptsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		if p.ID == id {
			return p.Target
		}
	}
	t.Fatalf("prompt %q not listed", id)
	return ""
}

func confirmScript(t *testing.T, app *studioflowharness.App, name string) scripts.CustomScript {
	t.Helper()
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: name, Kind: scripts.KindScript, Command: "echo hi", Color: "blue",
		Schedule: &scripts.Schedule{Cron: "* * * * *", Timezone: "UTC", Enabled: true, Confirm: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestScheduleConfirmRoutes(t *testing.T) {
	app, sink, clock := newApp(t)
	// "b" has the lower order, so it is the main window although "a" sorts first.
	addWindow(app, "b", 0)
	addWindow(app, "a", 1)
	if key, err := app.W.PromptsSvc.MainWindow(); err != nil || key != "b" {
		t.Fatalf("MainWindow = %q, %v; want b", key, err)
	}

	// The first script waits after 09:00; at 09:01 it is skipped (still waiting) and the second waits.
	confirmScript(t, app, "first")
	advance(t, clock, 30*time.Second)
	listed(t, app, prompts.KindSchedule, 1)
	confirmScript(t, app, "second")
	// A created script joins the scheduler loop asynchronously, so its first fire can land a minute
	// later; advance a minute at a time until it waits.
	for range 4 {
		if count(t, app, prompts.KindSchedule) == 2 {
			break
		}
		advance(t, clock, time.Minute)
		deadline := time.Now().Add(time.Second)
		for count(t, app, prompts.KindSchedule) < 2 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
	}
	entries := listed(t, app, prompts.KindSchedule, 2)
	for _, e := range entries {
		if e.Target != "b" {
			t.Fatalf("%s targets %q, want b", e.ID, e.Target)
		}
	}
	app.Contract(t, "prompts-route", "PromptsService.List#two-waiting", entries, studioflowharness.Mask("createdAt"))
	note, ok := sink.Shown("prompt:schedule")
	if !ok || note.Title != "2 scripts wait for confirmation" {
		t.Fatalf("shown note = %+v, %v; want the count title", note, ok)
	}

	first := entries[0].ID
	app.W.Windows.RemoveAndCount("b")
	if got := target(t, app, first); got != "a" {
		t.Fatalf("after b closed, target = %q, want a", got)
	}
	app.W.Windows.RemoveAndCount("a")
	if got := target(t, app, first); got != "" {
		t.Fatalf("with no window, target = %q, want queued", got)
	}
	addWindow(app, "c", 2)
	if got := target(t, app, first); got != "c" {
		t.Fatalf("after c opened, target = %q, want c", got)
	}

	addWindow(app, "d", 3)
	if err := app.W.PromptsSvc.Claim(prompts.ClaimArgs{ID: first, WindowKey: "d"}); err != nil {
		t.Fatal(err)
	}
	if got := target(t, app, first); got != "d" {
		t.Fatalf("claimed target = %q, want d", got)
	}
	if err := app.W.PromptsSvc.Claim(prompts.ClaimArgs{ID: "schedule:nope", WindowKey: "d"}); code(err) != "E_NOT_FOUND" {
		t.Fatalf("claim of an unknown id = %v, want E_NOT_FOUND", err)
	}

	// Answering one run withdraws its popup; the other keeps the kind's note, now with its own title.
	ref := entries[0].Ref
	pv, err := app.W.ScriptRuns.SchedulePreview(scriptruns.ScheduleArgs{ScriptID: runScript(t, app, ref)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.ScriptRuns.ConfirmAccept(scriptruns.ConfirmArgs{RunID: ref, Hash: pv.Hash}); err != nil {
		t.Fatal(err)
	}
	listed(t, app, prompts.KindSchedule, 1)
	if _, ok := sink.Shown("prompt:schedule"); !ok {
		t.Fatal("the note vanished while one run still waits")
	}
	if err := app.W.ScriptRuns.ConfirmDecline(scriptruns.IDArgs{ID: entries[1].Ref}); err != nil {
		t.Fatal(err)
	}
	listed(t, app, prompts.KindSchedule, 0)
	testx.WaitUntil(t, wait, func() bool { _, ok := sink.Shown("prompt:schedule"); return !ok })
}

func runScript(t *testing.T, app *studioflowharness.App, runID string) string {
	t.Helper()
	runs, err := app.W.ScriptRuns.List(scriptruns.ListArgs{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range runs {
		if r.ID == runID {
			return r.ScriptID
		}
	}
	t.Fatalf("run %q not found", runID)
	return ""
}

func TestUpdatePrompt(t *testing.T) {
	app, sink, _ := newApp(t)
	addWindow(app, "w1", 0)

	if err := app.W.PromptsSvc.Raise(prompts.RaiseArgs{Kind: prompts.KindUpdate, Ref: "1.3.0"}); err != nil {
		t.Fatal(err)
	}
	if err := app.W.PromptsSvc.Raise(prompts.RaiseArgs{Kind: prompts.KindUpdate, Ref: "1.3.0"}); err != nil {
		t.Fatal(err)
	}
	if got := listed(t, app, prompts.KindUpdate, 1); got[0].Target != "w1" || got[0].ID != "update:1.3.0" {
		t.Fatalf("update entry = %+v", got[0])
	}
	if _, ok := sink.Shown("prompt:update"); !ok {
		t.Fatal("no update note")
	}

	if err := app.W.PromptsSvc.Raise(prompts.RaiseArgs{Kind: prompts.KindApproval, Ref: "x"}); code(err) != "E_CONTRACT" {
		t.Fatalf("Raise of a non-update kind = %v, want E_CONTRACT", err)
	}
	if err := app.W.PromptsSvc.Dismiss(prompts.IDArgs{ID: "approval:x"}); code(err) != "E_CONTRACT" {
		t.Fatalf("Dismiss of a non-update id = %v, want E_CONTRACT", err)
	}
	if err := app.W.PromptsSvc.Dismiss(prompts.IDArgs{ID: "update:1.3.0"}); err != nil {
		t.Fatal(err)
	}
	listed(t, app, prompts.KindUpdate, 0)
	if _, ok := sink.Shown("prompt:update"); ok {
		t.Fatal("the update note stayed after Dismiss")
	}
}

func TestNotifyPromptsSetting(t *testing.T) {
	app, sink, _ := newApp(t)
	addWindow(app, "w1", 0)

	if err := app.W.PromptsSvc.SendTest(); err != nil {
		t.Fatal(err)
	}
	if _, ok := sink.Shown("prompt:test"); !ok {
		t.Fatal("SendTest posted nothing")
	}

	off := false
	if _, err := app.W.StudioSettings.Set(bridge.StudioSettingsSetArgs{Patch: model.SettingsPatch{Advanced: &model.AdvancedPatch{NotifyPrompts: &off}}}); err != nil {
		t.Fatal(err)
	}
	before := sink.Sends()
	if err := app.W.PromptsSvc.SendTest(); err != nil {
		t.Fatal(err)
	}
	if err := app.W.PromptsSvc.Raise(prompts.RaiseArgs{Kind: prompts.KindUpdate, Ref: "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	listed(t, app, prompts.KindUpdate, 1)
	if got := sink.Sends(); got != before {
		t.Fatalf("%d notes posted with notifyPrompts off, want 0", got-before)
	}
}
