package mobileflow_test

import (
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// The phone and the desktop reorder the same Backlog shelf at once: the result is one of the two
// serial orders, never a lost or doubled task.
func TestPhoneAndDesktopWriteRace(t *testing.T) {
	f := newFixture(t)
	app := f.app
	p, deviceID := pairedPhone(t, f)
	if err := app.W.Mobile.SetDevicePermissions(bridge.MobileDevicePermissionsArgs{ID: deviceID, Write: true}); err != nil {
		t.Fatal(err)
	}
	names := []string{"a", "b", "c", "d"}
	ids := map[string]string{}
	for _, title := range names {
		tk, err := app.W.AdeTask.CaptureTask(ctx, adewire.CaptureTaskArgs{Title: title})
		if err != nil {
			t.Fatal(err)
		}
		if err := app.W.AdeTask.ShelveTask(ctx, adewire.ShelveTaskArgs{TaskID: tk.ID, Shelf: adewire.ShelfBacklog}); err != nil {
			t.Fatal(err)
		}
		ids[title] = tk.ID
	}

	for round := range 10 {
		before := shelfTitles(t, app, adewire.ShelfBacklog, names...)
		var wg sync.WaitGroup
		var phoneStatus int
		var desktopErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			phoneStatus = p.post("/api/ade/shelf/move", map[string]string{"taskId": ids["a"], "shelf": adewire.ShelfBacklog, "beforeId": ""}).Status
		}()
		go func() {
			defer wg.Done()
			desktopErr = app.W.AdeTask.ShelveTask(ctx, adewire.ShelveTaskArgs{TaskID: ids["d"], Shelf: adewire.ShelfBacklog, BeforeID: ids["b"]})
		}()
		wg.Wait()
		if phoneStatus != http.StatusOK || desktopErr != nil {
			t.Fatalf("round %d: phone %d, desktop %v", round, phoneStatus, desktopErr)
		}
		got := shelfTitles(t, app, adewire.ShelfBacklog, names...)
		sorted := slices.Sorted(slices.Values(got))
		if !slices.Equal(sorted, names) {
			t.Fatalf("round %d: Backlog %v lost or doubled a task", round, got)
		}
		phoneFirst := serial(before, "a", "", "d", "b")
		desktopFirst := serial(before, "d", "b", "a", "")
		if !slices.Equal(got, phoneFirst) && !slices.Equal(got, desktopFirst) {
			t.Fatalf("round %d: Backlog %v after %v, want %v or %v", round, got, before, phoneFirst, desktopFirst)
		}
	}
}

// serial applies two shelf moves in order: each puts a title right before another, or last.
func serial(start []string, firstTitle, firstBefore, secondTitle, secondBefore string) []string {
	out := slices.Clone(start)
	move := func(title, before string) {
		out = slices.Delete(out, slices.Index(out, title), slices.Index(out, title)+1)
		if before == "" {
			out = append(out, title)
			return
		}
		out = slices.Insert(out, slices.Index(out, before), title)
	}
	move(firstTitle, firstBefore)
	move(secondTitle, secondBefore)
	return out
}

// A paired phone stays paired across a relaunch.
func TestPairedDeviceSurvivesRestart(t *testing.T) {
	f := newFixture(t)
	app := f.app
	_, deviceID := pairedPhone(t, f)
	before, err := app.W.Mobile.Devices()
	if err != nil || len(before) != 1 || before[0].ID != deviceID {
		t.Fatalf("Devices = %+v, %v", before, err)
	}
	app.Restart()
	after, err := app.W.Mobile.Devices()
	if err != nil || len(after) != 1 || after[0].ID != deviceID || after[0].Label != before[0].Label {
		t.Fatalf("Devices after restart = %+v, %v, want %+v", after, err, before)
	}
}

// J-MOB-012 and J-X-011: the phone and the desktop move the same task to the same stage at once.
// The task ends on one stage, the database agrees, and both clients are told.
func TestPhoneAndDesktopMoveSameStage(t *testing.T) {
	f := newFixture(t)
	app := f.app
	p, deviceID := pairedPhone(t, f)
	if err := app.W.Mobile.SetDevicePermissions(bridge.MobileDevicePermissionsArgs{ID: deviceID, Write: true}); err != nil {
		t.Fatal(err)
	}
	stream := p.events()
	// The finished run leaves the task at its manual stage.
	from, to := "review", "one"
	for round := range 6 {
		mark := app.Events.Mark()
		var wg sync.WaitGroup
		var phone reply
		var desktopErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			phone = p.post("/api/ade/tasks/step", map[string]string{"taskId": f.task.ID, "stepId": to, "fromStepId": from})
		}()
		go func() {
			defer wg.Done()
			_, desktopErr = app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: f.task.ID, StepID: to})
		}()
		wg.Wait()
		if phone.Status != http.StatusOK && phone.Status != http.StatusConflict {
			t.Fatalf("round %d: phone %d %s", round, phone.Status, phone.Body)
		}
		if desktopErr != nil {
			t.Fatalf("round %d: desktop %v", round, desktopErr)
		}
		if got := f.taskNow(t).EntryStepID; got != to {
			t.Fatalf("round %d: task at %q, want %q", round, got, to)
		}
		var board adewire.Board
		p.get("/api/ade/board").json(t, &board)
		for _, tk := range board.Tasks {
			if tk.ID == f.task.ID && tk.EntryStepID != to {
				t.Fatalf("round %d: phone reads stage %q, desktop %q", round, tk.EntryStepID, to)
			}
		}
		app.Events.WaitAfter(t, mark, adewire.ChannelBoard, nil, waitFor)
		stream.wait(t, adewire.ChannelBoard, waitFor)
		from, to = to, from
	}
}
