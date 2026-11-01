// Package appwire is Kira Space's composition root: it builds the 31 bound services, the git router
// and socket, the ADE tracker and board, and their teardown. main and the flow-test harness
// (internal/flowharness) both call Build, so a test cannot wire differently from production. It
// sits above internal/bridge in the layering, like internal/appshell.
package appwire

import (
	"io"
	"io/fs"
	"log/slog"
	"net/netip"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentnotify"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentusage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/buildinfo"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/codeworkspace"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/filehandlers"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitcred"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost/azure"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost/github"
	githostproviders "github.com/kirathecat/kira-studio/apps/kira-space/internal/githost/providers"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker/jira"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/lannet"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mobileterm"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mobileweb"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/openedfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentsandbox"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/appupdate"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/agentsvc"
	"github.com/kirathecat/kira-studio/internal/harness/providers"
	"github.com/kirathecat/kira-studio/internal/keepawake"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/memory/embed"
	"github.com/kirathecat/kira-studio/internal/metrics"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/windowsvc"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Options carries the OS seams main owns; everything else is wired the same way in production and
// in the harness.
type Options struct {
	Repos *repos.Repos
	// Cipher seals saved secret values; nil builds one from the keychain.
	Cipher *secrets.Cipher
	// DB is closed last by Teardown, after Repos.
	DB io.Closer
	// Emitter is the window event sink (a deferred Wails emitter in production).
	Emitter appevent.Emitter
	Browser bridge.Browser
	Dialogs bridge.Dialogs
	// Locator resolves git.gitPath; main passes the platform locator.
	Locator         gitclient.Locator
	KeepAwakeDriver keepawake.Driver
	// FileHandlers is the default-app seam; nil means this build cannot change defaults.
	FileHandlers filehandlers.Registry
	// MobileAssets is the embedded phone build, already rooted at its index.
	MobileAssets fs.FS
	// MobileIsLAN, MobileDetect, MobileFind and MobilePoll are the phone server's network seams;
	// nil/zero in production (lannet defaults).
	MobileIsLAN  func(netip.Addr) bool
	MobileDetect func() (lannet.Network, error)
	MobileFind   func(lannet.Identity) (lannet.Network, error)
	MobilePoll   time.Duration
	// GitHosts are the registered code-host providers; nil means GitHub and Azure Repos with their real CLIs.
	GitHosts *githost.Registry
	// IssueTracker seams the Jira provider's acli locator, runner and clock; the zero value is production's
	// (a flow-test seam). Its ChangeRequests is set by the wiring.
	IssueTracker jira.Options
	// TrackerGrace is how long the ADE tracker waits before it reads a spawned TUI session as
	// stopped; zero means the tracker default.
	TrackerGrace time.Duration
	// RebaseTimeout bounds one ADE rebase run; 0 keeps the default (20 minutes).
	RebaseTimeout time.Duration
	// BranchRequestWait is how long an agent's branch request waits for the user; 0 keeps the default
	// (30 minutes).
	BranchRequestWait time.Duration
	// Sandbox confines headless agents; nil uses the OS enforcer (a flow-test seam).
	Sandbox agentsandbox.Enforcer
	// SmartTimeout replaces every smart script's own timeout (a flow-test seam); 0 keeps each script's.
	SmartTimeout time.Duration
	// ScheduleTimeout replaces every headless scheduled run's own timeout and Clock drives the
	// scheduler and the run times (flow-test seams); production leaves both zero.
	ScheduleTimeout time.Duration
	Clock           scriptruns.Clock
	// EmbedSpec is the semantic-search model; nil means embed.Default (a flow-test seam).
	EmbedSpec *embed.Spec
	// Providers is the agent harness registry; nil means providers.New() (a flow-test seam).
	Providers *harness.Registry
	// ChangeRequestSync tunes the change request index sync; the zero value is production's (a
	// flow-test seam).
	ChangeRequestSync gitsession.CRSyncConfig
}

// ShellHooks completes the window-manager seams once the shell exists. CloseWindow, FocusWindow and
// SetWindowTitle default to the window registry.
type ShellHooks struct {
	OpenWindow    func(rec shell.WindowRecord)
	OpenNewWindow func()
	// OpenNewWindowKeyed opens a new window under key (an external file open).
	OpenNewWindowKeyed func(key string)
	CloseWindow        func(key string) bool
	FocusWindow        func(key string) bool
	SetWindowTitle     func(key, title string)
	// AppFocused reports whether any app window is key; nil defaults to the window registry.
	AppFocused func() bool
	// OpenPopup opens the capture popup window under key; Clipboard reads the system clipboard text;
	// Shortcuts registers the capture hotkey. nil leaves capture inert.
	OpenPopup func(key string)
	Clipboard func() (string, bool)
	Shortcuts shell.GlobalShortcuts
	// Wayland overrides shell.IsWayland (a flow-test seam); nil uses the process environment.
	Wayland func() bool
}

// Wired is the built object graph.
type Wired struct {
	Repos           *repos.Repos
	Deps            appcore.Deps
	Events          *bridge.Events
	Emitter         appevent.Emitter
	Windows         *shell.WindowRegistry
	Prompts         *prompts.Router
	CloseFlush      *shell.CloseFlushCoordinator
	Quitter         *shell.Quitter
	CredentialRelay *gitcred.Relay

	Git           gitWired
	Tracker       *ade.Tracker
	AgentHooks    *harness.Hooks
	AgentNotify   *agentnotify.Notifier
	TermBroker    *mobileterm.Broker
	TermRegistry  *terminal.Registry
	AdeBoard      *ade.TaskBoard
	KeepAwakeCtl  *keepawake.Controller
	Metrics       *metrics.Ticker
	UpdateInstall *appupdate.Installer

	GitCredential   *bridge.GitCredentialService
	GitHost         *bridge.GitHostService
	HostView        *bridge.HostViewService
	Capture         *bridge.CaptureService
	IssueTracker    *bridge.IssueTrackerService
	Link            *bridge.LinkService
	Files           *bridge.FilesService
	Settings        *bridge.SettingsService
	Layout          *bridge.LayoutService
	Tabs            *bridge.TabsService
	CustomScripts   *bridge.CustomScriptsService
	ScriptRuns      *bridge.ScriptRunsService
	runs            *scriptruns.Service
	sched           *scriptruns.Scheduler
	Terminal        *bridge.TerminalService
	AdeTask         *bridge.AdeTaskService
	Ops             *bridge.OpsService
	Lifecycle       *bridge.LifecycleService
	KeepAwake       *bridge.KeepAwakeService
	Mobile          *bridge.MobileAccessService
	Memory          *bridge.MemoryService
	MemoryImport    *bridge.MemoryImportService
	MemoryDistill   *bridge.MemoryDistillService
	WindowsSvc      *bridge.WindowsService
	PromptsSvc      *bridge.PromptsService
	LlmPromptsSvc   *bridge.LlmPromptsService
	Update          *bridge.UpdateService
	CodeWorkspace   *bridge.CodeWorkspaceService
	OpenedFiles     *openedfiles.Service
	OpenedFilesSvc  *bridge.OpenedFilesService
	FileHandlersSvc *bridge.FileHandlersService
	AgentNotifySvc  *bridge.AgentNotifyService
	AgentUsage      *agentusage.Service
	AgentUsageSvc   *bridge.AgentUsageService
	// Agents resolves the active agent provider per call.
	Agents   harness.Selector
	AgentSvc *bridge.AgentService

	reopen          func()
	appFocused      atomic.Pointer[func() bool]
	detachMetrics   func()
	detachOpLog     func()
	detachGitCred   func()
	detachMobile    func()
	beforeFlushOnce func()
	teardownOnce    func()
}

// Build wires every service.
func Build(opts Options) *Wired {
	repositories := opts.Repos
	w := &Wired{Repos: repositories}
	registry := opts.Providers
	if registry == nil {
		registry = providers.New()
	}
	w.Agents = agentSelector(registry, repositories.Settings)
	w.AgentSvc = &bridge.AgentService{Service: &agentsvc.Service{Agents: w.Agents}}
	repositories.CustomScripts.Agents = registry
	if opts.Cipher == nil {
		opts.Cipher = secrets.New()
	}
	repositories.CustomScripts.Secrets = secrets.NewStore(repositories.CustomScripts.DB, opts.Cipher)
	repositories.Settings.Agents = registry

	credentialRelay := gitcred.New()
	w.Windows = shell.NewWindowRegistry()
	w.SetAppFocused(w.Windows.AnyFocused)
	hosts := opts.GitHosts
	if hosts == nil {
		hosts = githostproviders.New(github.Options{}, azure.New(azure.Options{}))
	}
	git := wireGit(repositories, opts.Locator, hosts, opts.ChangeRequestSync)
	git.trackers = wireTrackers(repositories, git.registry.Hosts.Registry, git.opLog, opts.IssueTracker)
	w.Git, w.CredentialRelay = git, credentialRelay

	// The tap feeds every window-wide event to the phone event hub, which drops what is not allowlisted.
	mobileHub := mobileweb.NewHub()
	emitter := appevent.NewTap(opts.Emitter, mobileHub.Publish)
	promptStore := llmprompts.NewStore(filepath.Join(config.KiraSpaceHome(), "llm-prompts.json"))
	deps := appcore.Deps{Repos: repositories, Home: config.KiraSpaceHome(), Events: emitter, GitRegistry: git.registry, Prompts: promptStore}
	events := bridge.NewEvents(emitter)
	w.Emitter, w.Deps, w.Events = emitter, deps, events
	w.Prompts = prompts.New(prompts.Deps{
		Windows: w.Windows, App: "Space", Reopen: w.reopenWindow,
		Emit:   func(l []prompts.Routed) { emitter.Emit(bridge.ChannelPromptsChanged, l) },
		Reveal: func(key, id string) { emitter.EmitTo(key, bridge.ChannelPromptsReveal, map[string]string{"id": id}) },
		Enabled: func() bool {
			s, err := repositories.Settings.GetAll()
			return err != nil || s.Advanced.NotifyPrompts
		},
	})
	w.Windows.OnChange = w.Prompts.Reroute
	credentialRelay.Prompts = w.Prompts
	w.PromptsSvc = &bridge.PromptsService{Bound: &prompts.Bound{R: w.Prompts}}
	w.LlmPromptsSvc = &bridge.LlmPromptsService{Bound: &llmprompts.Bound{
		Store: promptStore, App: llmprompts.Space,
		Changed: func(rev uint64) { emitter.Emit(bridge.ChannelLlmPromptsChanged, map[string]uint64{"revision": rev}) },
	}}
	w.detachOpLog = events.AttachOpLog(git.opLog)

	// P119: the update-availability checker and its detached installer.
	updateChecker := appupdate.NewChecker(appupdate.Space.Name, buildinfo.Version)
	updateInstaller := appupdate.NewInstaller(appupdate.Space, buildinfo.Version)
	w.UpdateInstall = updateInstaller

	w.CodeWorkspace = &bridge.CodeWorkspaceService{
		Deps: deps, Discovery: git.discovery, Runner: git.runner, Registry: codeworkspace.NewRegistry(),
		OnReposChanged: func() { bridge.AdeTaskReposChanged(events) },
	}
	w.OpenedFiles = openedfiles.New(repositories.CodeRepos)
	w.OpenedFilesSvc = &bridge.OpenedFilesService{Files: w.OpenedFiles}
	handlersRegistry := opts.FileHandlers
	if handlersRegistry == nil {
		handlersRegistry = filehandlers.Unsupported{}
	}
	w.FileHandlersSvc = &bridge.FileHandlersService{Handlers: &filehandlers.Service{
		Registry: handlersRegistry, Previous: repositories.FileHandlerPrevious,
	}}
	w.GitCredential = &bridge.GitCredentialService{Deps: deps, Relay: credentialRelay}
	w.detachGitCred = bridge.AttachGitCredentialPush(w.GitCredential)
	w.GitHost = &bridge.GitHostService{Deps: deps, Browser: opts.Browser}
	w.HostView = &bridge.HostViewService{Deps: deps, Windows: repositories.Windows, Targets: &bridge.HostViewTargets{}, Trackers: git.trackers}
	if opts.Clock != nil {
		w.HostView.Now = opts.Clock.Now
	}
	w.Capture = &bridge.CaptureService{Deps: deps, HostView: w.HostView, Windows: repositories.Windows}
	w.Link = &bridge.LinkService{Browser: opts.Browser}
	w.Settings = &bridge.SettingsService{Deps: deps}
	w.Layout = &bridge.LayoutService{Deps: deps}
	w.Tabs = &bridge.TabsService{Deps: deps}
	// terminalRegistry is shared by the terminal service and the ADE tracker (P129 Part 1 §4.3) — one
	// Registry, since Tracker.Reconcile/Send both need the same live-session set and PTYs.
	terminalRegistry := terminal.NewRegistry()
	w.TermRegistry = terminalRegistry
	w.AgentNotify = agentnotify.New(agentnotify.Deps{
		Prefs: notifyPrefs(repositories), Describe: w.describeAgent, TaskTitle: w.taskTitle, Reveal: w.revealNote,
		AgentLabel: func() string { return w.Agents.Active().Label() },
		Alive:      func(key string) bool { return slices.Contains(w.Windows.Keys(), key) },
		AppFocused: func() bool { return (*w.appFocused.Load())() },
	})
	w.AgentNotifySvc = &bridge.AgentNotifyService{N: w.AgentNotify}
	w.AgentUsage = agentusage.New(agentusage.Deps{
		Enabled: usageEnabled(repositories), Path: filepath.Join(config.KiraSpaceHome(), "agent-usage.json"),
		OnChange: func() { bridge.AgentUsageChanged(emitter, w.AgentUsage.Get()) },
	})
	w.AgentUsageSvc = &bridge.AgentUsageService{U: w.AgentUsage}
	adeTracker, agentHooks := wireTracker(repositories, terminalRegistry, emitter, events, opts.TrackerGrace, w.AgentNotify, w.AgentUsage, w.Agents)
	w.Tracker, w.AgentHooks = adeTracker, agentHooks

	// P128 §2.1: the bound terminal methods live once in internal/terminal.BoundService; this
	// app's own TerminalService only embeds it. ComposeAgent (P129 Part 1 §4.1) rewrites every
	// agent launch through adeTracker.Compose.
	termBroker := wireTermBroker(adeTracker, terminalRegistry, emitter)
	w.TermBroker = termBroker
	runs := &scriptruns.Service{
		Runs: repositories.ScriptRuns, Scripts: repositories.CustomScripts, Registry: terminalRegistry, Home: deps.Home, App: "Space",
		Emit: func(r scriptruns.Run) {
			emitter.Emit(bridge.ChannelScriptRunsChanged, r)
			w.AgentNotify.HandleScriptRun(r)
		},
		EmitLog:      func(p scriptruns.LogPush) { emitter.Emit(bridge.ChannelScriptRunLog, p) },
		SmartTimeout: opts.SmartTimeout, ScheduleTimeout: opts.ScheduleTimeout, Prompts: w.Prompts, LlmPrompts: promptStore,
		Agent: w.Agents,
	}
	if opts.Clock != nil {
		runs.Now = opts.Clock.Now
	}
	runs.Sandbox = opts.Sandbox
	if runs.Sandbox == nil {
		runs.Sandbox = agentsandbox.System()
	}
	w.runs = runs
	if err := runs.Recover(); err != nil {
		slog.Warn("recover script runs", "scope", "startup", "err", err)
	}
	w.ScriptRuns = &bridge.ScriptRunsService{Bound: &scriptruns.Bound{Svc: runs}}
	w.Terminal = &bridge.TerminalService{BoundService: &terminal.BoundService{
		Emit: emitter, Registry: terminalRegistry, ComposeAgent: adeTracker.Compose, AbortAgent: adeTracker.Abort,
		Arbiter: termBroker, Scripts: runs, Agent: w.Agents,
	}}
	// P116 G5 keep-awake toggle plus P188's agent reason: the live Claude Code session count —
	// terminal agent tabs and running headless ade sessions — against agent.keepAwakeWithAgents.
	w.KeepAwakeCtl = keepawake.New(opts.KeepAwakeDriver)
	w.KeepAwake = &bridge.KeepAwakeService{
		Emit: emitter, Toggle: &keepawake.Toggle{Ctl: w.KeepAwakeCtl},
		AgentCount: agentSessionCount(repositories, terminalRegistry),
		Settings:   repositories.Settings.GetAll,
	}
	w.Settings.OnChanged = func(m model.Settings) {
		bridge.KeepAwakeRecompute(w.KeepAwake)
		bridge.CaptureApply(w.Capture, m.Capture)
	}
	// memory.db opens on the Memory module's first call.
	embedSpec := embed.Default
	if opts.EmbedSpec != nil {
		embedSpec = *opts.EmbedSpec
	}
	w.Memory = bridge.NewMemoryService(emitter, harness.ActiveInstaller(w.Agents), promptStore, embedSpec, w.Agents, memoryModelFrom(repositories.Settings))
	removeLegacyGitSocket()
	go removeRetiredModels()

	// Archive closes a task's review windows through the registry.
	w.AdeBoard = wireAdeTask(repositories, events, git, adeTracker, adeCloseTerminal(terminalRegistry),
		closeTaskReviewWindows(repositories, func(key string) bool {
			// Through the bound hook, so a shell that overrides CloseWindow sees archive closes too.
			if w.AdeTask != nil && w.AdeTask.CloseWindow != nil {
				return w.AdeTask.CloseWindow(key)
			}
			return w.Windows.Close(key)
		}), credentialRelay, w.KeepAwake, w.AgentNotify, w.AgentUsage, opts.RebaseTimeout, opts.BranchRequestWait, runs, w.Agents, w.Prompts)
	runs.ADE = w.AdeBoard
	w.sched = &scriptruns.Scheduler{Svc: runs, Clock: opts.Clock}
	w.sched.Start()
	w.IssueTracker = &bridge.IssueTrackerService{Engine: w.AdeBoard, Hosts: git.registry.Hosts.Registry, Providers: git.trackers}
	w.AdeTask = &bridge.AdeTaskService{Engine: w.AdeBoard, Registry: terminalRegistry, Emit: emitter, Hosts: git.registry.Hosts.Registry}
	w.Capture.Tasks = w.AdeTask
	// Registry.OnChange fires after every agent session registers or is removed (spawn and exit) —
	// Reconcile picks up both, and AgentSessionsChanged refreshes the P127 store's own live count
	// (P129 Part 1 §4.2 step 4).
	terminalRegistry.OnChange = func() {
		adeTracker.Reconcile()
		bridge.AgentSessionsChanged(emitter, terminalRegistry)
		bridge.KeepAwakeRecompute(w.KeepAwake)
	}
	bridge.KeepAwakeRecompute(w.KeepAwake)

	// OpenNewWindow is assigned by BindShell once the shell exists.
	w.WindowsSvc = &bridge.WindowsService{Service: &windowsvc.Service{Windows: deps.Repos.Windows}}

	// P116 G7: the status bar's CPU/memory readout.
	w.Metrics = metrics.NewAppTicker("Kira Space")
	w.Metrics.Start()
	w.detachMetrics = events.AttachMetrics(w.Metrics)

	w.AdeTask.FocusWindow = w.Windows.Focus
	w.CloseFlush = shell.NewCloseFlushCoordinator(events)

	mobileLaunches := &bridge.MobileLaunches{Emit: emitter, Window: w.Windows.MainKey}
	w.Mobile = bridge.NewMobileAccessService(&bridge.MobileAccessService{
		Deps: deps, Reader: w.AdeTask, Hub: mobileHub, Broker: mobileweb.NewBroker(time.Now, w.Prompts), Assets: opts.MobileAssets,
		AgentSessions: func() any { return w.Terminal.AgentSessions() },
		AgentInfo: func() any {
			info, _ := w.AgentSvc.Info()
			return info
		},
		Writer:    &bridge.MobileWriter{Svc: w.AdeTask, Launches: mobileLaunches},
		Launches:  mobileLaunches,
		Terminals: termBroker,
		Detect:    opts.MobileDetect, Find: opts.MobileFind, Poll: opts.MobilePoll,
		IsLAN: opts.MobileIsLAN,
	})
	w.detachMobile = bridge.AttachMobilePush(w.Mobile)

	w.beforeFlushOnce = sync.OnceFunc(func() {
		// The ticker stops before the flush wait rather than after it (P56 D3).
		w.Metrics.Stop()
		w.Windows.DetachAll()
	})
	w.teardownOnce = sync.OnceFunc(func() { w.teardown(opts.DB) })
	w.Quitter = shell.NewQuitter(events, w.beforeFlushOnce, w.teardownOnce, 2*time.Second, w.Windows.Keys)

	w.Files = &bridge.FilesService{Dialogs: opts.Dialogs}
	w.CustomScripts = &bridge.CustomScriptsService{Deps: deps, Changed: w.sched.Reload}
	w.Ops = &bridge.OpsService{Log: git.opLog}
	live := liveWork(repositories, terminalRegistry, runs)
	w.Quitter.SetGate(prompts.QuitGate{R: w.Prompts, LiveFn: func() int { return live().Total() }})
	w.Lifecycle = &bridge.LifecycleService{Flusher: w.Quitter, WindowFlusher: w.CloseFlush, Quit: w.Quitter, Live: live}
	w.MemoryImport = bridge.NewMemoryImportService(w.Memory, opts.Dialogs)
	w.MemoryDistill = bridge.NewMemoryDistillService(w.Memory, opts.Dialogs, emitter)
	w.Update = &bridge.UpdateService{Checker: updateChecker, Installer: updateInstaller, Quit: w.Quitter.RequestQuit}
	return w
}

// BindShell points the window-manager seams at the shell, which exists only after Build.
func (w *Wired) BindShell(h ShellHooks) {
	closeWindow, focusWindow, setTitle := h.CloseWindow, h.FocusWindow, h.SetWindowTitle
	if closeWindow == nil {
		closeWindow = w.Windows.Close
	}
	if focusWindow == nil {
		focusWindow = w.Windows.Focus
	}
	if setTitle == nil {
		setTitle = func(key, title string) { w.Windows.SetTitle(key, title) }
	}
	if h.AppFocused != nil {
		w.SetAppFocused(h.AppFocused)
	}
	w.AdeTask.OpenWindow = h.OpenWindow
	w.AdeTask.CloseWindow = closeWindow
	w.AdeTask.FocusWindow = focusWindow
	w.AdeTask.SetWindowTitle = setTitle
	w.HostView.OpenWindow = h.OpenWindow
	w.HostView.FocusWindow = focusWindow
	w.HostView.SetWindowTitle = setTitle
	w.WindowsSvc.OpenNewWindow = h.OpenNewWindow
	w.OpenedFiles.OpenWindow = h.OpenNewWindowKeyed
	w.bindCapture(h, closeWindow, focusWindow)
}

// bindCapture points the capture service at the shell and registers the hotkey from stored settings.
func (w *Wired) bindCapture(h ShellHooks, closeWindow, focusWindow func(string) bool) {
	if h.Shortcuts == nil {
		return
	}
	w.Capture.OpenPopup = h.OpenPopup
	w.Capture.Clipboard = h.Clipboard
	w.Capture.FocusWindow = focusWindow
	w.Capture.CloseWindow = closeWindow
	w.Capture.Shortcuts = h.Shortcuts
	w.Capture.Binding = shell.NewHotkeyBinding(h.Shortcuts, h.Wayland)
	s, err := w.Repos.Settings.GetAll()
	if err != nil {
		slog.Warn("capture: read settings", "scope", "capture", "err", err)
		return
	}
	bridge.CaptureApply(w.Capture, s.Capture)
}

// SetAppFocused swaps the any-window-focused probe; atomic because hook events read it concurrently.
func (w *Wired) SetAppFocused(fn func() bool) { w.appFocused.Store(&fn) }

// SetReopen sets what a notification click runs when no window is open (main owns the opener deps).
func (w *Wired) SetReopen(fn func()) { w.reopen = fn }

func (w *Wired) reopenWindow() {
	if w.reopen != nil {
		w.reopen()
	}
}

// StartMobile starts the phone server when settings enable it; call after the shell is up.
func (w *Wired) StartMobile() { bridge.StartMobileIfEnabled(w.Mobile) }

// Bound returns the 31 bound services in registration order.
func (w *Wired) Bound() []application.Service {
	return []application.Service{
		application.NewService(w.GitCredential),
		application.NewService(w.CodeWorkspace), application.NewService(w.GitHost), application.NewService(w.HostView), application.NewService(w.IssueTracker), application.NewService(w.Link),
		application.NewService(w.Files), application.NewService(w.Settings), application.NewService(w.Layout),
		application.NewService(w.Tabs), application.NewService(w.CustomScripts), application.NewService(w.ScriptRuns), application.NewService(w.Terminal),
		application.NewService(w.AdeTask), application.NewService(w.Ops), application.NewService(w.Lifecycle),
		application.NewService(w.KeepAwake), application.NewService(w.Mobile), application.NewService(w.Memory),
		application.NewService(w.MemoryImport), application.NewService(w.MemoryDistill), application.NewService(w.WindowsSvc), application.NewService(w.Update),
		application.NewService(w.AgentNotifySvc), application.NewService(w.AgentUsageSvc),
		application.NewService(w.PromptsSvc), application.NewService(w.LlmPromptsSvc), application.NewService(w.AgentSvc), application.NewService(w.OpenedFilesSvc),
		application.NewService(w.FileHandlersSvc), application.NewService(w.Capture),
	}
}

// GitRouter is the router the git stream serves.
func (w *Wired) GitRouter() *gitrpc.Router { return w.Git.router }

// BeforeFlush stops the metrics ticker and detaches window listeners; idempotent.
func (w *Wired) BeforeFlush() { w.beforeFlushOnce() }

// Teardown runs the ordered shutdown once; idempotent.
func (w *Wired) Teardown() { w.teardownOnce() }

func (w *Wired) teardown(db io.Closer) {
	// P119: a Cmd+Q mid-download aborts the install rather than leaving an orphan that later
	// swaps a bundle the user quit away from. After hand-off this is a no-op.
	w.UpdateInstall.Cancel()
	w.OpenedFiles.Close()
	w.detachMetrics()
	w.detachOpLog()
	// P87 §4: killing the assertion early keeps the window between "app is quitting" and
	// "caffeinate is dead" as short as possible.
	w.KeepAwakeCtl.Close()
	// terminal.ShutdownBound first: every PTY dies, and each one's own exit fires Registry.OnChange
	// (Reconcile marks its row stopped) while the DB is still open. Then shutdownTracker flushes
	// whatever last-active time is still only in memory and stops the hooks listener.
	terminal.ShutdownBound(w.Terminal.BoundService)
	w.sched.Close()
	w.runs.Close()
	shutdownTracker(w.Tracker, w.AgentHooks)
	w.AdeBoard.Close()
	w.detachGitCred()
	// Before DB close: ends open streams and aborts parked pairing requests.
	bridge.StopMobile(w.Mobile)
	w.detachMobile()
	// Ends entries, watchers, auto-fetch timers and review.db.
	w.Git.registry.Close()
	w.Git.opLog.Close()
	if w.Git.askpassBroker != nil {
		if err := w.Git.askpassBroker.Close(); err != nil {
			slog.Warn("close askpass broker", "scope", "shutdown", "err", err)
		}
	}
	// F5: stops every open codeworkspace.Session (cat-file pairs, in-flight searches) — before
	// repositories.Close(), since a running search still reads settings through Deps.Repos.
	w.CodeWorkspace.Shutdown()
	bridge.CloseMemory(w.Memory)
	if err := w.Repos.Close(); err != nil {
		slog.Warn("close repos", "scope", "shutdown", "err", err)
	}
	if err := db.Close(); err != nil {
		slog.Warn("close db", "scope", "shutdown", "err", err)
	}
}
