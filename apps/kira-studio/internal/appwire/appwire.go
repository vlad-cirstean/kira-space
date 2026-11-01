// Package appwire is Kira Studio's composition root: it builds the 33 bound services, the adapter
// router, the embedded modules and their teardown. main and the flow-test harness
// (internal/flowharness) both call Build, so a test cannot wire differently from production. It
// sits above internal/bridge in the layering, like internal/appshell.
package appwire

import (
	"path/filepath"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/kubernetes"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/localauth"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/maskrules"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/proof"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/appupdate"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/agentsvc"
	"github.com/kirathecat/kira-studio/internal/harness/providers"
	"github.com/kirathecat/kira-studio/internal/keepawake"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/logging"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Options carries the OS seams main owns; everything else is wired the same way in production and
// in the harness.
type Options struct {
	// DB is closed last by Teardown, after Repos.
	DB    *storage.DB
	Repos *repos.Repos
	// Cipher and Authorizer are shared by connections and variables, so a reveal grace is shared.
	Cipher     *secrets.Cipher
	Authorizer *localauth.Authorizer
	// Emitter is the window event sink (a deferred Wails emitter in production).
	Emitter         appevent.Emitter
	Dialogs         bridge.Dialogs
	KeepAwakeDriver keepawake.Driver
	McpInstaller    harness.MCPInstaller
	// Browser opens external URLs (a deferred Wails opener in production).
	Browser bridge.Browser
	AppName string
	Version string
	// SmartTimeout replaces every smart script's own timeout (a flow-test seam); production leaves it zero.
	SmartTimeout time.Duration
	// ScheduleTimeout replaces every headless scheduled run's own timeout and Clock drives the
	// scheduler and the run times (flow-test seams); production leaves both zero.
	ScheduleTimeout time.Duration
	Clock           scriptruns.Clock
	// DockerHubAPI replaces the Docker Hub base URL the image registry check calls (flow-test seam); empty keeps the default.
	DockerHubAPI string
	// DockerExtra appends docker options (flow-test seams for the restore tracker).
	DockerExtra []docker.Option
	// KubernetesExtra appends kubernetes options (flow-test seams: environment, list limits).
	KubernetesExtra []kubernetes.Option
	// Providers is the agent harness registry; nil means providers.New() (a flow-test seam).
	Providers *harness.Registry
	// WindowFocus replaces the registry's focus state and raise (a flow-test seam); nil keeps the registry.
	WindowFocus WindowFocus
}

// WindowFocus is the part of the window registry a prompt raise reads and drives.
type WindowFocus interface {
	Focused(key string) bool
	Focus(key string) bool
}

// Wired is the built object graph.
type Wired struct {
	StartedAt  time.Time
	Repos      *repos.Repos
	Deps       appcore.Deps
	Router     *adapterhost.Router
	Events     *bridge.Events
	Windows    *shell.WindowRegistry
	Prompts    *prompts.Router
	CloseFlush *shell.CloseFlushCoordinator
	Quitter    *shell.Quitter
	// TerminalRegistry and OnWindowClosing are the window-close hooks the shell runs.
	TerminalRegistry *terminal.Registry
	OnWindowClosing  []func(string)

	App             *bridge.AppService
	Settings        *bridge.SettingsService
	Layout          *bridge.LayoutService
	Tabs            *bridge.TabsService
	WindowsSvc      *bridge.WindowsService
	Connections     *bridge.ConnectionsService
	MaskRules       *bridge.MaskRulesService
	Tree            *bridge.TreeService
	Ops             *bridge.OpsService
	Filters         *bridge.FiltersService
	Files           *bridge.FilesService
	Queries         *bridge.QueriesService
	Schema          *bridge.SchemaService
	Kibana          *bridge.KibanaService
	Http            *bridge.HttpService
	Grpc            *bridge.GrpcService
	Collections     *bridge.CollectionsService
	Variables       *bridge.VariablesService
	ResponseHistory *bridge.ResponseHistoryService
	GrpcHistory     *bridge.GrpcHistoryService
	DataGrip        *bridge.DataGripService
	DbMcp           *bridge.DbMcpService
	KeepAwake       *bridge.KeepAwakeService
	Terminal        *bridge.TerminalService
	CustomScripts   *bridge.CustomScriptsService
	ScriptRuns      *bridge.ScriptRunsService
	Docker          *bridge.DockerService
	Kubernetes      *bridge.KubernetesService
	Link            *bridge.LinkService
	Update          *bridge.UpdateService
	Lifecycle       *bridge.LifecycleService
	PromptsSvc      *bridge.PromptsService
	LlmPromptsSvc   *bridge.LlmPromptsService
	Proof           *bridge.ProofService
	// ProofStore is the validation-proof store behind Proof and the DB MCP proof tools.
	ProofStore *proof.Service
	// Agents resolves the active agent provider per call.
	Agents   harness.Selector
	AgentSvc *bridge.AgentService

	proofShell      *registryShell
	reopen          func()
	beforeFlushOnce func()
	teardownOnce    func()
}

// Build wires every service. A settings read failure is the only error; main reports it fatal.
func Build(opts Options) (*Wired, error) {
	// P103 Part 3: repo-root internal/terminal's two process-constant vars, set once before any
	// Registry.Open: a spawned shell's TERM_PROGRAM/TERM_PROGRAM_VERSION env.
	terminal.TermProgram = opts.AppName
	terminal.TermProgramVersion = opts.Version

	startedAt := time.Now()
	repositories, db := opts.Repos, opts.DB
	registry := opts.Providers
	if registry == nil {
		registry = providers.New()
	}
	repositories.CustomScripts.Agents = registry
	agents := harness.Selector{Registry: registry, ID: func() (string, error) {
		s, err := repositories.Settings.GetAll()
		return s.Agent.Provider, err
	}}

	secretsRepo := repos.NewSecrets(db.DB, opts.Cipher)
	// P5: the same "needs a Cipher, constructed separately from repos.New's aggregate" shape as
	// secretsRepo just above.
	repositories.Variables = repos.NewVariables(db.DB, opts.Cipher)
	// M5 §2.5/§3.2: the per-connection correlation key column, same "needs a Cipher" shape.
	maskKeysRepo := repos.NewMaskKeys(db.DB, opts.Cipher)
	maskRulesSvc := maskrules.New(repositories.MaskRules, maskKeysRepo)

	// P5 D8: the SAME authorizer instance connections.New below is given: that is what makes the
	// reveal grace genuinely shared between a connection-password reveal and a variable reveal.
	promptStore := llmprompts.NewStore(filepath.Join(config.KiraHome(), "llm-prompts.json"))
	deps := appcore.Deps{
		DB:        db.DB,
		Home:      config.KiraHome(),
		StartedAt: startedAt.UnixMilli(),
		Repos:     repositories,
		ApiVars:   apivars.New(repositories.Variables, opts.Authorizer),
		MaskRules: maskRulesSvc,
		Prompts:   promptStore,
	}

	// Read from the just-migrated (possibly still-default) settings row, same as production would
	// before any user override exists: the cache budget below needs it.
	settings, err := deps.Repos.Settings.GetAll()
	if err != nil {
		return nil, err
	}
	// P72 §9.2: match the stored advanced.logLevel rather than always booting at Info.
	logging.SetLevel(settings.Advanced.LogLevel)

	adaptersW := wireAdapters(&deps, settings, repositories, secretsRepo, opts.Cipher, opts.Authorizer, db)
	router, connectionsSvc := adaptersW.router, adaptersW.connectionsSvc
	oplogWiring, metricsTicker := adaptersW.oplogWiring, adaptersW.metricsTicker

	deps.Events = opts.Emitter

	// P66: the update-availability checker: no network call at all from a dev/test build
	// (appupdate's own isReleaseBuild guard); owns no goroutine, no ticker, no file handle, so
	// nothing is added to the quit teardown below.
	updateChecker := appupdate.NewChecker(appupdate.Studio.Name, opts.Version)
	// P119: the detached installer. It owns a child process only while staging: teardown below
	// cancels it so a Cmd+Q mid-download aborts the install rather than orphaning a bundle swap.
	updateInstaller := appupdate.NewInstaller(appupdate.Studio, opts.Version)

	w := &Wired{}
	windows := shell.NewWindowRegistry()
	shellForProof := &registryShell{windows: windows}
	promptRouter := newPromptRouter(windows, repositories, opts, w.reopenWindow)
	windows.OnChange = promptRouter.Reroute

	proofDeps := proof.Deps{
		Repo: repositories.Proof, Prompts: promptRouter,
		Emit: func(channel string, payload any) { opts.Emitter.Emit(channel, payload) },
	}
	if opts.Clock != nil {
		proofDeps.Now = opts.Clock.Now
	}
	proofSvc := proof.New(proofDeps)
	proofSvc.PurgeStale()
	proofSvc.RaiseUnreviewed()
	proofTargets := &bridge.ProofWindowTargets{}

	installer := opts.McpInstaller
	if installer == nil {
		installer = harness.ActiveInstaller(agents)
	}
	embedded := wireEmbeddedServices(deps, installer, opts.KeepAwakeDriver, connectionsSvc, oplogWiring, metricsTicker, promptRouter, agents, proofSvc, opts)
	lifecycle := wireLifecycle(windows, embedded.events, embedded.eventsDetach, metricsTicker, oplogWiring, connectionsSvc,
		embedded.dbMcpSvc, embedded.keepAwakeSvc, embedded.terminalSvc, embedded.dockerSvc, embedded.kubeSvc, updateInstaller, embedded.runs, embedded.sched, repositories, db)
	embedded.sched.Start()

	live := func() shell.QuitStatus {
		return shell.QuitStatus{Agents: len(embedded.terminalSvc.Registry.AgentSessions()), Scripts: embedded.runs.Live()}
	}
	lifecycle.quitter.SetGate(prompts.QuitGate{R: promptRouter, LiveFn: func() int { return live().Total() }})

	*w = Wired{
		Prompts:    promptRouter,
		PromptsSvc: &bridge.PromptsService{Bound: &prompts.Bound{R: promptRouter}},
		LlmPromptsSvc: &bridge.LlmPromptsService{Bound: &llmprompts.Bound{
			Store: promptStore, App: llmprompts.Studio,
			Changed: func(rev uint64) {
				opts.Emitter.Emit(bridge.ChannelLlmPromptsChanged, map[string]uint64{"revision": rev})
			},
		}},

		StartedAt: startedAt, Repos: repositories, Deps: deps, Router: router, Events: embedded.events,
		Windows: lifecycle.windows, CloseFlush: lifecycle.closeFlush, Quitter: lifecycle.quitter,
		TerminalRegistry: embedded.terminalSvc.Registry,
		OnWindowClosing: []func(string){func(key string) {
			docker.CloseWindowBound(embedded.dockerSvc.BoundService, key)
			kubernetes.CloseWindowBound(embedded.kubeSvc.BoundService, key)
		}, proofTargets.Forget},

		App:             &bridge.AppService{Deps: deps},
		Settings:        &bridge.SettingsService{Deps: deps},
		Layout:          &bridge.LayoutService{Deps: deps},
		Tabs:            &bridge.TabsService{Deps: deps},
		WindowsSvc:      embedded.windowsSvc,
		Connections:     &bridge.ConnectionsService{Deps: deps},
		MaskRules:       &bridge.MaskRulesService{Deps: deps},
		Tree:            &bridge.TreeService{Deps: deps},
		Ops:             &bridge.OpsService{Deps: deps, Canceller: router},
		Filters:         &bridge.FiltersService{Deps: deps},
		Files:           &bridge.FilesService{Dialogs: opts.Dialogs},
		Queries:         &bridge.QueriesService{Deps: deps},
		Schema:          &bridge.SchemaService{Deps: deps},
		Kibana:          &bridge.KibanaService{Deps: deps},
		Http:            &bridge.HttpService{Deps: deps},
		Grpc:            &bridge.GrpcService{Deps: deps},
		Collections:     &bridge.CollectionsService{Deps: deps},
		Variables:       &bridge.VariablesService{Deps: deps},
		ResponseHistory: &bridge.ResponseHistoryService{Deps: deps},
		GrpcHistory:     &bridge.GrpcHistoryService{Deps: deps},
		DataGrip:        &bridge.DataGripService{Deps: deps},
		DbMcp:           embedded.dbMcpSvc,
		KeepAwake:       embedded.keepAwakeSvc,
		Terminal:        embedded.terminalSvc,
		CustomScripts:   &bridge.CustomScriptsService{Deps: deps, Changed: embedded.sched.Reload},
		ScriptRuns:      embedded.scriptRuns,
		Docker:          embedded.dockerSvc,
		Kubernetes:      embedded.kubeSvc,
		Link:            &bridge.LinkService{Browser: opts.Browser},
		Update: &bridge.UpdateService{
			Checker: updateChecker, Installer: updateInstaller, Quit: lifecycle.quitter.RequestQuit,
		},
		Lifecycle: &bridge.LifecycleService{Flusher: lifecycle.quitter, WindowFlusher: lifecycle.closeFlush, Quit: lifecycle.quitter, Live: live},

		ProofStore: proofSvc,
		Proof: &bridge.ProofService{
			Svc: proofSvc, Dialogs: opts.Dialogs, Windows: repositories.Windows, Events: opts.Emitter, Targets: proofTargets,
			Shell: shellForProof,
		},

		Agents:   agents,
		AgentSvc: &bridge.AgentService{Service: &agentsvc.Service{Agents: agents}},

		proofShell:      shellForProof,
		beforeFlushOnce: lifecycle.beforeFlush,
		teardownOnce:    lifecycle.teardown,
	}
	return w, nil
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

// SetReopen sets what a notification click runs when no window is open (main owns the opener deps).
func (w *Wired) SetReopen(fn func()) { w.reopen = fn }

func (w *Wired) reopenWindow() {
	if w.reopen != nil {
		w.reopen()
	}
}

// Bound returns the 35 bound services in registration order.
func (w *Wired) Bound() []application.Service {
	return []application.Service{
		application.NewService(w.App), application.NewService(w.Settings), application.NewService(w.Layout),
		application.NewService(w.Tabs), application.NewService(w.WindowsSvc), application.NewService(w.Connections),
		application.NewService(w.MaskRules), application.NewService(w.Tree), application.NewService(w.Ops),
		application.NewService(w.Filters), application.NewService(w.Files), application.NewService(w.Queries),
		application.NewService(w.Schema), application.NewService(w.Kibana), application.NewService(w.Http), application.NewService(w.Grpc),
		application.NewService(w.Collections), application.NewService(w.Variables),
		application.NewService(w.ResponseHistory), application.NewService(w.GrpcHistory),
		application.NewService(w.DataGrip), application.NewService(w.DbMcp), application.NewService(w.KeepAwake),
		application.NewService(w.Terminal), application.NewService(w.CustomScripts), application.NewService(w.ScriptRuns), application.NewService(w.Docker), application.NewService(w.Kubernetes), application.NewService(w.Link),
		application.NewService(w.Update), application.NewService(w.Lifecycle), application.NewService(w.PromptsSvc), application.NewService(w.LlmPromptsSvc), application.NewService(w.AgentSvc), application.NewService(w.Proof),
	}
}

// BeforeFlush stops the metrics ticker and detaches window listeners; idempotent.
func (w *Wired) BeforeFlush() { w.beforeFlushOnce() }

// Teardown runs the ordered shutdown once; idempotent.
func (w *Wired) Teardown() { w.teardownOnce() }
