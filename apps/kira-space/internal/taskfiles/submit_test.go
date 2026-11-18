package taskfiles

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func TestSubmitDraftAccepted(t *testing.T) {
	e := newEnv(t)
	e.disk("handovers/draft-implement.md", "# plan\nbody\n")
	r := e.submit(RoleRun, "s1", "")
	if f := r.find("handovers/draft-implement.md"); f.Status != StatusAccepted {
		t.Fatalf("%+v", f)
	}
	if got, ok := e.h.get("handovers/draft-implement.md"); !ok || got != "# plan\nbody\n" {
		t.Fatalf("cache = %q %v", got, ok)
	}
	// Unchanged afterwards, and a second submit reads nothing.
	e.st.reads = 0
	if f := e.submit(RoleRun, "s1", "").find("handovers/draft-implement.md"); f.Status != StatusUnchanged {
		t.Fatalf("%+v", f)
	}
	if e.st.reads != 0 {
		t.Fatalf("unchanged file was read %d times", e.st.reads)
	}
	if c := e.submit(RoleRun, "s1", "").Counts(); c != (Counts{}) {
		t.Fatalf("counts %+v", c)
	}
}

func TestSubmitWrongStageDraftRejectedAndRemoved(t *testing.T) {
	e := newEnv(t)
	e.disk("handovers/draft-review.md", "x\n")
	f := e.submit(RoleRun, "s1", "").find("handovers/draft-review.md")
	if f.Status != StatusRejected {
		t.Fatalf("%+v", f)
	}
	if _, err := os.Stat(filepath.Join(e.local.Root(), "handovers/draft-review.md")); !os.IsNotExist(err) {
		t.Fatal("rejected draft still on disk")
	}
}

func TestSubmitNoteNeedsReason(t *testing.T) {
	e := newEnv(t)
	const rel = "notes/20261101T090000-a1.md"
	e.seed(rel, "user note\n")
	e.disk(rel, "agent edit\n")
	f := e.submit(RoleRun, "s1", "").find(rel)
	want := ReasonError(rel, CauseNote)
	if f.Status != StatusInvalid || f.Message != want {
		t.Fatalf("got %q %q\nwant %q", f.Status, f.Message, want)
	}
	if got, _ := e.h.get(rel); got != "user note\n" {
		t.Fatalf("cache moved to %q", got)
	}
	f = e.submit(RoleRun, "s1", "typo fix", rel).find(rel)
	if f.Status != StatusPending {
		t.Fatalf("%+v", f)
	}
	if len(e.ev.pending) != 1 || e.ev.pending[0].Items[0].Path != rel || e.ev.pending[0].Reason != "typo fix" {
		t.Fatalf("pending events %+v", e.ev.pending)
	}
	if got, _ := e.h.get(rel); got != "user note\n" {
		t.Fatal("pending change applied before approval")
	}
}

func TestSubmitRestoresKiraFiles(t *testing.T) {
	e := newEnv(t)
	e.seed("README.md", "# readme\n")
	e.seed("state.yaml", "step: a\n")
	e.disk("README.md", "hacked\n")
	if err := os.Remove(filepath.Join(e.local.Root(), "state.yaml")); err != nil {
		t.Fatal(err)
	}
	r := e.submit(RoleRun, "s1", "")
	for _, rel := range []string{"README.md", "state.yaml"} {
		if f := r.find(rel); f.Status != StatusRestored {
			t.Fatalf("%s: %+v", rel, f)
		}
	}
	if e.read("README.md") != "# readme\n" || e.read("state.yaml") != "step: a\n" {
		t.Fatalf("not restored: %q %q", e.read("README.md"), e.read("state.yaml"))
	}
}

func TestSubmitParseErrorHasLine(t *testing.T) {
	e := newEnv(t)
	e.disk("links.yaml", "ok\nBAD here\n")
	f := e.submit(RoleRun, "s1", "").find("links.yaml")
	if f.Status != StatusInvalid || f.Line != 2 || !strings.Contains(f.Message, "bad token") {
		t.Fatalf("%+v", f)
	}
	rows, _ := e.repo.Files(e.taskID)
	if len(rows) != 1 || rows[0].Review != model.TaskFileInvalid || rows[0].AcceptedSHA256 != "" {
		t.Fatalf("rows %+v", rows)
	}
	e.disk("links.yaml", "fixed\n")
	if f := e.submit(RoleRun, "s1", "").find("links.yaml"); f.Status != StatusAccepted {
		t.Fatalf("%+v", f)
	}
}

func TestSubmitCollision(t *testing.T) {
	e := newEnv(t)
	const rel = "handovers/draft-implement.md"
	e.seed(rel, "user text\n") // accepted by "user" at e.now
	e.disk(rel, "agent text\n")
	req := func(reason string) FileResult {
		res, err := e.svc.Submit(context.Background(), Request{TaskID: e.taskID, Paths: []string{rel}, Reason: reason,
			Session: Session{Key: "s1", Role: RoleRun, Step: "implement", LastSync: e.now.Add(-time.Minute)}})
		if err != nil {
			t.Fatal(err)
		}
		return res.find(rel)
	}
	if f := req(""); f.Status != StatusInvalid || !strings.Contains(f.Message, "needs reason") {
		t.Fatalf("%+v", f)
	}
	f := req("keep mine")
	if f.Status != StatusPending || !strings.Contains(f.Message, "user changed it") {
		t.Fatalf("%+v", f)
	}
	if e.ev.pending[0].Items[0].Collision == nil {
		t.Fatal("pending item lacks the collision")
	}
}

func TestSubmitFolderLimits(t *testing.T) {
	e := newEnv(t)
	if err := os.MkdirAll(filepath.Join(e.local.Root(), "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(e.local.Root(), "notes/20261101T090000-ln.md")); err != nil {
		t.Fatal(err)
	}
	e.disk(".hidden/x.md", "x")
	e.disk("random.txt", "x")
	r := e.submit(RoleRun, "s1", "")
	if f := r.find("notes/20261101T090000-ln.md"); f.Status != StatusInvalid || !strings.Contains(f.Message, "symlink") {
		t.Fatalf("symlink: %+v", f)
	}
	if f := r.find(".hidden/x.md"); f.Status != "absent" {
		t.Fatalf("hidden reported: %+v", f)
	}
	if f := r.find("random.txt"); f.Status != StatusIgnored {
		t.Fatalf("stray: %+v", f)
	}
	// Sorts after the cap: listed nowhere, still on disk, so a truncated listing must not delete it.
	const past = "notes/20261101T095900-zzz.md"
	e.seed(past, "kept\n")
	for i := 0; i < MaxFilesPerTask+1; i++ {
		e.disk(fmt.Sprintf("notes/20261101T0900%02d-%03d.md", i%60, i), "n")
	}
	r = e.submit(RoleRun, "s1", "")
	if f := r.find("."); f.Status != StatusInvalid || !strings.Contains(f.Message, "more than 500 files") {
		t.Fatalf("501 files: %+v", f)
	}
	if f := r.find(past); f.Status != "absent" {
		t.Fatalf("unlisted indexed file handled: %+v", f)
	}
	if _, ok := e.h.get(past); !ok {
		t.Fatal("unlisted indexed file dropped from the cache")
	}
	if f := e.submit(RoleRun, "s1", "", past).find(past); f.Status != StatusInvalid || !strings.Contains(f.Message, "not read") {
		t.Fatalf("named unlisted file: %+v", f)
	}
}

func TestSubmitDeletedHandedHandoverPending(t *testing.T) {
	e := newEnv(t)
	const rel = "handovers/0001-plan-to-implement.md"
	e.seed(rel, "handed\n")
	if err := os.Remove(filepath.Join(e.local.Root(), rel)); err != nil {
		t.Fatal(err)
	}
	if f := e.submit(RoleRun, "s1", "").find(rel); f.Status != StatusInvalid {
		t.Fatalf("no reason: %+v", f)
	}
	if f := e.submit(RoleRun, "s1", "obsolete", rel).find(rel); f.Status != StatusPending {
		t.Fatalf("with reason: %+v", f)
	}
	if _, ok := e.h.get(rel); !ok {
		t.Fatal("cache dropped before approval")
	}
}

func TestWriteRecover(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	// Crash after the file write, before the database side: intent and file match.
	if err := e.svc.putIntent(e.taskID, "notes/20261101T090000-a1.md", sum([]byte("n\n")), "user"); err != nil {
		t.Fatal(err)
	}
	e.disk("notes/20261101T090000-a1.md", "n\n")
	// Crash before the file write: intent without a file.
	if err := e.svc.putIntent(e.taskID, "notes/20261101T090001-b2.md", sum([]byte("m\n")), "user"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if in, _ := e.repo.Intents(); len(in) != 0 {
		t.Fatalf("intents left: %+v", in)
	}
	rows, _ := e.repo.Files(e.taskID)
	if len(rows) != 1 || rows[0].RelPath != "notes/20261101T090000-a1.md" || rows[0].Review != model.TaskFileAccepted {
		t.Fatalf("rows %+v", rows)
	}
}

func TestChangedBudget(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < MaxFilesPerTask; i++ {
		e.seed(fmt.Sprintf("notes/20261101T090000-%03d.md", i), "n")
	}
	best := time.Hour
	for run := 0; run < 7; run++ {
		start := time.Now()
		got, err := e.svc.Changed(context.Background(), e.taskID)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("changed = %v", got[:min(3, len(got))])
		}
		best = min(best, time.Since(start))
	}
	// Generous for a loaded CI runner; still far below a pass that reads or hashes file contents.
	if best > 25*time.Millisecond {
		t.Fatalf("stat pass over %d files took %s, budget 25ms", MaxFilesPerTask, best)
	}
	e.disk("notes/20261101T090000-007.md", "changed")
	if got, _ := e.svc.Changed(context.Background(), e.taskID); len(got) != 1 {
		t.Fatalf("changed = %v", got)
	}
}
