package proofflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
)

// An open proof window learns that its proof changed: every content write raises kira:proof:changed and
// Detail.Version tells the window whether its copy is stale. Contract proof-stale;
// tests/ui/studio/proof-stale.spec.ts is the UI half.

const (
	staleScenario = "proof-stale"
	staleWait     = 20 * time.Second
	staleQuiet    = 900 * time.Millisecond
)

var staleMask = flowharness.Mask("id", "closedAt", "createdAt", "linkedAt")

// named reports an event that names the session.
func named(id string) func(flowharness.Event) bool {
	return func(ev flowharness.Event) bool {
		c, ok := ev.Data.(proof.Changed)
		return ok && c.SessionID == id
	}
}

// namedSince counts the events naming the session since mark.
func namedSince(f *fixture, mark int, id string) int {
	n := 0
	for _, ev := range f.app.Events.Since(mark, proof.ChannelChanged) {
		if named(id)(ev) {
			n++
		}
	}
	return n
}

func TestProofStaleSignal(t *testing.T) {
	f := newFixture(t)
	s := f.save(t, "Login", tableStep("rows"))
	f.app.Events.Wait(t, proof.ChannelChanged, named(s.ID), staleWait)
	loaded := session(t, f.app, s.ID)

	mark := f.app.Events.Mark()
	if got := f.save(t, "Login", tableStep("rows changed")); got.Pending {
		t.Fatalf("edit = %+v, want a silent accept of the newest proof", got)
	}
	ev := f.app.Events.WaitAfter(t, mark, proof.ChannelChanged, named(s.ID), staleWait)
	time.Sleep(staleQuiet)
	if n := namedSince(f, mark, s.ID); n != 1 {
		t.Fatalf("proof events = %d, want exactly one for an accepted edit", n)
	}

	changed := session(t, f.app, s.ID)
	if changed.Version == loaded.Version {
		t.Fatalf("version = %q before and after the edit", loaded.Version)
	}
	if changed.Current.Rev != loaded.Current.Rev+1 {
		t.Fatalf("rev = %d, want %d: an accepted edit bumps it", changed.Current.Rev, loaded.Current.Rev+1)
	}

	opts := []flowharness.ContractOption{staleMask, flowharness.Replace(s.ID, "session-1"),
		flowharness.Replace(loaded.Version, "<version:loaded>"), flowharness.Replace(changed.Version, "<version:changed>")}
	f.app.Contract(t, staleScenario, "ProofService.Session#loaded", loaded, opts...)
	f.app.Contract(t, staleScenario, "ProofService.Session#changed", changed, opts...)
	f.app.Contract(t, staleScenario, "event:"+ev.Channel+"#acceptedEdit", ev.Data, opts...)
}

func TestProofStaleUserPaths(t *testing.T) {
	f := newFixture(t)
	s := f.save(t, "Login", tableStep("rows"))
	f.app.Events.Wait(t, proof.ChannelChanged, named(s.ID), staleWait)
	f.newest(t) // a later proof: Login is older, so another agent's edit of it waits for review
	md := "proofs/" + s.Slug + "/proof.md"
	accept := func(rel string) {
		t.Helper()
		if err := f.app.W.AdeTask.DetailsAcceptDisk(ctx, adewire.DiskResolveArgs{TaskID: f.taskID, Paths: []string{rel}}); err != nil {
			t.Fatal(err)
		}
	}

	// A save on disk changes no cache row: no event until the user accepts.
	before := session(t, f.app, s.ID)
	mark := f.app.Events.Mark()
	f.write(t, "Login", []map[string]any{tableStep("rows by hand")})
	time.Sleep(staleQuiet)
	if n := namedSince(f, mark, s.ID); n != 0 {
		t.Fatalf("%d proof events for an unaccepted save, want none", n)
	}
	accept(md)
	f.app.Events.WaitAfter(t, mark, proof.ChannelChanged, named(s.ID), staleWait)
	time.Sleep(staleQuiet)
	if n := namedSince(f, mark, s.ID); n != 1 {
		t.Fatalf("proof events = %d after the accept, want one", n)
	}
	if after := session(t, f.app, s.ID); after.Version == before.Version {
		t.Fatalf("version = %q after the accept, want a new one", after.Version)
	}

	// Another agent's edit of an older proof waits for review; a second edit replaces it under the same rev.
	mark = f.app.Events.Mark()
	f.edit(t, "Login", tableStep("first proposal"))
	f.app.Events.WaitAfter(t, mark, proof.ChannelChanged, named(s.ID), staleWait)
	first := session(t, f.app, s.ID)
	if first.Pending == nil {
		t.Fatalf("detail = %+v, want a pending edit", first)
	}
	mark = f.app.Events.Mark()
	f.edit(t, "Login", tableStep("second proposal"))
	f.app.Events.WaitAfter(t, mark, proof.ChannelChanged, named(s.ID), staleWait)
	second := session(t, f.app, s.ID)
	if second.Pending == nil || second.Pending.Rev != first.Pending.Rev {
		t.Fatalf("pending = %+v, want the same rev as %+v", second.Pending, first.Pending)
	}
	if second.Version == first.Version {
		t.Fatalf("version = %q for a replaced pending edit, want a new one", second.Version)
	}

	// Discarding the edit on the banner drops it.
	mark = f.app.Events.Mark()
	if err := f.app.W.AdeTask.DetailsDiscardDisk(ctx, adewire.DiskResolveArgs{TaskID: f.taskID, Paths: []string{md}}); err != nil {
		t.Fatal(err)
	}
	f.app.Events.WaitAfter(t, mark, proof.ChannelChanged, named(s.ID), staleWait)
	if dropped := session(t, f.app, s.ID); dropped.Pending != nil || dropped.Version == second.Version {
		t.Fatalf("detail = %+v after the discard, want no pending edit and a new version", dropped)
	}

	// Deleting the folder, accepted by the user, removes the proof.
	del := f.idOf(t, "Newest")
	mark = f.app.Events.Mark()
	if err := os.RemoveAll(filepath.Join(f.folder, "proofs", "newest")); err != nil {
		t.Fatal(err)
	}
	accept("proofs/newest/proof.md")
	ev := f.app.Events.WaitAfter(t, mark, proof.ChannelChanged, named(del), staleWait)
	time.Sleep(staleQuiet)
	if n := namedSince(f, mark, del); n != 1 {
		t.Fatalf("proof events = %d for the delete, want one", n)
	}
	if _, err := f.app.W.Proof.Session(bridge.ProofSessionArgs{SessionID: del}); err == nil {
		t.Fatal("Session of a deleted proof answered, want not found")
	}
	f.app.Contract(t, staleScenario, "event:"+ev.Channel+"#deleted", ev.Data, staleMask, flowharness.Replace(del, "session-1"))
}
