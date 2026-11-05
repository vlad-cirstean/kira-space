package mobileflow_test

import (
	"bytes"
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// shelfTitles lists the titles of the named tasks on one shelf, in shelf order.
func shelfTitles(t *testing.T, app *flowharness.App, shelf string, names ...string) []string {
	t.Helper()
	b, err := app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	title := map[string]adewire.Task{}
	for _, tk := range b.Tasks {
		title[tk.ID] = tk
	}
	var out []string
	for _, id := range b.Plan.Order {
		tk := title[id]
		_, dated := b.Plan.Day[id]
		onBacklog := slices.Contains(b.Plan.Backlog, id)
		if dated || tk.Kind == "review" || onBacklog != (shelf == adewire.ShelfBacklog) || !slices.Contains(names, tk.Title) {
			continue
		}
		out = append(out, tk.Title)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPhoneWrites(t *testing.T) {
	f := newFixture(t)
	app := f.app
	p, deviceID := pairedPhone(t, f)
	setPerms := func(write bool) {
		t.Helper()
		if err := app.W.Mobile.SetDevicePermissions(bridge.MobileDevicePermissionsArgs{ID: deviceID, Write: write}); err != nil {
			t.Fatal(err)
		}
	}

	setPerms(false)
	r := p.post("/api/ade/later/items", map[string]string{"title": "refused"})
	if r.Status != http.StatusForbidden || r.code(t) != "E_WRITE_OFF" {
		t.Fatalf("write without permission = %d %s", r.Status, r.Body)
	}
	if got := shelfTitles(t, app, adewire.ShelfLater, "refused"); len(got) != 0 {
		t.Fatalf("a refused write changed Later: %v", got)
	}

	setPerms(true)
	if r := p.send(http.MethodPost, "/api/ade/later/items", map[string]string{"title": "x"}, ""); r.Status != http.StatusBadRequest || r.code(t) != "E_IDEMPOTENCY_KEY" {
		t.Fatalf("write without an Idempotency-Key = %d %s", r.Status, r.Body)
	}
	key := ids.NewID() // v7; p.post below sends v4 keys, which still run once
	first := p.send(http.MethodPost, "/api/ade/later/items", map[string]string{"title": "first"}, key)
	if first.Status != http.StatusOK {
		t.Fatalf("later add = %d %s", first.Status, first.Body)
	}
	app.Contract(t, "mobile-board", "args:POST /api/ade/later/items", map[string]string{"title": "first"})
	app.Contract(t, "mobile-board", "http:POST /api/ade/later/items", first.raw(t), flowharness.Mask("createdAt"))
	replay := p.send(http.MethodPost, "/api/ade/later/items", map[string]string{"title": "first"}, key)
	if replay.Status != http.StatusOK || replay.Header.Get("Idempotent-Replay") != "true" || !bytes.Equal(replay.Body, first.Body) {
		t.Fatalf("replay = %d %q (replay header %q), want the first response again", replay.Status, replay.Body, replay.Header.Get("Idempotent-Replay"))
	}
	names := []string{"first", "second"}
	if got := shelfTitles(t, app, adewire.ShelfLater, names...); !equal(got, []string{"first"}) {
		t.Fatalf("Later after a replayed add = %v, want one task", got)
	}
	var second adewire.Task
	r = p.post("/api/ade/later/items", map[string]string{"title": "second"})
	r.json(t, &second)
	if got := shelfTitles(t, app, adewire.ShelfLater, names...); !equal(got, []string{"second", "first"}) {
		t.Fatalf("Later = %v, want the newest first", got)
	}
	move := map[string]string{"taskId": second.ID, "shelf": adewire.ShelfBacklog, "beforeId": ""}
	if r := p.post("/api/ade/shelf/move", move); r.Status != http.StatusOK {
		t.Fatalf("shelf move = %d %s", r.Status, r.Body)
	}
	app.Contract(t, "mobile-board", "args:POST /api/ade/shelf/move", move)
	if got := shelfTitles(t, app, adewire.ShelfLater, names...); !equal(got, []string{"first"}) {
		t.Fatalf("Later after the move = %v, want first", got)
	}
	if got := shelfTitles(t, app, adewire.ShelfBacklog, names...); !equal(got, []string{"second"}) {
		t.Fatalf("Backlog after the move = %v, want second", got)
	}

	// The task sits at its manual step; prev and next walk it through the workflow.
	if got := f.taskNow(t).EntryStepID; got != "one" {
		t.Fatalf("task entry step = %q, want one", got)
	}
	if r := p.post("/api/ade/tasks/step", map[string]string{"taskId": f.task.ID, "stepId": "one"}); r.Status != http.StatusBadRequest {
		t.Fatalf("step move without fromStepId = %d %s, want 400", r.Status, r.Body)
	}
	if r := p.post("/api/ade/tasks/step", map[string]string{"taskId": f.task.ID, "stepId": "one", "fromStepId": "one"}); r.Status == http.StatusOK {
		t.Fatalf("step move from a stale step succeeded: %s", r.Body)
	}
	for _, move := range []struct{ from, to string }{{"review", "one"}, {"one", "review"}} {
		r := p.post("/api/ade/tasks/step", map[string]string{"taskId": f.task.ID, "stepId": move.to, "fromStepId": move.from})
		if r.Status != http.StatusOK {
			t.Fatalf("step %s to %s = %d %s", move.from, move.to, r.Status, r.Body)
		}
		if got := f.taskNow(t).EntryStepID; got != move.to {
			t.Fatalf("task entry step = %q, want %q", got, move.to)
		}
	}

	// A launch asks the desktop window to open the terminal and waits for its answer.
	mark := app.Events.Mark()
	launched := make(chan bridge.MobileOpenLaunchEvent, 1)
	go func() {
		ev := app.Events.WaitAfter(t, mark, bridge.ChannelMobileOpenLaunch, nil, waitFor)
		var open bridge.MobileOpenLaunchEvent
		ev.Decode(t, &open)
		if ev.Window != desktopWindow {
			t.Errorf("open-launch sent to %q, want %s", ev.Window, desktopWindow)
		}
		f.openLaunch(t, open.Launch)
		if err := app.W.Mobile.LaunchOpened(bridge.MobileLaunchOpenedArgs{TerminalID: open.Launch.TerminalID}); err != nil {
			t.Errorf("LaunchOpened: %v", err)
		}
		launched <- open
	}()
	r = p.post("/api/ade/tasks/launch", map[string]string{"taskId": f.task.ID, "fromStepId": "review"})
	if r.Status != http.StatusOK {
		t.Fatalf("launch = %d %s", r.Status, r.Body)
	}
	open := <-launched
	var res struct {
		SessionID string `json:"sessionId"`
	}
	r.json(t, &res)
	if res.SessionID == "" || res.SessionID != open.Launch.SessionID || open.TaskID != f.task.ID {
		t.Fatalf("launch result %+v for event %+v", res, open)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		sessions, _ := app.W.AdeTask.Sessions(ctx)
		for _, s := range sessions.Sessions {
			if s.ID == res.SessionID {
				return s.State == "running" && s.TerminalID == open.Launch.TerminalID
			}
		}
		return false
	})
	f.release(t)
}

// v7 and v4 keys each run once and replay; a short non-UUID key is refused.
func TestPhoneIdempotencyKeys(t *testing.T) {
	f := newFixture(t)
	p, deviceID := pairedPhone(t, f)
	if err := f.app.W.Mobile.SetDevicePermissions(bridge.MobileDevicePermissionsArgs{ID: deviceID, Write: true}); err != nil {
		t.Fatal(err)
	}
	const path = "/api/ade/later/items"
	if r := p.send(http.MethodPost, path, map[string]string{"title": "bad"}, "abc"); r.Status != http.StatusBadRequest || r.code(t) != "E_IDEMPOTENCY_KEY" {
		t.Fatalf("key abc = %d %s", r.Status, r.Body)
	}
	for name, key := range map[string]string{"v7": ids.NewID(), "v4": uuid.NewString()} {
		body := map[string]string{"title": name + " key"}
		first := p.send(http.MethodPost, path, body, key)
		again := p.send(http.MethodPost, path, body, key)
		if first.Status != http.StatusOK || again.Header.Get("Idempotent-Replay") != "true" || !bytes.Equal(first.Body, again.Body) {
			t.Fatalf("%s key: first %d, replay %d (replay header %q)", name, first.Status, again.Status, again.Header.Get("Idempotent-Replay"))
		}
		if got := shelfTitles(t, f.app, adewire.ShelfLater, body["title"]); len(got) != 1 {
			t.Fatalf("%s key ran %d times, want once", name, len(got))
		}
	}
}
