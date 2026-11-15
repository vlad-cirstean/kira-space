package ade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// stateBoard is a board harness with the task file layer on and one task without branches.
func stateBoard(t *testing.T) (*boardHarness, string) {
	t.Helper()
	h := newBoardHarness(t)
	h.board.deps.FilesDir = filepath.Join(t.TempDir(), "tasks")
	h.board.deps.TaskFiles = h.repos.TaskFiles
	h.board.deps.Sessions = h.repos.AdeSessions
	h.board.initDetails()
	h.addTask("t1", model.AdeTaskKindTask)
	return h, filepath.Join(h.board.deps.FilesDir, "t1", "state.yaml")
}

func hasWriter(b *TaskBoard, id string) bool { return b.stateWriterOf(id) != nil }

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// An idle writer stays while a session runs (it reads state.yaml with no call that touches it) and
// leaves once the agent stops.
func TestStateWriterIdleKeepsLiveAgent(t *testing.T) {
	old := stateIdle
	stateIdle = 60 * time.Millisecond
	t.Cleanup(func() { stateIdle = old })
	h, _ := stateBoard(t)
	b := h.board
	if err := h.repos.AdeSessions.InsertTUI(model.AdeSession{ID: "s1", Mode: model.AdeSessionModeTUI, TaskID: "t1", Cwd: "/w", State: model.AdeSessionStateRunning, StartedAt: 1, LastActiveAt: 1}); err != nil {
		t.Fatal(err)
	}
	b.markStateDirty("t1", true)
	if !hasWriter(b, "t1") {
		t.Fatal("no writer after a touch")
	}
	time.Sleep(10 * stateIdle)
	if !hasWriter(b, "t1") {
		t.Fatal("writer left while a session runs")
	}
	if err := h.repos.AdeSessions.MarkStopped("s1", 2); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the writer to leave after the session stopped", func() bool { return !hasWriter(b, "t1") })
}

// An archive waits for a render in flight, so the final file is the archived one and no later render
// of the dropped writer overwrites it.
func TestArchiveWaitsForInFlightRender(t *testing.T) {
	h, statePath := stateBoard(t)
	b := h.board
	// Statuses read from a one-connection database the test holds: the render blocks inside Statuses.List.
	db2, err := storage.OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db2.Close() })
	db2.DB.SetMaxOpenConns(1)
	blocked := *h.repos.AdeStatuses
	blocked.DB = db2.DB
	b.deps.Statuses = &blocked
	held, err := db2.DB.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	b.markStateDirty("t1", true)
	w := b.stateWriterOf("t1")
	eventually(t, "the render to hold the writer lock", func() bool {
		if w.mu.TryLock() {
			w.mu.Unlock()
			return false
		}
		return true
	})
	if err := h.repos.AdeTasks.ArchiveTask("t1", 5); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		b.writeArchivedState("t1")
	}()
	select {
	case <-done:
		t.Fatal("archive finished while a render was in flight")
	case <-time.After(150 * time.Millisecond):
	}
	if err := held.Close(); err != nil { // release the render
		t.Fatal(err)
	}
	<-done
	got, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "status: archived") {
		t.Fatalf("state.yaml after archive:\n%s", got)
	}
	b.markStateDirty("t1", true)
	time.Sleep(3 * stateCoalesce)
	if again, _ := os.ReadFile(statePath); string(again) != string(got) {
		t.Fatalf("a later render rewrote the archived file:\n%s", again)
	}
}
