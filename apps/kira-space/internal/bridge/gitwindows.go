package bridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitaskpass"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// nativeClientID/nativeLabel identify the native graph's own gitsession.Conn (ClientID/
// ClientLabel) — used only for attribution (e.g. the undo slot's "made in this window" label),
// never for trust: there is no pairing here to trust (docs/HISTORY.md (v1.5 C10)
// §3.2).
const (
	nativeClientID = "kira-native"
	nativeLabel    = "Kira Space"
)

// newStreamConnID mints a per-window gitsession.ConnID. There is no handshake here to mint one
// for us — a fresh random id per Conn is all gitsession.Conn needs it for: keying this window's own
// repo holds and walk state, never anything cross-process.
func newStreamConnID() gitsession.ConnID {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf) // crypto/rand.Read never errors on a Reader that never fails to fill.
	return gitsession.ConnID(hex.EncodeToString(buf))
}

type requestFn = func(ctx context.Context, method string, params json.RawMessage) (any, error)

// patchTouchesRestrictedField reports whether patch sets a field repoSettings.set must refuse from
// a renderer: the worktree prepare script and base path are written only in-process by Kira Space
// (the ADE repo config).
func patchTouchesRestrictedField(patch gitrpc.RepoSettingsPatchWire) bool {
	return patch.WorktreePrepareScript != nil || patch.WorktreeBasePath != nil
}

// repoSettingsSetTouchesRestrictedField is patchTouchesRestrictedField over raw stream params.
func repoSettingsSetTouchesRestrictedField(params json.RawMessage) bool {
	var p gitrpc.RepoSettingsSetParams
	if err := json.Unmarshal(params, &p); err != nil {
		// Malformed params: let it through to the real handler, which rejects it properly. This
		// wrapper only ever narrows what the allowlist admits — it is never a substitute for the
		// inner handler's own request validation.
		return false
	}
	return patchTouchesRestrictedField(p.Patch)
}

func errRestrictedSetting() error {
	return ipcerr.T(ipcerr.CodeReadOnly, "backend.space.gitrpc.fieldUnavailable.body", nil)
}

// guardRepoSettingsSet adds the one FIELD-level restriction the stream needs: repoSettings.set is
// served, but only for a patch that leaves every restricted field absent.
func guardRepoSettingsSet(next requestFn) requestFn {
	return func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		if method == "repoSettings.set" && repoSettingsSetTouchesRestrictedField(params) {
			return nil, errRestrictedSetting()
		}
		return next(ctx, method, params)
	}
}

// CredentialAsker is the relay a window's credential prompts go to (gitcred.Relay).
type CredentialAsker interface {
	AskFrom(ctx context.Context, origin, source string, req gitaskpass.Request) (string, bool)
}

// GitWindows owns one gitsession.Conn per window key, shared by GitService and every git stream of
// that window. Not a bound service: Close must not be callable from a page.
type GitWindows struct {
	router *gitrpc.Router
	relay  CredentialAsker
	events appevent.Emitter

	mu    sync.Mutex
	byKey map[string]*gitWindow
}

type gitWindow struct {
	conn     *gitsession.Conn
	handlers gitrpc.Handlers
	streams  int
	// bound holds repoIds opened through GitService; they keep the Conn alive past its last stream.
	bound map[string]struct{}
}

// NewGitWindows builds the registry; events receives every git event as kira:git:<event>.
func NewGitWindows(router *gitrpc.Router, relay CredentialAsker, events appevent.Emitter) *GitWindows {
	return &GitWindows{router: router, relay: relay, events: events, byKey: map[string]*gitWindow{}}
}

// get returns the window's Conn, creating it on first use. The empty key is valid (flow harness).
func (g *GitWindows) get(key string) *gitWindow {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.getLocked(key)
}

// lookup returns the window's Conn only if one exists. A stream for a closed window must not
// resurrect it: nothing would close the new Conn.
func (g *GitWindows) lookup(key string) (*gitWindow, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	w, ok := g.byKey[key]
	return w, ok
}

// getLocked creates the Conn with credential prompts routed to the relay tagged with key.
func (g *GitWindows) getLocked(key string) *gitWindow {
	if w, ok := g.byKey[key]; ok {
		return w
	}
	conn := gitsession.NewConn(newStreamConnID(), nativeClientID, nativeLabel, nil)
	conn.Module = oplog.ModuleGit
	conn.RouteCredentials(func(ctx context.Context, req gitaskpass.Request) (string, bool) {
		return g.relay.AskFrom(ctx, key, nativeLabel, req)
	})
	// Still opted out, but no longer because writes were refused here — a periodic
	// background `git fetch --prune` is a network write the user never pressed a button for, and
	// every fetch this phase admits is one they did.
	// One deleted line whenever someone wants automatic background fetching for this Conn.
	conn.DisableAutoFetch()
	w := &gitWindow{conn: conn, handlers: g.router.ForConn(conn)}
	conn.SetEmit(func(method string, payload any) {
		g.events.EmitTo(key, GitEventPrefix+method, payload)
	})
	g.byKey[key] = w
	return w
}

// Close releases the window's Conn and every hold on it. Idempotent.
func (g *GitWindows) Close(key string) {
	g.mu.Lock()
	w, ok := g.byKey[key]
	delete(g.byKey, key)
	g.mu.Unlock()
	if ok {
		w.conn.Close()
	}
}

// noteBoundOpen records a GitService-held repo so the window's Conn outlives its last stream.
func (g *GitWindows) noteBoundOpen(key, repoID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	w, ok := g.byKey[key]
	if !ok {
		return
	}
	if w.bound == nil {
		w.bound = map[string]struct{}{}
	}
	w.bound[repoID] = struct{}{}
}

// noteBoundClose forgets a GitService-held repo.
func (g *GitWindows) noteBoundClose(key, repoID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if w, ok := g.byKey[key]; ok {
		delete(w.bound, repoID)
	}
}

// attachStream counts a git stream on the window; the last detach releases the Conn, as the
// per-stream Conn was released on stream close.
func (g *GitWindows) attachStream(key string) (*gitWindow, func()) {
	g.mu.Lock()
	w := g.getLocked(key)
	w.streams++
	g.mu.Unlock()
	return w, func() {
		g.mu.Lock()
		w.streams--
		last := w.streams == 0 && len(w.bound) == 0 && g.byKey[key] == w
		if last {
			delete(g.byKey, key)
		}
		g.mu.Unlock()
		if last {
			w.conn.Close()
		}
	}
}
