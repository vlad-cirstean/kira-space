package appwire

import (
	"context"
	"database/sql"
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
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/enginecache"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/homefold"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/localauth"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/preconnect"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiooplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage"
	studiomodel "github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	studiorepos "github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tree"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/harness"
)

// studioAdapters is wireStudioAdapters' own result: the adapter router, the connections service, oplog's
// wiring — every piece the later wiring and teardown still reach.
type studioAdapters struct {
	router         *adapterhost.Router
	connectionsSvc *connections.Service
	oplogWiring    *studiooplog.Wiring
}

// wireStudioAdapters runs main's own adapter/cache/connections/tree/oplog block: the adapter
// router (backed by enginecache) -> preconnect supervisor -> connections service, started -> tree
// service -> the cache budget pushed to the router -> oplog, started -> the two per-launch history
// sweeps plus the freelist reclaim . deps is mutated in place
// (Router/Connections/Tree) at the exact points the original sequential code set them, since
// several bridge.XxxService{Deps: deps} literals built later copy *deps by value.
func wireStudioAdapters(deps *appcore.StudioDeps, settings studiomodel.Settings, repositories *studiorepos.Repos, files *connfiles.Loader, authorizer *localauth.Authorizer, db *studiostorage.DB) studioAdapters {
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
		Conns: repositories.Connections, Files: files, Metadata: repositories.Metadata,
		Auth: authorizer, Backend: router, Preconnect: preconnectSupervisor,
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
	oplogWiring := studiooplog.New(router.Host(), repositories.Ops, settings.Advanced.OpLogRetentionDays)
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
	if err := (&studiorepos.Maintenance{DB: db.DB}).Reclaim(); err != nil {
		slog.Warn("reclaim freed pages", "scope", "startup", "err", err)
	}

	return studioAdapters{
		router: router, connectionsSvc: connectionsSvc, oplogWiring: oplogWiring,
	}
}

// studioEmbedded is wireStudioEmbedded's own result: the embedded services the later wiring and
// teardown still reach.
type studioEmbedded struct {
	dbMcpSvc      *bridge.DbMcpService
	dbMcpSessions *dbmcp.SessionPool
	dockerSvc     *bridge.DockerService
	kubeSvc       *bridge.KubernetesService
	eventsDetach  func()
	// stopReregister cancels and waits out the post-fold DB MCP re-registration.
	stopReregister func()
}

// wireStudioEmbedded wires the Studio-only embedded services: DB MCP (with its own approval
// broker), Docker and Kubernetes. Process-wide singletons (terminal
// registry, prompt router, agent settings, events) come from w, built by the Space side first.
func (w *Wired) wireStudioEmbedded(deps appcore.StudioDeps, installer harness.MCPInstaller, connectionsSvc *connections.Service, oplogWiring *studiooplog.Wiring, proofSvc *proof.Service, agentAPI *agentapi.Service, opts Options) studioEmbedded {
	// The approval broker outlives the server's own start/stop (constructed here, not inside
	// DbMcpService.startLocked), so the event subscription stays valid across a restart of the
	// embedded server within one app run. StartIfEnabled's own failure (a bind conflict) is logged,
	// never fatal.
	dbMcpApprovals := dbmcp.NewApprovalBroker(time.Now, w.Approvals)
	kubeSvc := &bridge.KubernetesService{BoundService: kubernetes.NewBoundService(deps.Repos.KubeClusters, deps.Events, append([]kubernetes.Option{kubernetes.WithRecorder(bridge.KubeRecorder{Host: deps.Router.Host()})}, opts.KubernetesExtra...)...)}
	dbMcpSvc := bridge.NewDbMcpService(deps, installer, dbMcpApprovals, proofSvc, kubernetes.ServiceOf(kubeSvc.BoundService))
	dbMcpSvc.Port = opts.DbMcpPort
	dbMcpSessions := bridge.NewDbMcpSessions(dbMcpSvc)

	// The ops tools read live runs, so the server starts only once the run service exists.
	dbMcpSvc.Runs = bridge.NewScriptRunControl(w.runs)
	dbMcpSvc.Scripts = bridge.NewScriptControl(w.runs)
	dbMcpSvc.API = agentAPI
	bridge.StartDbMcpIfEnabled(dbMcpSvc)
	stopReregister := reregisterAfterFold(dbMcpSvc, w.Repos.CustomScripts.DB)
	dockerSvc := &bridge.DockerService{BoundService: docker.NewBoundService(deps.Events, dockerOptions(opts, deps.Repos.DockerRestore)...)}
	docker.StartRestoreBound(dockerSvc.BoundService)
	eventsDetach := w.Events.AttachStudio(bridge.StudioSources{Connections: connectionsSvc, Oplog: oplogWiring})

	return studioEmbedded{
		dbMcpSvc: dbMcpSvc, dbMcpSessions: dbMcpSessions,
		dockerSvc: dockerSvc, kubeSvc: kubeSvc, eventsDetach: eventsDetach, stopReregister: stopReregister,
	}
}

// reregisterAfterFold rewrites the Claude Code kira-db registration once after a home fold, in the
// background. An unfinished attempt (server off, install failed) stays flagged and retries next start.
func reregisterAfterFold(svc *bridge.DbMcpService, db *sql.DB) func() {
	need, err := homefold.NeedsReregister(db)
	if err != nil {
		slog.Warn("db mcp: read fold marker", "scope", "dbmcp", "err", err)
		return func() {}
	}
	if !need {
		return func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if bridge.ReregisterAfterFold(ctx, svc) && ctx.Err() == nil {
			if err := homefold.ClearReregister(db); err != nil {
				slog.Warn("db mcp: clear fold marker", "scope", "dbmcp", "err", err)
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func dockerOptions(opts Options, restore docker.RestoreStore) []docker.Option {
	out := []docker.Option{docker.WithRestoreStore(restore)}
	if opts.DockerHubAPI != "" {
		out = append(out, docker.WithHubAPI(opts.DockerHubAPI))
	}
	return append(out, opts.DockerExtra...)
}
