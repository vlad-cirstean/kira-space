package appwire

import (
	"log/slog"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/clickhouse"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/kafka"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/kibana"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/mariadb"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/mongo"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/mysql"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/postgres"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/redis"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/s3"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/sqlite"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/sqs"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/dbmcp"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/enginecache"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/kubernetes"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/localauth"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/preconnect"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/proof"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tree"
	"github.com/kirathecat/kira-studio/internal/appupdate"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/keepawake"
	"github.com/kirathecat/kira-studio/internal/metrics"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/windowsvc"
)

// adaptersWired is wireAdapters' own result: the adapter router, the connections service, oplog's
// wiring and the metrics ticker — every piece main's own later blocks (events wiring, teardown,
// the Services list) still reach past this function's own return.
type adaptersWired struct {
	router         *adapterhost.Router
	connectionsSvc *connections.Service
	oplogWiring    *oplog.Wiring
	metricsTicker  *metrics.Ticker
}

// wireAdapters runs main's own adapter/cache/connections/tree/oplog/metrics block: the adapter
// router (backed by enginecache) -> preconnect supervisor -> connections service, started -> tree
// service -> the cache budget pushed to the router -> oplog, started -> the two per-launch history
// sweeps plus the freelist reclaim -> the process-metrics ticker, started. deps is mutated in place
// (Router/Connections/Tree) at the exact points the original sequential code set them, since
// several bridge.XxxService{Deps: deps} literals built later copy *deps by value.
func wireAdapters(deps *appcore.Deps, settings model.Settings, repositories *repos.Repos, secretsRepo *repos.SecretsRepo, cipher *secrets.Cipher, authorizer *localauth.Authorizer, db *storage.DB) adaptersWired {
	adapterDeps := adapters.Deps{Log: func(level, message string) {
		switch level {
		case "error":
			slog.Error("adapter log", "scope", "adapter", "detail", message)
		case "warn":
			slog.Warn("adapter log", "scope", "adapter", "detail", message)
		default:
			slog.Info("adapter log", "scope", "adapter", "detail", message)
		}
	}}
	goCache := enginecache.NewCache(settings.Cache.L2BudgetMb*1024*1024, adapterDeps.Log)
	router := adapterhost.NewRouter(adapterDeps, goCache)
	deps.Router = router

	preconnectSupervisor := preconnect.New()
	connectionsSvc := connections.New(connections.Deps{
		Conns: repositories.Connections, Secrets: secretsRepo, Metadata: repositories.Metadata,
		Cipher: cipher, Auth: authorizer, Backend: router, Preconnect: preconnectSupervisor,
		MaskRules: repositories.MaskRules,
	})
	connectionsSvc.Start()
	deps.Connections = connectionsSvc

	treeSvc := tree.New(repositories.Connections, repositories.Metadata, router, connectionsSvc)
	deps.Tree = treeSvc

	// Configure pushes the budget to both caches (§4.9).
	router.PushCacheConfig(settings)

	// The router's in-process scheduler is oplog's only EventSource now (P58f D9) — every kind has
	// been native since P58e, so the Node child never produces an op:start/op:end of its own to fan
	// in (enginebackend.Merge, which used to do that, is deleted).
	oplogWiring := oplog.New(router.Host(), repositories.Ops, settings.Advanced.OpLogRetentionDays)
	oplogWiring.Start()

	// P8 D7/F18: a scratch tab's response history is swept once per launch, beside oplog's own
	// startup prune — tabs is the liveness oracle (TabsRepo.Save always re-inserts every tab
	// that's currently open), so this removes only a closed tab's history, never a live one's.
	if err := repositories.ResponseHistory.SweepOrphans(); err != nil {
		slog.Warn("sweep orphaned response history", "scope", "startup", "err", err)
	}
	// P11 D11: the same startup prune, for a scratch tab's gRPC call history.
	if err := repositories.GrpcHistory.SweepOrphans(); err != nil {
		slog.Warn("sweep orphaned grpc call history", "scope", "startup", "err", err)
	}
	// P23 D5(b): return freed pages to the filesystem once the freelist is worth reclaiming — a
	// no-op on a database opened before this phase (auto_vacuum stays NONE, D5(a) never converts
	// an existing file) and on one whose freelist is still small.
	if err := (&repos.Maintenance{DB: db.DB}).Reclaim(); err != nil {
		slog.Warn("reclaim freed pages", "scope", "startup", "err", err)
	}

	metricsTicker := metrics.NewAppTicker("Kira Studio")
	metricsTicker.Start()

	return adaptersWired{
		router: router, connectionsSvc: connectionsSvc, oplogWiring: oplogWiring, metricsTicker: metricsTicker,
	}
}

// embeddedWired is wireEmbeddedServices' own result — every embedded-service handle main's later
// blocks (the Services list, teardown, the window-closing terminal cleanup) still reach past this
// function's own return.
type embeddedWired struct {
	dbMcpSvc     *bridge.DbMcpService
	keepAwakeSvc *bridge.KeepAwakeService
	windowsSvc   *bridge.WindowsService
	terminalSvc  *bridge.TerminalService
	scriptRuns   *bridge.ScriptRunsService
	runs         *scriptruns.Service
	sched        *scriptruns.Scheduler
	dockerSvc    *bridge.DockerService
	kubeSvc      *bridge.KubernetesService
	events       *bridge.Events
	eventsDetach func()
}

// wireEmbeddedServices runs main's own embedded-service block: DB MCP (with its own approval
// broker) -> keep-awake -> the windows service handle -> the embedded terminal -> the app-wide
// event bus, attached to every producer built so far. deps is taken by value, since every call
// site here is at or after the point main's own deps.Events assignment (the emitter) has already
// run.
func wireEmbeddedServices(deps appcore.Deps, installer harness.MCPInstaller, keepAwakeDriver keepawake.Driver, connectionsSvc *connections.Service, oplogWiring *oplog.Wiring, metricsTicker *metrics.Ticker, promptRouter *prompts.Router, agents harness.Selector, proofSvc *proof.Service, opts Options) embeddedWired {
	// The DB MCP server's embedded instance — owned by this app's own lifecycle.
	// StartIfEnabled's own failure (a bind conflict) is logged, never fatal.
	// The approval broker outlives the server's own start/stop (constructed here, not
	// inside DbMcpService.startLocked), so the event subscription wired below stays valid across a
	// restart of the embedded server within one app run.
	dbMcpApprovals := dbmcp.NewApprovalBroker(time.Now)
	dbMcpApprovals.Prompts = promptRouter
	kubeSvc := &bridge.KubernetesService{BoundService: kubernetes.NewBoundService(deps.Repos.KubeClusters, deps.Events, opts.KubernetesExtra...)}
	dbMcpSvc := bridge.NewDbMcpService(deps, installer, dbMcpApprovals, proofSvc, kubernetes.ServiceOf(kubeSvc.BoundService))
	bridge.StartDbMcpIfEnabled(dbMcpSvc)

	// P87 §3/§4: one keep-awake assertion for the whole app, driven by the titlebar toggle. The
	// driver is a runtime.GOOS switch — a real caffeinate child on macOS, a documented no-op
	// everywhere else.
	keepAwakeCtl := keepawake.New(keepAwakeDriver)
	keepAwakeSvc := &bridge.KeepAwakeService{Deps: deps, Ctl: keepAwakeCtl, Toggle: &keepawake.Toggle{Ctl: keepAwakeCtl}}

	// Hoisted so openNewWindow (defined below, once `app` exists) can be assigned onto
	// it — the title bar's "New window" button reaches this same OpenNewWindow closure the ⇧⌘N
	// menu command already uses. The bound methods live once in windowsvc.Service; this
	// app's own WindowsService only embeds it.
	windowsSvc := &bridge.WindowsService{Service: &windowsvc.Service{Windows: deps.Repos.Windows}}

	// P83 §3.2/§4: the embedded terminal's own bound service — a PTY registry behind a Wails
	// service plus ChannelTerminal's push channel. P128 §2.1: the bound methods live once in
	// internal/terminal.BoundService; this app's own TerminalService only embeds it.
	registry := terminal.NewRegistry()
	runs := &scriptruns.Service{
		Runs: deps.Repos.ScriptRuns, Scripts: deps.Repos.CustomScripts, Registry: registry, Home: deps.Home, App: "Studio",
		Emit:         func(r scriptruns.Run) { deps.Events.Emit(bridge.ChannelScriptRunsChanged, r) },
		EmitLog:      func(p scriptruns.LogPush) { deps.Events.Emit(bridge.ChannelScriptRunLog, p) },
		SmartTimeout: opts.SmartTimeout, ScheduleTimeout: opts.ScheduleTimeout,
		Prompts: promptRouter, LlmPrompts: deps.Prompts, Agent: agents,
	}
	if opts.Clock != nil {
		runs.Now = opts.Clock.Now
	}
	if err := runs.Recover(); err != nil {
		slog.Warn("recover script runs", "scope", "startup", "err", err)
	}
	terminalSvc := &bridge.TerminalService{BoundService: &terminal.BoundService{Emit: deps.Events, Registry: registry, Scripts: runs, Agent: agents}}
	dockerSvc := &bridge.DockerService{BoundService: docker.NewBoundService(deps.Events, dockerOptions(opts, deps.Repos.DockerRestore)...)}
	docker.StartRestoreBound(dockerSvc.BoundService)
	events := bridge.NewEvents(deps.Events)
	eventsDetach := events.Attach(bridge.Sources{Connections: connectionsSvc, Oplog: oplogWiring, Metrics: metricsTicker, DbMcp: dbMcpApprovals})

	return embeddedWired{
		dbMcpSvc: dbMcpSvc, keepAwakeSvc: keepAwakeSvc,
		windowsSvc: windowsSvc, terminalSvc: terminalSvc, dockerSvc: dockerSvc, kubeSvc: kubeSvc,
		scriptRuns: &bridge.ScriptRunsService{Bound: &scriptruns.Bound{Svc: runs}}, runs: runs,
		sched:  &scriptruns.Scheduler{Svc: runs, Clock: opts.Clock},
		events: events, eventsDetach: eventsDetach,
	}
}

// lifecycleWired is wireLifecycle's own result: the window registry, the close-flush coordinator
// and the quitter main's later blocks (openWindow, BuildMenu, app's own ShouldQuit/OnShutdown)
// still reach past this function's own return.
type lifecycleWired struct {
	windows     *shell.WindowRegistry
	closeFlush  *shell.CloseFlushCoordinator
	quitter     *shell.Quitter
	beforeFlush func()
	teardown    func()
}

// wireLifecycle runs main's own pre-app window-lifecycle block: the window registry -> the
// close-flush coordinator -> beforeFlush/teardown (today's OnShutdown, minus the ticker Stop,
// which moves to beforeFlush, run before the flush wait rather than after it — index.ts:156)
// -> the quitter built over both.
func wireLifecycle(windows *shell.WindowRegistry, events *bridge.Events, eventsDetach func(), metricsTicker *metrics.Ticker, oplogWiring *oplog.Wiring, connectionsSvc *connections.Service, dbMcpSvc *bridge.DbMcpService, keepAwakeSvc *bridge.KeepAwakeService, terminalSvc *bridge.TerminalService, dockerSvc *bridge.DockerService, kubeSvc *bridge.KubernetesService, updateInstaller *appupdate.Installer, runs *scriptruns.Service, sched *scriptruns.Scheduler, repositories *repos.Repos, db *storage.DB) lifecycleWired {
	// windows holds every currently open window's shell.Attach cleanup, keyed by that window's own
	// identity (replacing the single detachWindow/mainWindow pair that only ever worked because at
	// most one window could exist at a time). beforeFlush detaches every one of them, not just the
	// most recently created.

	// closeFlush routes each window's own "flush before close" ack back to whichever
	// shell.AttachCloseFlush hook is waiting for it (P8 C6, F8's fix) — a separate handshake from
	// the quit one below: at most one window is ever waiting at a time.
	closeFlush := shell.NewCloseFlushCoordinator(events)

	// teardown is today's OnShutdown, minus the ticker Stop (which moves to beforeFlush, run
	// before the flush wait rather than after it — P56 D3/index.ts:156).
	beforeFlush := sync.OnceFunc(func() {
		metricsTicker.Stop()
		windows.DetachAll()
	})
	teardown := sync.OnceFunc(func() {
		// P119: a Cmd+Q mid-download aborts the install rather than leaving an orphan that later
		// swaps a bundle the user quit away from. After hand-off this is a no-op.
		updateInstaller.Cancel()
		eventsDetach()
		oplogWiring.Stop()
		// DB MCP stops before connectionsSvc.Shutdown(), not after — its stopFn
		// abandons parked approvals then waits out closeHTTP's graceful drain, so no run_query can
		// still be mid-flight, dialing a preconnect target on demand, once Shutdown below starts
		// tearing preconnect down.
		bridge.StopDbMcp(dbMcpSvc)
		connectionsSvc.Shutdown()
		// P87 §4: killing the assertion early keeps the window between "app is quitting" and
		// "caffeinate is dead" as short as possible — order otherwise isn't load-bearing here, the
		// controller's release is independent of the PTY registry terminal.ShutdownBound(terminalSvc.BoundService) stops.
		bridge.StopKeepAwake(keepAwakeSvc)
		terminal.ShutdownBound(terminalSvc.BoundService)
		docker.StopRestoreBound(dockerSvc.BoundService)
		kubernetes.ServiceOf(kubeSvc.BoundService).Shutdown()
		sched.Close()
		runs.Close()
		if err := repositories.Close(); err != nil {
			slog.Warn("close repos", "scope", "shutdown", "err", err)
		}
		if err := db.Close(); err != nil {
			slog.Warn("close db", "scope", "shutdown", "err", err)
		}
	})
	quitter := shell.NewQuitter(events, beforeFlush, teardown, 2*time.Second, windows.Keys)

	return lifecycleWired{
		windows: windows, closeFlush: closeFlush, quitter: quitter, beforeFlush: beforeFlush, teardown: teardown,
	}
}

// newPromptRouter builds the popup router over the window registry (P246): lists broadcast, a
// reveal reaches one window, and advanced.notifyPrompts gates the OS notifications.
func newPromptRouter(windows *shell.WindowRegistry, repositories *repos.Repos, opts Options, reopen func()) *prompts.Router {
	emit := opts.Emitter
	var win prompts.Windows = windows
	if opts.WindowFocus != nil {
		win = focusWindows{WindowRegistry: windows, focus: opts.WindowFocus}
	}
	d := prompts.Deps{
		Windows: win, App: "Studio", Reopen: reopen,
		Emit:   func(l []prompts.Routed) { emit.Emit(bridge.ChannelPromptsChanged, l) },
		Reveal: func(key, id string) { emit.EmitTo(key, bridge.ChannelPromptsReveal, map[string]string{"id": id}) },
		Enabled: func() bool {
			s, err := repositories.Settings.GetAll()
			return err != nil || s.Advanced.NotifyPrompts
		},
		// A read error raises: showing the approval is the harmless direction.
		RaiseOnOpen: func(k prompts.Kind) bool {
			if k != prompts.KindDbMcp {
				return false
			}
			s, err := repositories.Settings.GetAll()
			return err != nil || s.DbMcp.FocusOnApproval
		},
	}
	if opts.Clock != nil {
		d.Now = opts.Clock.Now
	}
	return prompts.New(d)
}

// focusWindows swaps the registry's focus state and raise for a test seam.
type focusWindows struct {
	*shell.WindowRegistry
	focus WindowFocus
}

func (f focusWindows) Focused(key string) bool { return f.focus.Focused(key) }
func (f focusWindows) Focus(key string) bool   { return f.focus.Focus(key) }

func dockerOptions(opts Options, restore docker.RestoreStore) []docker.Option {
	out := []docker.Option{docker.WithRestoreStore(restore)}
	if opts.DockerHubAPI != "" {
		out = append(out, docker.WithHubAPI(opts.DockerHubAPI))
	}
	return append(out, opts.DockerExtra...)
}
