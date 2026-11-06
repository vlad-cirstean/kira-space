package captureflow_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakeacli"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakeaz"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost/azure"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker/jira"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeclock"
	"github.com/kirathecat/kira-studio/internal/shell"
)

var ctx = context.Background()

// Contract capture: tests/ui/capture.spec.ts and settings-capture.spec.ts read these.
const scenario = "capture"

const (
	ghLink    = "https://github.com/acme/api/pull/7"
	azLink    = "https://dev.azure.com/acme/Web%20App/_git/api/pullrequest/12"
	jiraSite  = "acme.atlassian.net"
	jiraLink  = "https://acme.atlassian.net/browse/PROJ-12"
	forgeLink = "https://forge.test/acme/api/-/merge_requests/12"
)

type rig struct {
	app *flowharness.App
}

// newRig boots an app with GitHub, Azure Repos (fake az), a Forge and Jira (fake acli) all signed in.
func newRig(t *testing.T, opts ...flowharness.Opt) *rig {
	t.Helper()
	forge := fakehost.New()
	az := fakeaz.New()
	az.SetDefaults("https://dev.azure.com/acme", "Web App")
	acli := fakeacli.New(jiraSite)
	acli.SetIssue("PROJ-12", fakeacli.IssueJSON("PROJ-12", "Export report as CSV", "In Progress", "indeterminate", 0))
	clock := fakeclock.New(time.Now())
	all := append([]flowharness.Opt{
		flowharness.WithGitHost(forge), flowharness.WithClock(clock),
		flowharness.WithAzure(azure.Options{Locator: az, Runner: az, Clock: clock}),
		flowharness.WithIssueTracker(jira.Options{Locator: acli, Runner: acli, Clock: clock}),
	}, opts...)
	return &rig{app: flowharness.New(t, all...)}
}

// press copies text and presses the default hotkey; it returns the popup key and its item.
func (r *rig) press(t *testing.T, text string) (string, bridge.CaptureItem) {
	t.Helper()
	r.app.Clipboard.SetText(text)
	return r.fire(t)
}

func (r *rig) fire(t *testing.T) (string, bridge.CaptureItem) {
	t.Helper()
	r.app.Shortcuts.Fire(t, model.DefaultCaptureHotkey)
	opened := r.app.WindowMgr.OpenedPopups
	if len(opened) == 0 {
		t.Fatal("hotkey opened no popup")
	}
	key := opened[len(opened)-1]
	return key, r.item(t, key)
}

func (r *rig) item(t *testing.T, key string) bridge.CaptureItem {
	t.Helper()
	item, err := r.app.W.Capture.WindowItem(ctx, bridge.CaptureWindowArgs{WindowKey: key})
	if err != nil || item == nil {
		t.Fatalf("WindowItem(%s) = %+v, %v", key, item, err)
	}
	return *item
}

func (r *rig) confirm(t *testing.T, key string, it bridge.CaptureItem, title string, day *string) bridge.CaptureResult {
	t.Helper()
	res, err := r.app.W.Capture.Confirm(ctx, bridge.CaptureConfirmArgs{WindowKey: key, ItemID: it.ID, Title: title, Day: day})
	if err != nil {
		t.Fatalf("Confirm = %v", err)
	}
	return res
}

func (r *rig) task(t *testing.T, id string) adewire.Task {
	t.Helper()
	board, err := r.app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range board.Tasks {
		if tk.ID == id {
			return tk
		}
	}
	t.Fatalf("task %s not on the board", id)
	return adewire.Task{}
}

func (r *rig) status(t *testing.T) shell.HotkeyState {
	t.Helper()
	st, err := r.app.W.Capture.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func (r *rig) setCapture(p model.CapturePatch) error {
	_, err := r.app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{Capture: &p}})
	return err
}

func str(s string) *string { return &s }
func yes(b bool) *bool     { return &b }

func isOpen(t *testing.T, r *rig, key string, closed bool) {
	t.Helper()
	if got := slices.Contains(r.app.WindowMgr.Closed, key); got != closed {
		t.Errorf("popup %s closed = %v, want %v", key, got, closed)
	}
}

// The default chord is registered once at boot and the binding reports active.
func TestBootRegistersDefaultChord(t *testing.T) {
	r := newRig(t)
	if got := r.app.Shortcuts.Registered(); !slices.Equal(got, []string{model.DefaultCaptureHotkey}) || r.app.Shortcuts.Registers() != 1 {
		t.Fatalf("registered %v (%d register calls), want only %s once", got, r.app.Shortcuts.Registers(), model.DefaultCaptureHotkey)
	}
	st := r.status(t)
	if st.State != shell.HotkeyActive || st.Accelerator != model.DefaultCaptureHotkey {
		t.Fatalf("Status = %+v", st)
	}
	r.app.Contract(t, scenario, "CaptureService.Status#active", st)
	if n := r.app.Clipboard.Reads(); n != 0 {
		t.Fatalf("clipboard read %d times before any press", n)
	}
}

// Change request links classify with a provider label; the confirm args are the UI's contract.
func TestChangeRequestLinks(t *testing.T) {
	r := newRig(t)
	for _, c := range []struct{ name, link, label string }{
		{"github", ghLink, "PR #7 acme/api"},
		{"azure", azLink, "PR #12 acme/Web App/api"},
		{"forge", forgeLink, "MR #12 acme/api"},
	} {
		t.Run(c.name, func(t *testing.T) {
			key, it := r.press(t, "  "+c.link+"\n")
			if it.Kind != bridge.CaptureKindChangeRequest || it.Label != c.label || it.Text != c.link {
				t.Fatalf("item = %+v", it)
			}
			if c.name == "github" {
				r.app.Contract(t, scenario, "CaptureService.WindowItem#changeRequest", it, flowharness.Mask("id"))
				r.app.Contract(t, scenario, "args:CaptureService.Confirm", bridge.CaptureConfirmArgs{
					WindowKey: key, ItemID: it.ID, Title: it.Title, Day: str("2026-03-02"),
				}, flowharness.Mask("windowKey", "itemId"))
			}
		})
	}
}

// A Jira key and a Jira link classify as the same issue.
func TestIssueKeyAndLink(t *testing.T) {
	r := newRig(t)
	_, it := r.press(t, "PROJ-12")
	if it.Kind != bridge.CaptureKindIssue || it.Label != "Jira PROJ-12" {
		t.Fatalf("item = %+v", it)
	}
	r.app.Contract(t, scenario, "CaptureService.WindowItem#issue", it, flowharness.Mask("id"))
	_, it2 := r.press(t, jiraLink)
	if it2.Kind != bridge.CaptureKindIssue || it2.Label != it.Label {
		t.Fatalf("link item = %+v", it2)
	}
}

// Other text goes to Later with the edited title, the full text in notes, at the top of the shelf.
func TestTextToLater(t *testing.T) {
	r := newRig(t)
	text := "\n  Fix the export header\nsecond line with detail\n"
	key, it := r.press(t, text)
	if it.Kind != bridge.CaptureKindText || it.Title != "Fix the export header" {
		t.Fatalf("item = %+v", it)
	}
	r.app.Contract(t, scenario, "CaptureService.WindowItem#text", it, flowharness.Mask("id"))
	older, err := r.app.W.AdeTask.CaptureTask(ctx, adewire.CaptureTaskArgs{Title: "Older"})
	if err != nil {
		t.Fatal(err)
	}
	res := r.confirm(t, key, it, "  Export header  ", nil)
	r.app.Contract(t, scenario, "CaptureService.Confirm#later", res, flowharness.Mask("taskId"))
	tk := r.task(t, res.TaskID)
	if tk.Title != "Export header" || tk.Notes != strings.TrimSpace(text) || tk.HostURL != "" || tk.Jira != nil {
		t.Fatalf("task = %+v", tk)
	}
	if hs, err := r.app.W.AdeTask.Handovers(ctx, adewire.TaskArgs{TaskID: res.TaskID}); err != nil || len(hs.Handovers) != 1 ||
		hs.Handovers[0].State != "note" || hs.Handovers[0].Source != "user" || hs.Handovers[0].Body != tk.Notes {
		t.Fatalf("captured text did not land as one user note: %+v, %v", hs, err)
	}
	board, _ := r.app.W.AdeTask.Board(ctx)
	var order []string
	for _, b := range board.Tasks {
		order = append(order, b.ID)
	}
	if slices.Index(order, res.TaskID) > slices.Index(order, older.ID) {
		t.Errorf("captured task is below the older one: %v", order)
	}
	if _, onDay := board.Plan.Day[res.TaskID]; onDay {
		t.Errorf("a Later capture has a plan day: %v", board.Plan.Day)
	}
	isOpen(t, r, key, true)
}

// A day lands the task at the top of that day; a malformed day is refused and adds nothing.
func TestTodayLandsOnDay(t *testing.T) {
	r := newRig(t)
	day := "2026-03-02"
	older, err := r.app.W.AdeTask.CaptureTask(ctx, adewire.CaptureTaskArgs{Title: "Older", Day: &day})
	if err != nil {
		t.Fatal(err)
	}
	key, it := r.press(t, "Ship the report")
	if _, err := r.app.W.Capture.Confirm(ctx, bridge.CaptureConfirmArgs{WindowKey: key, ItemID: it.ID, Title: "Ship", Day: str("2026-13-40")}); err == nil {
		t.Fatal("Confirm with a bad day succeeded")
	}
	isOpen(t, r, key, false)
	res := r.confirm(t, key, it, "Ship", &day)
	board, _ := r.app.W.AdeTask.Board(ctx)
	if got := board.Plan.Day[res.TaskID]; got != day {
		t.Fatalf("plan day = %q, want %q", got, day)
	}
	if slices.Index(board.Plan.Order, res.TaskID) > slices.Index(board.Plan.Order, older.ID) {
		t.Errorf("captured task is below the older one: %v", board.Plan.Order)
	}
}

// A change request in a workspace without its repo still becomes a task with the link; a notice says
// the branch is missing, the popup stays and a repeat Confirm is refused. An issue keeps its key.
func TestChangeRequestUnknownRepo(t *testing.T) {
	r := newRig(t)
	key, it := r.press(t, ghLink)
	res := r.confirm(t, key, it, "Review the API change", nil)
	if !strings.Contains(res.Notice, "No repo in your workspace") {
		t.Fatalf("notice = %q", res.Notice)
	}
	r.app.Contract(t, scenario, "CaptureService.Confirm#noBranch", res, flowharness.Mask("taskId"))
	tk := r.task(t, res.TaskID)
	if tk.HostURL != ghLink || tk.Notes != ghLink || tk.HostLink == nil || len(tk.BranchIDs) != 0 {
		t.Fatalf("PR task = %+v", tk)
	}
	isOpen(t, r, key, false)
	if _, err := r.app.W.Capture.Confirm(ctx, bridge.CaptureConfirmArgs{WindowKey: key, ItemID: it.ID, Title: "again"}); err == nil {
		t.Fatal("repeat Confirm after a notice succeeded")
	}

	key, it = r.press(t, "PROJ-12")
	res = r.confirm(t, key, it, "Export CSV", nil)
	if tk := r.task(t, res.TaskID); tk.Jira == nil || tk.Jira.Key != "PROJ-12" || res.Notice != "" {
		t.Fatalf("issue task = %+v, notice %q", tk, res.Notice)
	}
	isOpen(t, r, key, true)
}

// An empty clipboard shows the empty state: Confirm refused, Dismiss closes.
func TestEmptyClipboard(t *testing.T) {
	r := newRig(t)
	r.app.Clipboard.SetEmpty()
	key, it := r.fire(t)
	if it.Kind != bridge.CaptureKindEmpty {
		t.Fatalf("item = %+v", it)
	}
	r.app.Contract(t, scenario, "CaptureService.WindowItem#empty", it, flowharness.Mask("id"))
	if _, err := r.app.W.Capture.Confirm(ctx, bridge.CaptureConfirmArgs{WindowKey: key, ItemID: it.ID, Title: "x"}); err == nil {
		t.Fatal("Confirm on an empty item succeeded")
	}
	if err := r.app.W.Capture.Dismiss(ctx, bridge.CaptureWindowArgs{WindowKey: key}); err != nil {
		t.Fatal(err)
	}
	isOpen(t, r, key, true)
	if err := r.app.W.Capture.Dismiss(ctx, bridge.CaptureWindowArgs{WindowKey: key}); err != nil {
		t.Fatalf("second Dismiss = %v, want idempotent", err)
	}
}

// A second press while the popup is open replaces its item in place and refuses the old item id.
func TestSecondPressReplacesItem(t *testing.T) {
	r := newRig(t)
	key, first := r.press(t, "first thought")
	mark := r.app.Events.Mark()
	key2, second := r.press(t, ghLink)
	if key2 != key || len(r.app.WindowMgr.OpenedPopups) != 1 || second.ID == first.ID || second.Kind != bridge.CaptureKindChangeRequest {
		t.Fatalf("second press: key %s (was %s), popups %v, item %+v", key2, key, r.app.WindowMgr.OpenedPopups, second)
	}
	ev := r.app.Events.Since(mark, bridge.ChannelCaptureItem)
	if len(ev) != 1 || ev[0].Window != key {
		t.Fatalf("item events = %+v", ev)
	}
	r.app.ContractEvent(t, scenario, ev[0], flowharness.Mask("id"))
	if !slices.Contains(r.app.WindowMgr.Focused, key) {
		t.Errorf("popup not focused: %v", r.app.WindowMgr.Focused)
	}
	if _, err := r.app.W.Capture.Confirm(ctx, bridge.CaptureConfirmArgs{WindowKey: key, ItemID: first.ID, Title: "x"}); err == nil {
		t.Fatal("Confirm with the replaced item id succeeded")
	}
	// Once the popup is gone a press opens a fresh one.
	if err := r.app.W.Capture.Dismiss(ctx, bridge.CaptureWindowArgs{WindowKey: key}); err != nil {
		t.Fatal(err)
	}
	key3, _ := r.fire(t)
	if key3 == key || len(r.app.WindowMgr.OpenedPopups) != 2 {
		t.Fatalf("press after dismiss reused %s; popups %v", key3, r.app.WindowMgr.OpenedPopups)
	}
}

// Text over 64 KiB is cut on a rune boundary and flagged.
func TestLongTextTruncated(t *testing.T) {
	r := newRig(t)
	_, it := r.press(t, strings.Repeat("é", 40_000))
	if !it.Truncated || len(it.Text) > 64<<10 || len(it.Text) < 64<<10-3 || len(it.Title) > 500 {
		t.Fatalf("truncated item: %d text bytes, %d title bytes, truncated %v", len(it.Text), len(it.Title), it.Truncated)
	}
	if strings.ToValidUTF8(it.Text, "") != it.Text || strings.ToValidUTF8(it.Title, "") != it.Title {
		t.Fatal("cut inside a rune")
	}
}

// Settings change the registered chord live; a bad or reserved chord is refused; every state change
// is pushed.
func TestSettingsReRegister(t *testing.T) {
	r := newRig(t)
	sc := r.app.Shortcuts
	mark := r.app.Events.Mark()

	if err := r.setCapture(model.CapturePatch{Hotkey: str("CmdOrCtrl+Alt+K")}); err != nil {
		t.Fatal(err)
	}
	if got := sc.Registered(); !slices.Equal(got, []string{"CmdOrCtrl+Alt+K"}) {
		t.Fatalf("registered %v, want only the new chord", got)
	}
	if st := r.status(t); st.State != shell.HotkeyActive || st.Accelerator != "CmdOrCtrl+Alt+K" {
		t.Fatalf("Status = %+v", st)
	}
	r.app.Clipboard.SetText("hello")
	sc.Fire(t, "CmdOrCtrl+Alt+K")
	if len(r.app.WindowMgr.OpenedPopups) != 1 {
		t.Fatal("the new chord opened no popup")
	}

	for _, bad := range []string{"", "Shift+K", "CmdOrCtrl+Shift+N", "CmdOrCtrl+Hyper+K", "K", "cmdorctrl+k"} {
		if err := r.setCapture(model.CapturePatch{Hotkey: str(bad)}); err == nil {
			t.Errorf("hotkey %q accepted", bad)
		}
	}
	if got := sc.Registered(); !slices.Equal(got, []string{"CmdOrCtrl+Alt+K"}) {
		t.Fatalf("a refused write changed the registration: %v", got)
	}

	// The OS refuses a chord: failed with its reason, and the working chord stays registered.
	sc.FailNext(errors.New("failed to register global shortcut: held by another app"))
	if err := r.setCapture(model.CapturePatch{Hotkey: str("CmdOrCtrl+Alt+J")}); err != nil {
		t.Fatal(err)
	}
	failed := r.status(t)
	if failed.State != shell.HotkeyFailed || !strings.Contains(failed.Reason, "held by another app") || !slices.Equal(sc.Registered(), []string{"CmdOrCtrl+Alt+K"}) {
		t.Fatalf("Status = %+v, registered %v", failed, sc.Registered())
	}
	r.app.Contract(t, scenario, "CaptureService.Status#failed", failed)

	// Disabled: unregistered and off. Enabled again: registered.
	if err := r.setCapture(model.CapturePatch{Enabled: yes(false), Hotkey: str("CmdOrCtrl+Alt+K")}); err != nil {
		t.Fatal(err)
	}
	off := r.status(t)
	if off.State != shell.HotkeyOff || len(sc.Registered()) != 0 {
		t.Fatalf("Status = %+v, registered %v", off, sc.Registered())
	}
	r.app.Contract(t, scenario, "CaptureService.Status#off", off)
	if err := r.setCapture(model.CapturePatch{Enabled: yes(true)}); err != nil {
		t.Fatal(err)
	}
	if st := r.status(t); st.State != shell.HotkeyActive {
		t.Fatalf("Status = %+v", st)
	}

	// A registration failure reported later (a deferred or portal bind) flips active to failed.
	bridge.CaptureReportShortcutError(r.app.W.Capture, errors.New("failed to register global shortcut \"x\": taken"))
	if st := r.status(t); st.State != shell.HotkeyFailed || !strings.Contains(st.Reason, "taken") {
		t.Fatalf("Status after report = %+v", st)
	}

	changes := r.app.Events.Since(mark, bridge.ChannelCaptureStatus)
	if len(changes) < 5 {
		t.Fatalf("status events = %d, want one per change", len(changes))
	}
}

// On Wayland the portal decides the trigger, so a registered chord reads as requested.
func TestWaylandRequested(t *testing.T) {
	r := newRig(t, flowharness.WithWayland())
	st := r.status(t)
	if st.State != shell.HotkeyRequested || st.Accelerator != model.DefaultCaptureHotkey {
		t.Fatalf("Status = %+v", st)
	}
	r.app.Contract(t, scenario, "CaptureService.Status#requested", st)
}

// A stored setting survives a relaunch: the stored chord is the one registered.
func TestRelaunchKeepsChord(t *testing.T) {
	r := newRig(t)
	if err := r.setCapture(model.CapturePatch{Hotkey: str("CmdOrCtrl+Alt+K")}); err != nil {
		t.Fatal(err)
	}
	r.app.Restart()
	if got := r.app.Shortcuts.Registered(); !slices.Equal(got, []string{"CmdOrCtrl+Alt+K"}) {
		t.Fatalf("registered %v after relaunch", got)
	}
}
