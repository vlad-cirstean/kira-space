package dbmcpflow

import (
	"slices"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeclock"
	"github.com/kirathecat/kira-studio/internal/flowtest/notifysink"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// An approval raises the window once per burst, for both stages, until dbMcp.focusOnApproval is off.
// Contract dbmcp-focus: tests/ui/dbmcp-focus.spec.ts drives the setting switch.
func TestDbMcpApprovalFocus(t *testing.T) {
	clock := fakeclock.New(time.Date(2030, 1, 7, 9, 0, 0, 0, time.UTC))
	f := newFixture(t, studioflowharness.WithClock(clock))
	sink := notifysink.New()
	f.app.W.Prompts.SetSink(sink)
	f.app.W.Windows.Add("w1", 0, nil, func() {})
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	const write = "insert into notes (body) values ('three')"
	// away is the user in another app, long after the last raise.
	away := func() {
		f.app.Focus.Blur()
		clock.Advance(3 * time.Second)
	}
	raised := func(n int) {
		t.Helper()
		testx.WaitUntil(t, 10*time.Second, func() bool { return len(f.app.Focus.Keys()) == n })
	}

	away()
	first := callAsync(sess, f.connID, write)
	req := f.pending(t)
	raised(1)
	if keys := f.app.Focus.Keys(); !slices.Equal(keys, []string{"w1"}) {
		t.Fatalf("raised %v, want [w1]", keys)
	}
	entry := f.dbmcpEntries(t, 1)[0]
	f.app.Events.Wait(t, bridge.ChannelPromptsReveal, func(e studioflowharness.Event) bool {
		var p struct{ ID string }
		e.Decode(t, &p)
		return e.Window == "w1" && p.ID == entry.ID
	}, 10*time.Second)

	// Inside the quiet window a second request queues without another raise.
	f.app.Focus.Blur()
	second := callAsync(sess, f.connID, write)
	f.dbmcpEntries(t, 2)

	// Answering raises nothing, queue advance included.
	f.answer(t, false, req.RequestID)
	wait(t, first)
	f.answer(t, false, f.pending(t).RequestID)
	wait(t, second)
	f.dbmcpEntries(t, 0)

	// Preview on: the query stage and the response stage each raise.
	f.setPreview(t, true)
	away()
	third := callAsync(sess, f.connID, write)
	raised(2)
	away()
	f.answer(t, true, f.pending(t).RequestID)
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
	raised(3)
	f.answer(t, true, f.pending(t).RequestID)
	wait(t, third)

	// Off: the window stays put and the notification still shows.
	off := false
	args := bridge.StudioSettingsSetArgs{Patch: model.SettingsPatch{DbMcp: &model.DbMcpPatch{FocusOnApproval: &off}}}
	if _, err := f.app.W.StudioSettings.Set(args); err != nil {
		t.Fatal(err)
	}
	f.app.Contract(t, "dbmcp-focus", "args:SettingsService.Set", args)
	away()
	fourth := callAsync(sess, f.connID, write)
	f.dbmcpEntries(t, 1)
	testx.WaitUntil(t, 10*time.Second, func() bool { _, ok := sink.Shown("prompt:approval"); return ok })
	// Nothing to wait on for a call that must not happen; give a stray goroutine time to show itself.
	time.Sleep(300 * time.Millisecond)
	if keys := f.app.Focus.Keys(); len(keys) != 3 {
		t.Fatalf("raised %v with the setting off, want no new raise", keys)
	}
	f.answer(t, false, f.pending(t).RequestID)
	wait(t, fourth)
}
