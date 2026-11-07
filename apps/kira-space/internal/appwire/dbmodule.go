package appwire

import (
	"log/slog"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/clickhouse"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/kafka"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/kibana"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/mariadb"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/mongo"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/mysql"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/postgres"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/redis"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/s3"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/sqlite"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/sqs"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbaudit"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/enginecache"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/localauth"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskrules"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/preconnect"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tree"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/shell"
)

// dbModule is what wireAdapters builds and wireEmbedded and the teardown still reach.
type dbModule struct {
	router         *adapterhost.Router
	connectionsSvc *connections.Service
	oplogWiring    *oplog.DbSource
	connFiles      *connfiles.Loader
	apiFiles       *apifiles.Store
	apiSecrets     *apifiles.Secrets
}

// wireAdapters wires the database module's backbone over the one database: the connections and API
// config folders, variables, mask rules, the adapter router (backed by enginecache), the preconnect
// supervisor, the connections and tree services, column review, the op log and the per-launch history
// sweeps. It fills the module's fields of deps in place and runs after the prompt router and the
// approval queue exist. A settings read failure is the only error.
func (w *Wired) wireAdapters(deps *appcore.Deps, opts Options, opLog *oplog.Log) (*dbModule, error) {
	repositories, db := opts.Repos, opts.DB
	emitter := w.Emitter
	authorizer := opts.Authorizer
	if authorizer == nil {
		authorizer = localauth.New(time.Now, localauth.Evaluate, localauth.Available)
	}

	// Variables and the per-connection correlation key need a Cipher, so they are constructed apart from repos.New.
	repositories.Variables = repos.NewVariables(db.DB, opts.Cipher)
	maskKeysRepo := repos.NewMaskKeys(db.DB, opts.Cipher)

	// The connections folder. Its callbacks reach the connections service, built after it; they run
	// only once Start has, below.
	var connectionsSvc *connections.Service
	var maskRulesSvc *maskrules.Service
	var maskReviewSvc *maskreview.Service
	home := config.KiraSpaceHome()
	connFiles, err := connfiles.New(connfiles.Options{
		DB: db.DB, Dir: configfs.ModuleDir(home, connfiles.Folder),
		Secrets: &connfiles.Secrets{Store: secrets.NewStore(db.DB, opts.Cipher), DB: db.DB},
		OnChange: func(ch connfiles.Changes) {
			connectionsSvc.ConfigChanged(ch)
			bridge.EmitConnectionChanges(emitter, maskRulesSvc, maskReviewSvc, ch)
		},
		OnForget: func(id string) {
			connectionsSvc.Removed(id)
			maskReviewSvc.Forget(id)
		},
	})
	if err != nil {
		return nil, err
	}
	maskRulesSvc = maskrules.New(repositories.MaskRules, maskKeysRepo, connFiles)

	// The SAME authorizer instance connections.New below is given: that is what makes the
	// reveal grace genuinely shared between a connection-password reveal and a variable reveal.
	apiSecrets := &apifiles.Secrets{Store: secrets.NewStore(db.DB, opts.Cipher), DB: db.DB}
	apiFiles, err := apifiles.New(apifiles.Options{
		DB: db.DB, Dir: configfs.ModuleDir(home, apifiles.Folder), Secrets: apiSecrets,
		OnChange: func(ch apifiles.Changes) { bridge.EmitApiChanges(emitter, ch) },
	})
	if err != nil {
		return nil, err
	}
	deps.DB = db.DB
	deps.StartedAt = time.Now().UnixMilli()
	deps.ApiVars = apivars.New(repositories.Variables, authorizer, apiSecrets)
	deps.MaskRules = maskRulesSvc
	deps.ConnectionFiles = connFiles

	// Read from the just-migrated (possibly still-default) settings row: the cache budget needs it.
	settings, err := repositories.Settings.GetAll()
	if err != nil {
		return nil, err
	}
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
	connectionsSvc = connections.New(connections.Deps{
		Conns: repositories.Connections, Files: connFiles, Metadata: repositories.Metadata,
		Auth: authorizer, Backend: router, Preconnect: preconnectSupervisor,
	})
	connectionsSvc.Start()
	deps.Connections = connectionsSvc
	deps.Tree = tree.New(repositories.Connections, repositories.Metadata, router, connectionsSvc)

	// Configure pushes the budget to both caches (§4.9).
	router.PushCacheConfig(settings)

	// The router's in-process scheduler is the database ops' only event source.
	oplogWiring := oplog.NewDbSource(router.Host(), opLog, repositories.OpLog.PruneCategory,
		func(id string) string {
			if c, err := repositories.Connections.Get(id); err == nil && c != nil {
				return c.Name
			}
			return ""
		},
		func() int {
			s, err := repositories.Settings.GetAll()
			if err != nil {
				return model.DefaultSettings().Advanced.OpLogRetentionDays
			}
			return s.Advanced.OpLogRetentionDays
		})
	oplogWiring.Start()

	// A scratch tab's response history is swept once per launch, beside oplog's own startup prune —
	// tabs is the liveness oracle, so this removes only a closed tab's history, never a live one's.
	if err := repositories.ResponseHistory.SweepOrphans(); err != nil {
		slog.Warn("sweep orphaned response history", "scope", "startup", "err", err)
	}
	if err := repositories.GrpcHistory.SweepOrphans(); err != nil {
		slog.Warn("sweep orphaned grpc call history", "scope", "startup", "err", err)
	}
	// Return freed pages to the filesystem once the freelist is worth reclaiming — a no-op on a
	// database whose auto_vacuum stays NONE and on one whose freelist is still small.
	if err := (&repos.Maintenance{DB: db.DB}).Reclaim(); err != nil {
		slog.Warn("reclaim freed pages", "scope", "startup", "err", err)
	}

	// Column review. Built here because its popup needs the router; the connections folder's
	// callbacks above reach it through the variable and run only once connFiles.Start has.
	maskReviewSvc = maskreview.New(maskreview.Deps{
		Conns: repositories.Connections, States: connectionsSvc, Tree: deps.Tree, Rules: maskRulesSvc,
		Reviewed: repositories.MaskReview, Prompts: w.Prompts,
	})
	deps.MaskReview = maskReviewSvc
	connectionsSvc.OnMetadataInvalidated(maskReviewSvc.Connected)
	connectionsSvc.OnStateChange(func(st model.ConnectionState) {
		if st.Status == "disconnected" || st.Status == "error" {
			maskReviewSvc.Forget(st.ConnectionID)
		}
	})

	return &dbModule{
		router: router, connectionsSvc: connectionsSvc, oplogWiring: oplogWiring,
		connFiles: connFiles, apiFiles: apiFiles, apiSecrets: apiSecrets,
	}, nil
}

// dbEmbedded is wireEmbedded's own result: the embedded services the later wiring and teardown still reach.
type dbEmbedded struct {
	dbMcpSvc      *bridge.DbMcpService
	dbMcpSessions *dbmcp.SessionPool
	dockerSvc     *bridge.DockerService
	kubeSvc       *bridge.KubernetesService
	eventsDetach  func()
}

// wireEmbedded wires the embedded services: DB MCP (with its own approval broker), Docker and
// Kubernetes. Process-wide singletons (terminal registry, prompt router, agent settings, events) come
// from w.
func (w *Wired) wireEmbedded(deps appcore.Deps, installer harness.MCPInstaller, connectionsSvc *connections.Service, oplogWiring *oplog.DbSource, proofSvc *proof.Service, agentAPI *agentapi.Service, opts Options) dbEmbedded {
	// The approval broker outlives the server's own start/stop (constructed here, not inside
	// DbMcpService.startLocked), so the event subscription stays valid across a restart of the
	// embedded server within one app run. StartIfEnabled's own failure (a bind conflict) is logged,
	// never fatal.
	dbMcpApprovals := dbmcp.NewApprovalBroker(time.Now, w.Approvals)
	kubeSvc := &bridge.KubernetesService{BoundService: kubernetes.NewBoundService(deps.Repos.KubeClusters, deps.Events, append([]kubernetes.Option{kubernetes.WithRecorder(bridge.KubeRecorder{Host: deps.Router.Host()})}, opts.KubernetesExtra...)...)}
	dbMcpSvc := bridge.NewDbMcpService(deps, installer, dbMcpApprovals, proofSvc, kubernetes.ServiceOf(kubeSvc.BoundService))
	dbMcpSvc.Port = opts.DbMcpPort
	dbMcpSvc.SessionAudit = w.dbAudit
	dbMcpApprovals.SetObserver(w.dbAudit)
	dbMcpSessions := bridge.NewDbMcpSessions(dbMcpSvc)

	// The ops tools read live runs, so the server starts only once the run service exists.
	dbMcpSvc.Runs = bridge.NewScriptRunControl(w.runs)
	dbMcpSvc.Scripts = bridge.NewScriptControl(w.runs)
	dbMcpSvc.API = agentAPI
	bridge.StartDbMcpIfEnabled(dbMcpSvc)
	dockerSvc := &bridge.DockerService{BoundService: docker.NewBoundService(deps.Events, dockerOptions(opts, deps.Repos.DockerRestore)...)}
	docker.StartRestoreBound(dockerSvc.BoundService)
	eventsDetach := w.Events.AttachDb(bridge.DbSources{Connections: connectionsSvc})

	return dbEmbedded{
		dbMcpSvc: dbMcpSvc, dbMcpSessions: dbMcpSessions,
		dockerSvc: dockerSvc, kubeSvc: kubeSvc, eventsDetach: eventsDetach,
	}
}

// wireDbServices builds the proof store, the embedded services and the database module's bound
// services over the full Deps, starts the config folders and registers the teardown. It runs after
// the script runs service exists and before the task board, which takes the proof store and the DB MCP
// session pool.
func (w *Wired) wireDbServices(dbm *dbModule, opts Options) (*proof.Service, *dbmcp.SessionPool) {
	deps, repositories, emitter := w.Deps, w.Repos, w.Emitter
	if err := repositories.DbAudit.ReconcileRunning("app exited"); err != nil {
		slog.Warn("db audit: reconcile", "scope", "startup", "err", err)
	}
	w.dbAudit = dbaudit.New(dbaudit.Deps{
		Store: repositories.DbAudit, Redactor: dbm.router,
		ConnName: func(id string) string {
			if c, err := repositories.Connections.Get(id); err == nil && c != nil {
				return c.Name
			}
			return ""
		},
		Emit: func(taskID string) { emitter.Emit(adewire.ChannelDbAudit, adewire.DbAuditEvent{TaskID: taskID}) },
	})
	proofDeps := proof.Deps{
		Repo: repositories.Proof, Prompts: w.Prompts, Approvals: w.Approvals,
		Emit: func(channel string, payload any) { emitter.Emit(channel, payload) },
	}
	if opts.Clock != nil {
		proofDeps.Now = opts.Clock.Now
	}
	proofSvc := proof.New(proofDeps)
	proofSvc.PurgeStale()
	proofSvc.RaiseUnreviewed()
	proofTargets := &bridge.ProofWindowTargets{}
	shellForProof := &registryShell{windows: w.Windows}

	installer := opts.McpInstaller
	if installer == nil {
		installer = harness.ActiveInstaller(w.Agents)
	}
	agentAPI := &agentapi.Service{Envs: dbm.apiFiles, Secrets: dbm.apiSecrets, Ops: dbm.router.Host(), Globals: bridge.AgentAPIGlobals(repositories.Settings)}
	embedded := w.wireEmbedded(deps, installer, dbm.connectionsSvc, dbm.oplogWiring, proofSvc, agentAPI, opts)
	dbm.apiFiles.Start()
	dbm.connFiles.Start()

	w.Router = dbm.router
	w.App = &bridge.AppService{Deps: deps}
	w.Connections = &bridge.ConnectionsService{Deps: deps}
	w.MaskRules = &bridge.MaskRulesService{Deps: deps}
	w.ConnectionConfig = &bridge.ConnectionConfigService{Deps: deps, Files: dbm.connFiles}
	w.ConnectionFiles = dbm.connFiles
	w.Tree = &bridge.TreeService{Deps: deps}
	w.Filters = &bridge.FiltersService{Deps: deps}
	w.Queries = &bridge.QueriesService{Deps: deps}
	w.Schema = &bridge.SchemaService{Deps: deps}
	w.Kibana = &bridge.KibanaService{Deps: deps}
	w.Http = &bridge.HttpService{Deps: deps}
	w.Grpc = &bridge.GrpcService{Deps: deps}
	w.Collections = &bridge.CollectionsService{Deps: deps, Files: dbm.apiFiles}
	w.Variables = &bridge.VariablesService{Deps: deps, Files: dbm.apiFiles}
	w.ApiConfig = &bridge.ApiConfigService{Deps: deps, Files: dbm.apiFiles, Agent: agentAPI}
	w.ApiFiles = dbm.apiFiles
	w.ResponseHistory = &bridge.ResponseHistoryService{Deps: deps}
	w.GrpcHistory = &bridge.GrpcHistoryService{Deps: deps}
	w.DataGrip = &bridge.DataGripService{Deps: deps}
	w.DbMcp = embedded.dbMcpSvc
	w.Docker = embedded.dockerSvc
	w.Kubernetes = embedded.kubeSvc
	w.ProofStore = proofSvc
	w.Proof = &bridge.ProofService{
		Svc: proofSvc, Dialogs: opts.Dialogs, Windows: repositories.Windows, Events: emitter, Targets: proofTargets,
		Shell: shellForProof,
	}
	w.proofShell = shellForProof
	w.OnWindowClosing = []func(string){func(key string) {
		docker.CloseWindowBound(embedded.dockerSvc.BoundService, key)
		kubernetes.CloseWindowBound(embedded.kubeSvc.BoundService, key)
	}, proofTargets.Forget}

	w.dbTeardown = func() {
		embedded.eventsDetach()
		dbm.oplogWiring.Stop()
		// DB MCP stops before connectionsSvc.Shutdown(), not after — its stopFn abandons parked
		// approvals then waits out closeHTTP's graceful drain, so no run_query can still be
		// mid-flight, dialing a preconnect target on demand, once Shutdown below starts tearing
		// preconnect down.
		embedded.dbMcpSessions.Close()
		bridge.StopDbMcp(embedded.dbMcpSvc)
		dbm.connectionsSvc.Shutdown()
		docker.StopRestoreBound(embedded.dockerSvc.BoundService)
		kubernetes.ServiceOf(embedded.kubeSvc.BoundService).Shutdown()
		dbm.apiFiles.Close()
		dbm.connFiles.Close()
	}
	return proofSvc, embedded.dbMcpSessions
}

// registryShell drives the proof window through the window registry; main binds Open once the
// shell exists.
type registryShell struct {
	windows *shell.WindowRegistry
	open    func(shell.WindowRecord)
}

func (r *registryShell) Keys() []string              { return r.windows.Keys() }
func (r *registryShell) Focus(key string) bool       { return r.windows.Focus(key) }
func (r *registryShell) SetTitle(key, t string) bool { return r.windows.SetTitle(key, t) }
func (r *registryShell) Open(rec shell.WindowRecord) {
	if r.open != nil {
		r.open(rec)
	}
}

// BindProofShell sets how the proof window is opened, once the shell exists.
func (w *Wired) BindProofShell(open func(shell.WindowRecord)) { w.proofShell.open = open }

func dockerOptions(opts Options, restore docker.RestoreStore) []docker.Option {
	out := []docker.Option{docker.WithRestoreStore(restore)}
	if opts.DockerHubAPI != "" {
		out = append(out, docker.WithHubAPI(opts.DockerHubAPI))
	}
	return append(out, opts.DockerExtra...)
}
