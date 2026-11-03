package gitrpc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// fakeRepoSettingsStore is a minimal, in-memory stand-in for storage/repos.GitRepoSettingsRepo,
// the same "closures over a plain map" shape registry_test.go's own fakes already use elsewhere in
// this chapter.
type fakeRepoSettingsStore struct {
	rows map[string]model.GitRepoSettings
}

func newFakeRepoSettingsStore() *fakeRepoSettingsStore {
	return &fakeRepoSettingsStore{rows: map[string]model.GitRepoSettings{}}
}

func (f *fakeRepoSettingsStore) get(repoID string) (model.GitRepoSettings, error) {
	if s, ok := f.rows[repoID]; ok {
		return s, nil
	}
	return model.DefaultGitRepoSettings(), nil
}

func (f *fakeRepoSettingsStore) set(repoID string, patch model.GitRepoSettingsPatch) (model.GitRepoSettings, error) {
	current, err := f.get(repoID)
	if err != nil {
		return model.GitRepoSettings{}, err
	}
	if patch.GraphPageSize != nil {
		current.GraphPageSize = *patch.GraphPageSize
	}
	if patch.GraphScope != nil {
		current.GraphScope = *patch.GraphScope
	}
	if patch.StashShowInGraph != nil {
		current.StashShowInGraph = *patch.StashShowInGraph
	}
	if patch.StashIncludeUntracked != nil {
		current.StashIncludeUntracked = *patch.StashIncludeUntracked
	}
	if patch.ReviewBaseCandidates != nil {
		current.ReviewBaseCandidates = *patch.ReviewBaseCandidates
	}
	if patch.PullStrategy != nil {
		current.PullStrategy = *patch.PullStrategy
	}
	if patch.WorktreePrepareScript != nil {
		current.WorktreePrepareScript = *patch.WorktreePrepareScript
	}
	f.rows[repoID] = current
	return current, nil
}

func newTestRouter() (*Router, *gitsession.Registry) {
	store := newFakeRepoSettingsStore()
	reg := gitsession.NewRegistry(nil)
	reg.RepoSettingsGet = store.get
	reg.RepoSettingsSet = func(repoID string, patch model.GitRepoSettingsPatch) (model.GitRepoSettings, error) {
		return store.set(repoID, patch)
	}
	return New(Deps{Registry: reg, ServerVersion: "test"}), reg
}

// TestRepoSettings_GetSetRoundTrip proves repoSettings.get/set thread a real patch through to
// storage and back, including the seven-key snapshot shape (D4).
func TestRepoSettings_GetSetRoundTrip(t *testing.T) {
	router, _ := newTestRouter()
	conn := gitsession.NewConn("conn-1", "client-1", "label", nil)
	t.Cleanup(conn.Close)

	got, err := router.handleRepoSettingsSet(context.Background(), conn, []byte(`{
		"repoId": "/repos/a",
		"patch": {"kiraSpace.pull.strategy": "rebase"}
	}`))
	if err != nil {
		t.Fatalf("repoSettings.set: %v", err)
	}
	snap, ok := got.(RepoSettingsSnapshot)
	if !ok {
		t.Fatalf("repoSettings.set result = %T, want RepoSettingsSnapshot", got)
	}
	if snap.PullStrategy != "rebase" {
		t.Fatalf("PullStrategy = %q, want %q", snap.PullStrategy, "rebase")
	}

	got, err = router.handleRepoSettingsGet(context.Background(), conn, []byte(`{"repoId": "/repos/a"}`))
	if err != nil {
		t.Fatalf("repoSettings.get: %v", err)
	}
	snap, ok = got.(RepoSettingsSnapshot)
	if !ok {
		t.Fatalf("repoSettings.get result = %T, want RepoSettingsSnapshot", got)
	}
	if snap.PullStrategy != "rebase" {
		t.Fatalf("Get after Set: PullStrategy = %q, want %q", snap.PullStrategy, "rebase")
	}
}

// TestRepoSettings_DecomposedRepoIDReadsBackTheComposedlyWrittenRow is a regression guard:
// every other repo-keyed handler normalizes repoId to NFC before use, but repoSettings.get/set
// didn't — RepoEntry.RepoSettings() reads by Summary.RepoID, which IS NFC-normalized
// internally, so a client writing under a decomposed spelling and reading back under the composed
// one (or vice versa) used to see two entirely different storage rows. The emitted
// repoSettings.changed event must also echo the normalized spelling, matching repo.changed's own
// always-NFC convention elsewhere.
func TestRepoSettings_DecomposedRepoIDReadsBackTheComposedlyWrittenRow(t *testing.T) {
	decomposedE := string([]byte{0x65, 0xcc, 0x81}) // "e" + U+0301, decomposed "é"
	composedE := string([]byte{0xc3, 0xa9})         // U+00E9, composed "é"
	decomposedRepoID := "/repos/caf" + decomposedE
	composedRepoID := "/repos/caf" + composedE

	router, _ := newTestRouter()
	conn := gitsession.NewConn("conn-1", "client-1", "label", nil)
	t.Cleanup(conn.Close)

	var mu sync.Mutex
	var changedRepoID string
	conn.SetEmit(func(method string, payload any) {
		if method == "repoSettings.changed" {
			mu.Lock()
			changedRepoID = payload.(RepoSettingsChangedPayload).RepoID
			mu.Unlock()
		}
	})
	handlers := router.ForConn(conn)

	setParams, err := json.Marshal(map[string]any{
		"repoId": decomposedRepoID,
		"patch":  map[string]any{"kiraSpace.pull.strategy": "rebase"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := handlers.Request(context.Background(), "repoSettings.set", setParams); err != nil {
		t.Fatalf("repoSettings.set (decomposed): %v", err)
	}

	// repoSettings.changed is delivered on ForConn's own forwarding goroutine, decoupled from the
	// request's own caller (settings_test.go's own TestRepoSettings_ChangedEventReachesEveryConnection
	// doc comment) — poll rather than assume it already landed.
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		got := changedRepoID
		mu.Unlock()
		if got == composedRepoID {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("repoSettings.changed RepoID = %q, want the normalized composed spelling %q", got, composedRepoID)
		}
		time.Sleep(time.Millisecond)
	}

	getParams, err := json.Marshal(map[string]string{"repoId": composedRepoID})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := router.handleRepoSettingsGet(context.Background(), conn, getParams)
	if err != nil {
		t.Fatalf("repoSettings.get (composed): %v", err)
	}
	snap, ok := got.(RepoSettingsSnapshot)
	if !ok {
		t.Fatalf("repoSettings.get result = %T, want RepoSettingsSnapshot", got)
	}
	if snap.PullStrategy != "rebase" {
		t.Fatalf("Get(composed) after Set(decomposed): PullStrategy = %q, want %q (same repo, two un-normalized spellings)", snap.PullStrategy, "rebase")
	}
}

// TestRepoSettings_GraphScopeIsScopedAcrossRepos guards, at the RPC layer, that repoSettings.set
// on repo A is not visible via repoSettings.get on repo B.
func TestRepoSettings_GraphScopeIsScopedAcrossRepos(t *testing.T) {
	router, _ := newTestRouter()
	conn := gitsession.NewConn("conn-1", "client-1", "label", nil)
	t.Cleanup(conn.Close)

	if _, err := router.handleRepoSettingsSet(context.Background(), conn, []byte(`{
		"repoId": "/repos/a",
		"patch": {"kiraSpace.graph.scope": "head"}
	}`)); err != nil {
		t.Fatalf("repoSettings.set(a): %v", err)
	}

	got, err := router.handleRepoSettingsGet(context.Background(), conn, []byte(`{"repoId": "/repos/b"}`))
	if err != nil {
		t.Fatalf("repoSettings.get(b): %v", err)
	}
	snap := got.(RepoSettingsSnapshot)
	if snap.GraphScope != "all" {
		t.Fatalf("Get(b).GraphScope = %q, want %q (the default — unscoped by a's write)", snap.GraphScope, "all")
	}
}

// changedEventCollector is a mutex-guarded stand-in for the plain slice this test used: ForConn's
// repoSettings.changed delivery now runs on each connection's own dedicated forwarding goroutine
// (handlers.go), never synchronously on the repoSettings.set caller's own goroutine — so a plain,
// unguarded slice appended to from that goroutine and read back from the test's own goroutine
// would be a genuine data race.
type changedEventCollector struct {
	mu  sync.Mutex
	got []RepoSettingsChangedPayload
}

func (c *changedEventCollector) record(payload RepoSettingsChangedPayload) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, payload)
}

func (c *changedEventCollector) snapshot() []RepoSettingsChangedPayload {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]RepoSettingsChangedPayload(nil), c.got...)
}

func (c *changedEventCollector) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = nil
}

// TestRepoSettings_ChangedEventReachesEveryConnection is the event-fan-out guard: two
// different Conns — even ones that have never opened the repo the write happened on — both
// receive repoSettings.changed, proving the live-propagation mechanism fans every change out to
// every open connection, not just the one that made the request, regardless of which key changed.
func TestRepoSettings_ChangedEventReachesEveryConnection(t *testing.T) {
	router, _ := newTestRouter()

	connA := gitsession.NewConn("conn-a", "client-a", "label-a", nil)
	connB := gitsession.NewConn("conn-b", "client-b", "label-b", nil)
	t.Cleanup(connA.Close)
	t.Cleanup(connB.Close)

	var gotA, gotB changedEventCollector
	handlersA := router.ForConn(connA)
	handlersB := router.ForConn(connB)
	connA.SetEmit(func(method string, payload any) {
		if method == "repoSettings.changed" {
			gotA.record(payload.(RepoSettingsChangedPayload))
		}
	})
	connB.SetEmit(func(method string, payload any) {
		if method == "repoSettings.changed" {
			gotB.record(payload.(RepoSettingsChangedPayload))
		}
	})

	// A sets graph.scope on repo A; both connections must be told, even though connB never opened
	// (or even heard of) repo A — the RPC layer never filters repoSettings.changed recipients by
	// repoId, for any key.
	if _, err := handlersA.Request(context.Background(), "repoSettings.set", []byte(`{
		"repoId": "/repos/a",
		"patch": {"kiraSpace.graph.scope": "head"}
	}`)); err != nil {
		t.Fatalf("repoSettings.set via connA: %v", err)
	}

	// G32 round-3 architecture/security review, finding #5: each connection's own delivery now runs
	// on a dedicated forwarding goroutine, decoupled from repoSettings.set's own caller — never
	// synchronous the way notify.Emitter.Emit's own subscriber callback still is — so this poll is
	// no longer merely defensive, it is load-bearing.
	deadline := time.Now().Add(time.Second)
	for len(gotA.snapshot()) == 0 || len(gotB.snapshot()) == 0 {
		if time.Now().After(deadline) {
			break
		}
	}

	a, b := gotA.snapshot(), gotB.snapshot()
	if len(a) != 1 || a[0].Settings.GraphScope != "head" {
		t.Fatalf("connA received %v, want exactly one repoSettings.changed with graphScope=head", a)
	}
	if len(b) != 1 || b[0].Settings.GraphScope != "head" {
		t.Fatalf("connB received %v, want exactly one repoSettings.changed with graphScope=head (fanned out even though connB never opened repo A)", b)
	}

	// §3.18's own "via EITHER one's repoSettings.set" — the reverse direction, B setting on a
	// third repo, must fan out to both connections too, not only the direction proven above.
	gotA.reset()
	gotB.reset()
	if _, err := handlersB.Request(context.Background(), "repoSettings.set", []byte(`{
		"repoId": "/repos/c",
		"patch": {"kiraSpace.pull.strategy": "rebase"}
	}`)); err != nil {
		t.Fatalf("repoSettings.set via connB: %v", err)
	}
	deadline = time.Now().Add(time.Second)
	for len(gotA.snapshot()) == 0 || len(gotB.snapshot()) == 0 {
		if time.Now().After(deadline) {
			break
		}
	}
	a, b = gotA.snapshot(), gotB.snapshot()
	if len(a) != 1 || a[0].Settings.PullStrategy != "rebase" {
		t.Fatalf("connA received %v, want exactly one repoSettings.changed with pullStrategy=rebase (reverse direction)", a)
	}
	if len(b) != 1 || b[0].Settings.PullStrategy != "rebase" {
		t.Fatalf("connB received %v, want exactly one repoSettings.changed with pullStrategy=rebase (reverse direction)", b)
	}
}

// TestRepoSettings_WedgedConnectionDoesNotBlockOthers is the regression proof: before the fix,
// notify.Emitter.Emit called every
// repoSettings.changed subscriber SEQUENTIALLY on the repoSettings.set caller's own goroutine, and
// each subscriber's own c.Emit could block indefinitely on a slow-reading or wedged connection's
// socket. A permanently-blocked connection ("wedged") must never delay repoSettings.set's own RPC
// response for a DIFFERENT, healthy connection, nor prevent that healthy connection from receiving
// its own repoSettings.changed delivery.
func TestRepoSettings_WedgedConnectionDoesNotBlockOthers(t *testing.T) {
	router, _ := newTestRouter()

	wedged := gitsession.NewConn("conn-wedged", "client-wedged", "label-wedged", nil)
	other := gitsession.NewConn("conn-other", "client-other", "label-other", nil)
	t.Cleanup(wedged.Close)
	t.Cleanup(other.Close)

	router.ForConn(wedged) // subscribes wedged to repoSettings.changed; its Handlers are unused.
	handlersOther := router.ForConn(other)

	block := make(chan struct{}) // never closed -- simulates a client that never drains its socket.
	wedged.SetEmit(func(method string, _ any) {
		if method == "repoSettings.changed" {
			<-block
		}
	})
	var gotOther changedEventCollector
	other.SetEmit(func(method string, payload any) {
		if method == "repoSettings.changed" {
			gotOther.record(payload.(RepoSettingsChangedPayload))
		}
	})

	done := make(chan error, 1)
	go func() {
		_, err := handlersOther.Request(context.Background(), "repoSettings.set", []byte(`{
			"repoId": "/repos/a",
			"patch": {"kiraSpace.graph.scope": "head"}
		}`))
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("repoSettings.set: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("repoSettings.set never returned — a wedged OTHER connection blocked the RPC response")
	}

	deadline := time.Now().Add(time.Second)
	for len(gotOther.snapshot()) == 0 {
		if time.Now().After(deadline) {
			break
		}
	}
	got := gotOther.snapshot()
	if len(got) != 1 || got[0].Settings.GraphScope != "head" {
		t.Fatalf("the healthy connection received %v, want exactly one repoSettings.changed with graphScope=head", got)
	}
}

// P172: no connection can set the git path over the wire.
func TestSettingsSetGitPath_IsNotAMethod(t *testing.T) {
	router, _ := newTestRouter()
	conn := gitsession.NewConn("conn-a", "client-a", "label-a", nil)
	t.Cleanup(conn.Close)
	_, err := router.ForConn(conn).Request(context.Background(), "settings.setGitPath", []byte(`{"gitPath":"/x"}`))
	var ie *ipcerr.Error
	if !errors.As(err, &ie) || ie.Code != "E_UNKNOWN_METHOD" {
		t.Fatalf("err = %v, want E_UNKNOWN_METHOD", err)
	}
}

// P172: a socket client never writes the prepare script — the whole patch is refused, nothing is
// stored and no event fans out.
func TestRepoSettings_PrepareScriptIsRefusedFromTheWire(t *testing.T) {
	for name, patch := range map[string]string{
		"alone":             `{"kiraSpace.worktree.prepareScript": "touch /tmp/x"}`,
		"with allowed leaf": `{"kiraSpace.worktree.prepareScript": "touch /tmp/x", "kiraSpace.graph.pageSize": 77}`,
	} {
		t.Run(name, func(t *testing.T) {
			router, _ := newTestRouter()
			connA := gitsession.NewConn("conn-a", "client-a", "label-a", nil)
			connB := gitsession.NewConn("conn-b", "client-b", "label-b", nil)
			t.Cleanup(connA.Close)
			t.Cleanup(connB.Close)
			var got changedEventCollector
			connB.SetEmit(func(method string, payload any) {
				if method == "repoSettings.changed" {
					got.record(payload.(RepoSettingsChangedPayload))
				}
			})
			handlersA := router.ForConn(connA)
			router.ForConn(connB)

			_, err := handlersA.Request(context.Background(), "repoSettings.set",
				[]byte(`{"repoId": "/repos/a", "patch": `+patch+`}`))
			if err == nil || !strings.Contains(err.Error(), "prepare script") {
				t.Fatalf("repoSettings.set err = %v, want a prepareScript refusal", err)
			}
			var ie *ipcerr.Error
			if !errors.As(err, &ie) || ie.Code != "E_READ_ONLY" {
				t.Fatalf("err = %v, want E_READ_ONLY", err)
			}

			snap, err := router.deps.Registry.RepoSettingsGet("/repos/a")
			if err != nil {
				t.Fatalf("RepoSettingsGet: %v", err)
			}
			def := model.DefaultGitRepoSettings()
			if snap.WorktreePrepareScript != def.WorktreePrepareScript || snap.GraphPageSize != def.GraphPageSize {
				t.Fatalf("stored = %+v, want defaults (nothing written)", snap)
			}
			time.Sleep(50 * time.Millisecond)
			if n := len(got.snapshot()); n != 0 {
				t.Fatalf("repoSettings.changed delivered %d times, want 0", n)
			}
		})
	}
}
