package appwire

import (
	"log/slog"
	"path/filepath"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/localauth"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskrules"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	studiomodel "github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	studiorepos "github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
	"github.com/kirathecat/kira-studio/internal/configfs/configsvc"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

// buildStudio wires the Studio services over Studio's own DB and repos, on top of the process-wide
// singletons the Space side already built (approvals, prompt router, window registry, terminal
// registry, agent stores, events). It fills the Studio fields of w and the Studio halves of the
// merged services. A settings read failure is the only error.
func (w *Wired) buildStudio(opts Options) error {
	repositories, db := opts.StudioRepos, opts.StudioDB
	emitter := w.Emitter
	registry := w.Agents.Registry
	repositories.CustomScripts.Agents = registry
	repositories.CustomScripts.Secrets = secrets.NewStore(repositories.CustomScripts.DB, opts.Cipher)
	authorizer := opts.Authorizer
	if authorizer == nil {
		authorizer = localauth.New(time.Now, localauth.Evaluate, localauth.Available)
	}

	// P5: needs a Cipher, so constructed separately from repos.New's aggregate.
	repositories.Variables = studiorepos.NewVariables(db.DB, opts.Cipher)
	// M5 §2.5/§3.2: the per-connection correlation key column, same "needs a Cipher" shape.
	maskKeysRepo := studiorepos.NewMaskKeys(db.DB, opts.Cipher)

	// The connections folder. Its callbacks reach the connections service, built after it; they run
	// only once Start has, below.
	var connectionsSvc *connections.Service
	var maskRulesSvc *maskrules.Service
	var maskReviewSvc *maskreview.Service
	studioHome := config.StudioHome()
	connFiles, err := connfiles.New(connfiles.Options{
		DB: db.DB, Dir: filepath.Join(studioHome, "connections"),
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
		return err
	}
	maskRulesSvc = maskrules.New(repositories.MaskRules, maskKeysRepo, connFiles)

	// P5 D8: the SAME authorizer instance connections.New below is given: that is what makes the
	// reveal grace genuinely shared between a connection-password reveal and a variable reveal.
	apiSecrets := &apifiles.Secrets{Store: secrets.NewStore(db.DB, opts.Cipher), DB: db.DB}
	apiFiles, err := apifiles.New(apifiles.Options{
		DB: db.DB, Dir: filepath.Join(studioHome, "api"), Secrets: apiSecrets,
		OnChange: func(ch apifiles.Changes) { bridge.EmitApiChanges(emitter, ch) },
	})
	if err != nil {
		return err
	}
	deps := appcore.StudioDeps{
		DB:        db.DB,
		Home:      studioHome,
		StartedAt: time.Now().UnixMilli(),
		Repos:     repositories,
		ApiVars:   apivars.New(repositories.Variables, authorizer, apiSecrets),
		MaskRules: maskRulesSvc,
		Prompts:   w.Deps.Prompts,
		Events:    emitter,

		ConnectionFiles: connFiles,
	}

	// Read from the just-migrated (possibly still-default) settings row: the cache budget needs it.
	settings, err := deps.Repos.Settings.GetAll()
	if err != nil {
		return err
	}
	adapters := wireStudioAdapters(&deps, settings, repositories, connFiles, authorizer, db)
	router := adapters.router
	connectionsSvc = adapters.connectionsSvc

	// P534: column review. Built here because its popup needs the router; the connections folder's
	// callbacks above reach it through the variable and run only once connFiles.Start has.
	maskReviewSvc = maskreview.New(maskreview.Deps{
		Conns: repositories.Connections, States: connectionsSvc, Tree: deps.Tree, Rules: maskRulesSvc,
		Reviewed: repositories.MaskReview, Prompts: w.Prompts,
	})
	deps.MaskReview = maskReviewSvc
	connectionsSvc.OnMetadataInvalidated(maskReviewSvc.Connected)
	connectionsSvc.OnStateChange(func(st studiomodel.ConnectionState) {
		if st.Status == "disconnected" || st.Status == "error" {
			maskReviewSvc.Forget(st.ConnectionID)
		}
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
	agentAPI := &agentapi.Service{Envs: apiFiles, Secrets: apiSecrets, Ops: router.Host(), Globals: bridge.AgentAPIGlobals(repositories.Settings)}
	embedded := w.wireStudioEmbedded(deps, installer, connectionsSvc, adapters.oplogWiring, proofSvc, agentAPI, opts)
	embedded.sched.Start()
	apiFiles.Start()
	connFiles.Start()
	if w.AgentSettings != nil {
		if err := w.AgentSettings.Sweep(filepath.Join(studioHome, "automations-mcp")); err != nil {
			slog.Warn("agent settings: sweep studio", "scope", "startup", "err", err)
		}
	}

	w.StudioRepos, w.StudioDeps, w.Router = repositories, deps, router
	w.App = &bridge.AppService{Deps: deps}
	w.StudioSettings = &bridge.StudioSettingsService{Deps: deps}
	w.Connections = &bridge.ConnectionsService{Deps: deps}
	w.MaskRules = &bridge.MaskRulesService{Deps: deps}
	w.ConnectionConfig = &bridge.ConnectionConfigService{Deps: deps, Files: connFiles}
	w.ConnectionFiles = connFiles
	w.Tree = &bridge.TreeService{Deps: deps}
	w.StudioOps = &bridge.StudioOpsService{Deps: deps, Canceller: router}
	w.Filters = &bridge.FiltersService{Deps: deps}
	w.Queries = &bridge.QueriesService{Deps: deps}
	w.Schema = &bridge.SchemaService{Deps: deps}
	w.Kibana = &bridge.KibanaService{Deps: deps}
	w.Http = &bridge.HttpService{Deps: deps}
	w.Grpc = &bridge.GrpcService{Deps: deps}
	w.Collections = &bridge.CollectionsService{Deps: deps, Files: apiFiles}
	w.Variables = &bridge.VariablesService{Deps: deps, Files: apiFiles}
	w.ApiConfig = &bridge.ApiConfigService{Deps: deps, Files: apiFiles, Agent: agentAPI}
	w.ApiFiles = apiFiles
	w.ResponseHistory = &bridge.ResponseHistoryService{Deps: deps}
	w.GrpcHistory = &bridge.GrpcHistoryService{Deps: deps}
	w.DataGrip = &bridge.DataGripService{Deps: deps}
	w.DbMcp = embedded.dbMcpSvc
	w.StudioConfigFiles = &bridge.StudioConfigFilesService{Service: configsvc.New(opts.FileOpener, map[string]string{"runs": filepath.Join(studioHome, "runs"), "api": apiFiles.Dir(), "connections": connFiles.Dir()})}
	w.Docker = embedded.dockerSvc
	w.Kubernetes = embedded.kubeSvc
	w.ProofStore = proofSvc
	w.Proof = &bridge.ProofService{
		Svc: proofSvc, Dialogs: opts.Dialogs, Windows: repositories.Windows, Events: emitter, Targets: proofTargets,
		Shell: shellForProof,
	}
	w.proofShell = shellForProof
	w.studioRuns, w.studioSched, w.studioRunLoader = embedded.runs, embedded.sched, embedded.runLoader
	w.OnWindowClosing = []func(string){func(key string) {
		docker.CloseWindowBound(embedded.dockerSvc.BoundService, key)
		kubernetes.CloseWindowBound(embedded.kubeSvc.BoundService, key)
	}, proofTargets.Forget}

	w.Tabs.Studio = deps
	w.CustomScripts.Studio = &scripts.Service{Loader: embedded.runLoader}
	w.ScriptRuns.Studio = &scriptruns.Bound{Svc: embedded.runs}
	w.launchers.studio = embedded.runs
	w.studioTeardownEarly = func() {
		embedded.eventsDetach()
		adapters.oplogWiring.Stop()
		// DB MCP stops before connectionsSvc.Shutdown(), not after — its stopFn abandons parked
		// approvals then waits out closeHTTP's graceful drain, so no run_query can still be
		// mid-flight, dialing a preconnect target on demand, once Shutdown below starts tearing
		// preconnect down.
		bridge.StopDbMcp(embedded.dbMcpSvc)
		connectionsSvc.Shutdown()
		docker.StopRestoreBound(embedded.dockerSvc.BoundService)
		kubernetes.ServiceOf(embedded.kubeSvc.BoundService).Shutdown()
	}
	w.studioTeardownLate = func() {
		embedded.sched.Close()
		embedded.runLoader.Close()
		apiFiles.Close()
		connFiles.Close()
		embedded.runs.Close()
		if err := repositories.Close(); err != nil {
			slog.Warn("close studio repos", "scope", "shutdown", "err", err)
		}
		if err := db.Close(); err != nil {
			slog.Warn("close studio db", "scope", "shutdown", "err", err)
		}
	}
	return nil
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

// scriptLaunchers routes the embedded terminal's script-launch reports to the stack that owns the
// script or run, until the home fold leaves one stack.
type scriptLaunchers struct {
	space     *scriptruns.Service
	studio    *scriptruns.Service
	spaceOwns interface {
		Get(id string) (*scripts.CustomScript, error)
	}
	studioOwns interface {
		Get(id string) (*scripts.CustomScript, error)
	}
}

func (l *scriptLaunchers) byScript(id string) *scriptruns.Service {
	if l.studio == nil {
		return l.space
	}
	if rec, err := l.spaceOwns.Get(id); err == nil && rec != nil {
		return l.space
	}
	if rec, err := l.studioOwns.Get(id); err == nil && rec != nil {
		return l.studio
	}
	return l.space
}

func (l *scriptLaunchers) byRun(id string) *scriptruns.Service {
	if l.studio == nil {
		return l.space
	}
	if _, err := l.space.Get(id); err == nil {
		return l.space
	}
	if _, err := l.studio.Get(id); err == nil {
		return l.studio
	}
	return l.space
}

func (l *scriptLaunchers) Begin(scriptID, terminalID, launchToken string) (terminal.ScriptLaunch, error) {
	return l.byScript(scriptID).Begin(scriptID, terminalID, launchToken)
}

func (l *scriptLaunchers) BeginAgent(token, command, terminalID string) ([]string, error) {
	env, err := l.space.BeginAgent(token, command, terminalID)
	if err != nil && l.studio != nil {
		if env, serr := l.studio.BeginAgent(token, command, terminalID); serr == nil {
			return env, nil
		}
	}
	return env, err
}

func (l *scriptLaunchers) Failed(runID string, err error) { l.byRun(runID).Failed(runID, err) }

func (l *scriptLaunchers) Exited(terminalID string, code int, cause terminal.CloseCause) {
	l.space.Exited(terminalID, code, cause)
	if l.studio != nil {
		l.studio.Exited(terminalID, code, cause)
	}
}

func (l *scriptLaunchers) Output(terminalID string, b []byte) {
	l.space.Output(terminalID, b)
	if l.studio != nil {
		l.studio.Output(terminalID, b)
	}
}
