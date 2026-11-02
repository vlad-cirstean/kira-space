package ipcfixture

import (
	"context"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"sync"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/enginecache"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/maskrules"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/preconnect"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tree"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// cacheBudgetBytes matches the plan's own §4.2 illustration (64<<20) — any budget large enough
// that a fixture scenario never evicts under memory pressure works; the exact number is not itself
// under test here (internal/enginecache has its own suite for that).
const cacheBudgetBytes = 64 << 20

// App wires the real app stack a fixture generator needs: storage through the real
// migrations, a real (Linux-fallback) cipher, connections.Service and tree.Service over a real
// adapterhost.Router/Dispatcher, and the three bridge services every committed fixture's channels
// route through. It is the Go analogue of tests/ipc/support/harness.ts, built with strictly higher
// fidelity: no connectionSummaryOf fabrication and no Map-backed tree stand-in, because the real
// services already exist to call.
type App struct {
	Repos       *repos.Repos
	Secrets     *repos.SecretsRepo
	Cipher      *secrets.Cipher
	Connections *connections.Service
	Tree        *tree.Service
	Router      *adapterhost.Router
	Dispatcher  *adapterhost.Dispatcher

	ConnectionsSvc *bridge.ConnectionsService
	TreeSvc        *bridge.TreeService
	OpsSvc         *bridge.OpsService
	MaskRulesSvc   *bridge.MaskRulesService

	// connectedMu guards connectedIDs — every real adapter this harness connects
	// (Recorder.ConnectionsConnect), tracked so NewApp's own t.Cleanup can disconnect all of them
	// automatically. A mutex, not a bare slice, since nothing here assumes a single caller
	// goroutine for the lifetime of one App.
	connectedMu  sync.Mutex
	connectedIDs []string
}

// trackConnected records id as a connection this harness successfully connected — F9's own
// bookkeeping for NewApp's automatic disconnect cleanup.
func (a *App) trackConnected(id string) {
	a.connectedMu.Lock()
	defer a.connectedMu.Unlock()
	a.connectedIDs = append(a.connectedIDs, id)
}

// disconnectTracked disconnects every connection trackConnected has recorded so far — F9's own
// cleanup half, split out from NewApp's t.Cleanup closure so it is independently testable (a real
// connect needs Docker; this mechanism does not).
func (a *App) disconnectTracked() {
	a.connectedMu.Lock()
	ids := append([]string(nil), a.connectedIDs...)
	a.connectedMu.Unlock()
	for _, id := range ids {
		_ = a.Router.Disconnect(context.Background(), id)
	}
}

// NewApp builds one App per test, in a fresh temp KIRA_HOME, and registers cleanup in the reverse
// order main.go's own teardown does.
func NewApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("KIRA_HOME", t.TempDir())
	// Required on Linux (CLAUDE.md's Secrets section) — the real macOS Keychain path is untestable
	// here, and every fixture scenario needs a working cipher to store a connection's password.
	t.Setenv("KIRA_INSECURE_SECRETS", "1")

	db, err := storage.Open()
	if err != nil {
		t.Fatalf("ipcfixture: storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	r, err := repos.New(db.DB)
	if err != nil {
		t.Fatalf("ipcfixture: repos.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	cipher := secrets.New()
	if !cipher.Status().Available {
		t.Fatalf("ipcfixture: cipher unavailable: %+v", cipher.Status())
	}
	secretsRepo := repos.NewSecrets(db.DB, cipher)

	deps := adapters.Deps{Log: func(level, message string) {}}
	cache := enginecache.NewCache(cacheBudgetBytes, deps.Log)
	router := adapterhost.NewRouter(deps, cache)
	dispatcher := adapterhost.NewDispatcher(router.Host(), cache)

	pre := preconnect.New()
	connectionsSvc := connections.New(connections.Deps{
		Conns: r.Connections, Secrets: secretsRepo, Metadata: r.Metadata,
		Cipher: cipher, Backend: router, Preconnect: pre, MaskRules: r.MaskRules,
	})
	connectionsSvc.Start()
	t.Cleanup(connectionsSvc.Shutdown)

	treeSvc := tree.New(r.Connections, r.Metadata, router, connectionsSvc)

	// M5 §4.6/§9: the same "needs a Cipher, constructed separately" shape as secretsRepo above.
	maskKeysRepo := repos.NewMaskKeys(db.DB, cipher)
	maskRulesSvc := maskrules.New(r.MaskRules, maskKeysRepo)

	appDeps := appcore.Deps{
		DB: db.DB, Repos: r, Connections: connectionsSvc, Tree: treeSvc, Router: router,
		MaskRules: maskRulesSvc,
	}

	app := &App{
		Repos: r, Secrets: secretsRepo, Cipher: cipher, Connections: connectionsSvc, Tree: treeSvc, Router: router, Dispatcher: dispatcher,
		ConnectionsSvc: &bridge.ConnectionsService{Deps: appDeps},
		TreeSvc:        &bridge.TreeService{Deps: appDeps},
		OpsSvc:         &bridge.OpsService{Deps: appDeps, Canceller: router},
		MaskRulesSvc:   &bridge.MaskRulesService{Deps: appDeps},
	}
	// Every *_test.go file using this harness calls Recorder.ConnectionsConnect
	// and none of them call the matching disconnect — real adapters otherwise accumulate in the
	// process-global registry (adapters.live) across test runs, well after the test DB and
	// everything else this func's own t.Cleanup calls above have already gone. Registered last, so
	// it runs first (t.Cleanup is LIFO): live adapters are disconnected before
	// connectionsSvc.Shutdown tears down preconnect sidecars, and well before db.Close()/r.Close()
	// above.
	t.Cleanup(app.disconnectTracked)
	return app
}

// SeedConnection inserts a connection row through the real repo (bypassing only
// connections.Service.Create's own random uuid.NewString() assignment, so the fixture's connection
// id is the fixed literal every committed fixture already carries — a harness-only deviation, the
// same one internal/tree's and internal/connections' own fake-backend tests already make) and
// stores its password through the real secrets repo and its URI sealed under ScopeConnectionURI.
func (a *App) SeedConnection(t *testing.T, id string, fields model.ConnectionFields, password *string) model.ConnectionSummary {
	t.Helper()
	var uriEnc *string
	if fields.URI != nil {
		enc, err := a.Cipher.Encrypt(secrets.ScopeConnectionURI, *fields.URI)
		if err != nil {
			t.Fatalf("ipcfixture: seal uri %s: %v", id, err)
		}
		uriEnc = &enc
	}
	created, err := a.Repos.Connections.InsertWithSecret(id, fields, kiratime.NowISO(), nil, uriEnc)
	if err != nil {
		t.Fatalf("ipcfixture: seed connection %s: %v", id, err)
	}
	if err := a.Secrets.Set(id, password); err != nil {
		t.Fatalf("ipcfixture: seed secret %s: %v", id, err)
	}
	return created
}

// Recorder is one scenario's worth of ControlSnapshot/PortSnapshot capture (tests/ipc/support/
// harness.ts's own openHarness() closures, restated as methods since Go has no closures-over-
// mutable-slice idiom as light as JS's). Each method both drives the real call and appends its own
// fixture entry — the same "one call site, one snapshot" discipline every backend.spec.ts follows.
type Recorder struct {
	App     *App
	Control []ControlSnapshot
	Port    []PortSnapshot
}

func NewRecorder(app *App) *Recorder { return &Recorder{App: app} }

func (r *Recorder) recordControl(channel string, args, response any) {
	r.Control = append(r.Control, ControlSnapshot{Channel: channel, Args: rawJSON(args), Response: rawJSON(response)})
}

func (r *Recorder) recordPort(op string, payload, response any, delayMs *int) {
	r.Port = append(r.Port, PortSnapshot{Op: op, Payload: rawJSON(payload), Response: rawJSON(response), DelayMs: delayMs})
}

// OverrideLastControlResponse replaces the most recently recorded ControlSnapshot's Response —
// for the rare case where a Recorder call's real answer carries a value the committed fixture
// freezes (e.g. mysql's InnoDB row-count estimate, frozen.go's FreezeNodeDetail) but the test
// itself still needs the real, unfrozen result to keep driving the scenario.
func (r *Recorder) OverrideLastControlResponse(response any) {
	r.Control[len(r.Control)-1].Response = rawJSON(response)
}

// ConnectionsList is IPC.connectionsList: bridge.ConnectionsService.List, frozen per §4.3(d).
func (r *Recorder) ConnectionsList(t *testing.T) []model.ConnectionSummary {
	t.Helper()
	list, err := r.App.ConnectionsSvc.List()
	if err != nil {
		t.Fatalf("ipcfixture: connections list: %v", err)
	}
	frozen := make([]FrozenConnectionSummary, len(list))
	for i, s := range list {
		frozen[i] = FreezeConnectionSummary(s)
	}
	r.recordControl(channelConnectionsList, nil, frozen)
	return list
}

// ConnectionsStates is IPC.connectionsStates: bridge.ConnectionsService.States.
func (r *Recorder) ConnectionsStates(t *testing.T) []model.ConnectionState {
	t.Helper()
	states, err := r.App.ConnectionsSvc.States()
	if err != nil {
		t.Fatalf("ipcfixture: connections states: %v", err)
	}
	if states == nil {
		states = []model.ConnectionState{}
	}
	r.recordControl(channelConnectionsStates, nil, states)
	return states
}

// ConnectionsConnect is IPC.connectionsConnect: bridge.ConnectionsService.Connect.
func (r *Recorder) ConnectionsConnect(t *testing.T, id string) model.ConnectionState {
	t.Helper()
	state, err := r.App.ConnectionsSvc.Connect(bridge.ConnectionsIDArgs{ID: id})
	if err != nil {
		t.Fatalf("ipcfixture: connect %s: %v", id, err)
	}
	if state.Status != "connected" {
		msg := ""
		if state.Error != nil {
			msg = *state.Error
		}
		t.Fatalf("ipcfixture: connect %s: status=%s error=%s", id, state.Status, msg)
	}
	r.App.trackConnected(id) // F9: NewApp's own t.Cleanup disconnects this automatically.
	r.recordControl(channelConnectionsConnect, bridge.ConnectionsIDArgs{ID: id}, FreezeConnectionState(state))
	return state
}

// TreeChildren is IPC.treeChildren: bridge.TreeService.Children.
func (r *Recorder) TreeChildren(t *testing.T, connectionID, path string, refresh bool) tree.ChildrenResult {
	t.Helper()
	args := bridge.TreeChildrenArgs{ConnectionID: connectionID, Path: path, Refresh: refresh}
	result, err := r.App.TreeSvc.Children(args)
	if err != nil {
		t.Fatalf("ipcfixture: tree children %s %q: %v", connectionID, path, err)
	}
	if result.Nodes == nil {
		result.Nodes = []model.TreeNode{}
	}
	r.recordControl(channelTreeChildren, args, result)
	return result
}

// TreeDescribe is IPC.treeDescribe: bridge.TreeService.Describe.
func (r *Recorder) TreeDescribe(t *testing.T, connectionID, path string, refresh bool, tabID *string) tree.DescribeResult {
	t.Helper()
	args := bridge.TreeDescribeArgs{ConnectionID: connectionID, Path: path, Refresh: refresh, TabID: tabID}
	result, err := r.App.TreeSvc.Describe(args)
	if err != nil {
		t.Fatalf("ipcfixture: tree describe %s %q: %v", connectionID, path, err)
	}
	r.recordControl(channelTreeDescribe, args, result)
	return result
}

// TreeDefinition is IPC.treeDefinition: bridge.TreeService.Definition.
func (r *Recorder) TreeDefinition(t *testing.T, connectionID, path string, refresh bool, tabID *string) tree.DefinitionResult {
	t.Helper()
	args := bridge.TreeDescribeArgs{ConnectionID: connectionID, Path: path, Refresh: refresh, TabID: tabID}
	result, err := r.App.TreeSvc.Definition(args)
	if err != nil {
		t.Fatalf("ipcfixture: tree definition %s %q: %v", connectionID, path, err)
	}
	r.recordControl(channelTreeDefinition, args, result)
	return result
}

// OpsCancel is IPC.opsCancel: bridge.OpsService.Cancel. Every committed fixture captures this
// channel with no args and no response (the TypeScript spec pushed `{channel: IPC.opsCancel}`
// literally, never the real opId or the void result) — matched here rather than "improved", so a
// regenerated fixture does not gratuitously diff read-mode's own committed file.
func (r *Recorder) OpsCancel(t *testing.T, opID string) bool {
	t.Helper()
	cancelled, err := r.App.Router.Cancel(context.Background(), opID)
	if err != nil {
		t.Fatalf("ipcfixture: cancel %s: %v", opID, err)
	}
	r.recordControl(channelOpsCancel, nil, nil)
	return cancelled
}

// DataRead is DATA_OP.read: adapterhost.Dispatcher.Read, decoded to a LogicalPage per §4.3(b) and
// wrapped the same way every backend.spec.ts wraps it: {kind: 'read', page, source}.
func (r *Recorder) DataRead(t *testing.T, req adapterhost.ReadRequestWire, delayMs *int) adapterhost.ReadResponse {
	t.Helper()
	resp, err := r.App.Dispatcher.Read(context.Background(), req)
	if err != nil {
		t.Fatalf("ipcfixture: data read %s: %v", req.OpID, err)
	}
	logical, err := DecodePage(resp.Page)
	if err != nil {
		t.Fatalf("ipcfixture: decode page for %s: %v", req.OpID, err)
	}
	response := struct {
		Kind   string `json:"kind"`
		Page   any    `json:"page"`
		Source string `json:"source"`
	}{Kind: "read", Page: logical, Source: resp.Source}
	r.recordPort(dataOpRead, req, response, delayMs)
	return resp
}

// DataCount is DATA_OP.count: adapterhost.Dispatcher.Count, wrapped {kind: 'count', value, exact,
// stale, source} the same way every backend.spec.ts wraps it.
func (r *Recorder) DataCount(t *testing.T, req adapterhost.CountRequestWire) adapterhost.CountResponse {
	t.Helper()
	resp, err := r.App.Dispatcher.Count(context.Background(), req)
	if err != nil {
		t.Fatalf("ipcfixture: data count %s: %v", req.OpID, err)
	}
	response := struct {
		Kind   string `json:"kind"`
		Value  int64  `json:"value"`
		Exact  bool   `json:"exact"`
		Stale  bool   `json:"stale"`
		Source string `json:"source"`
	}{Kind: "count", Value: resp.Value, Exact: resp.Exact, Stale: false, Source: resp.Source}
	r.recordPort(dataOpCount, req, response, nil)
	return resp
}

// DataExecute is DATA_OP.execute: adapterhost.Dispatcher.Execute (console/SQL-mode statements),
// wrapped {kind: 'execute', pages} the same way every backend.spec.ts wraps it.
func (r *Recorder) DataExecute(t *testing.T, req adapterhost.ExecuteRequestWire) adapterhost.ExecuteResponse {
	t.Helper()
	resp, err := r.App.Dispatcher.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("ipcfixture: data execute %s: %v", req.OpID, err)
	}
	logicalPages := make([]any, len(resp.Pages))
	for i, p := range resp.Pages {
		logical, err := DecodePage(p)
		if err != nil {
			t.Fatalf("ipcfixture: decode page %d for %s: %v", i, req.OpID, err)
		}
		logicalPages[i] = logical
	}
	response := struct {
		Kind  string `json:"kind"`
		Pages []any  `json:"pages"`
	}{Kind: "execute", Pages: logicalPages}
	r.recordPort(dataOpExecute, req, response, nil)
	return resp
}

// DataMutate is DATA_OP.mutate: adapterhost.Dispatcher.Mutate, wrapped {kind: 'mutate',
// affectedRows} the same way every backend.spec.ts wraps it.
func (r *Recorder) DataMutate(t *testing.T, req adapterhost.MutateRequestWire) adapterhost.MutateResponse {
	t.Helper()
	resp, err := r.App.Dispatcher.Mutate(context.Background(), req)
	if err != nil {
		t.Fatalf("ipcfixture: data mutate %s: %v", req.OpID, err)
	}
	response := struct {
		Kind         string `json:"kind"`
		AffectedRows int    `json:"affectedRows"`
	}{Kind: "mutate", AffectedRows: resp.AffectedRows}
	r.recordPort(dataOpMutate, req, response, nil)
	return resp
}

// DataInvalidate is DATA_OP.invalidate: adapterhost.Dispatcher.Invalidate, wrapped {kind:
// 'invalidate'} — the op itself returns nothing, matching data-ops.ts's own void response. The
// recorded payload omits Scope when empty (every committed fixture's own invalidate payload
// carries only {connectionId, path} — data-ops.ts's own Scope is optional and Go's
// InvalidateRequestWire models its "" default, not its absence, so the wire type itself isn't
// reused for recording here, only for the real call).
func (r *Recorder) DataInvalidate(t *testing.T, req adapterhost.InvalidateRequestWire) {
	t.Helper()
	r.App.Dispatcher.Invalidate(req)
	payload := struct {
		ConnectionID string `json:"connectionId"`
		Path         string `json:"path"`
		Scope        string `json:"scope,omitempty"`
	}{ConnectionID: req.ConnectionID, Path: req.Path, Scope: req.Scope}
	response := struct {
		Kind string `json:"kind"`
	}{Kind: "invalidate"}
	r.recordPort(dataOpInvalidate, payload, response, nil)
}
