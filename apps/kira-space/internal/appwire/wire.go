package appwire

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adeflow"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentnotify"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentusage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/buildinfo"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitaskpass"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitcred"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker/jira"
	trackerproviders "github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker/providers"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mobileterm"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentctx"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/memory"
	memembed "github.com/kirathecat/kira-studio/internal/memory/embed"
	"github.com/kirathecat/kira-studio/internal/memory/memcheck"
	"github.com/kirathecat/kira-studio/internal/memory/memorycli"
	"github.com/kirathecat/kira-studio/internal/memory/modelstore"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

// RunArgvShim runs the askpass, memory-mcp, memory-ingest, memory-embed and memory-check
// subcommands, before anything Wails-related, so they never start a window. memory-mcp is Claude
// Code's stdio MCP server; it runs before startupfail and the single-instance lock, so it works
// while the window is open.
// memory-embed is the ONNX embedding worker those processes and the app spawn; same ordering for
// the same reason. memory-check is the kira-memory doc validator, run as a git hook.
// memory-ingest is the detached bulk-import runner (one per memory.db) that the app and the MCP
// server spawn.
func RunArgvShim(args []string) (code int, ok bool) {
	if len(args) < 2 {
		return 0, false
	}
	switch args[1] {
	case "askpass":
		return gitaskpass.RunHelper(args[2:], os.Environ(), os.Stdout), true
	case "memory-mcp":
		return memorycli.Run(args[2:], promptsPath(config.KiraSpaceHome()), agentSelectorAt(config.DbPath()), memoryModelAt(config.DbPath()), memoryOps(config.DbPath(), opOwnersDir())), true
	case "memory-ingest":
		return memorycli.RunIngest(args[2:], promptsPath(config.KiraSpaceHome()), agentSelectorAt(config.DbPath()), memoryModelAt(config.DbPath())), true
	case "memory-embed":
		return memembed.RunWorker(args[2:]), true
	case "memory-check":
		return memcheck.Run(args[2:], os.Stdout, os.Stderr), true
	}
	return 0, false
}

// removeRetiredModels deletes model directories no feature loads any more (P224).
func removeRetiredModels() {
	if err := modelstore.RemoveRetired(memory.Home()); err != nil {
		slog.Warn("remove retired models", "scope", "memory", "err", err)
	}
}

// removeLegacyGitSocket deletes the git.sock and git.sock.lock a pre-P243 build left in the home.
// Safe in Build: the single-instance lock means no other Space owns this home.
func removeLegacyGitSocket() {
	for _, name := range []string{"git.sock", "git.sock.lock"} {
		if err := os.Remove(filepath.Join(config.KiraSpaceHome(), name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("remove legacy git socket", "scope", "startup", "file", name, "err", err)
		}
	}
}

// gitWired is wireGit's own result.
type gitWired struct {
	runner        gitclient.Runner
	discovery     *gitclient.Discovery
	registry      *gitsession.Registry
	askpassBroker *gitaskpass.Broker
	router        *gitrpc.Router
	opLog         *oplog.Log
	trackers      *issuetracker.Registry
}

// wireTrackers builds the issue tracker registry.
func wireTrackers(opLog *oplog.Log, o jira.Options) *issuetracker.Registry {
	if o.Runner == nil {
		o.Runner = jira.NewExecRunner()
	}
	o.Runner = oplog.NewRecordingToolRunner(o.Runner, opLog, jira.ProviderLabel)
	return trackerproviders.New(o)
}

// wireTracker constructs and starts the ade.Tracker and the harness.Hooks hub it drives, wired
// together before any window exists so Compose never races an unset hooks func with a real launch.
// Rows a previous process life left running are stopped by the task board's Recover.
func wireTracker(
	repositories *repos.Repos, registry *terminal.Registry, emitter appevent.Emitter, events *bridge.Events,
	grace time.Duration, notifier *agentnotify.Notifier, usage *agentusage.Service, agents harness.Selector,
	contextFile func() (string, error),
) (*ade.Tracker, *harness.Hooks) {
	tracker := ade.NewTracker(ade.TrackerDeps{
		Agent: agents, ContextFile: contextFile, Store: repositories.AdeSessions, LiveAgents: registry.AgentSessions,
		WriteTerminal: registry.Write, Now: time.Now, Grace: grace,
		OnChange: func() { bridge.AdeTaskSessionsChanged(events) },
	})

	// Hooks are always on: a start failure (curl missing, a bind conflict) is logged, never fatal —
	// sessions still spawn and track, activity icons stay absent.
	hooks := harness.NewHooks(agents.Registry, harness.HookOptions{
		OnEvent: func(ev harness.AgentEvent) {
			tracker.HandleEvent(ev)
			bridge.EmitAgentEvent(emitter, ev)
			notifier.HandleEvent(ev)
		},
		StatusLine: usageEnabled(repositories),
		OnStatusLine: func(_ string, rl harness.RateLimits) {
			var five, seven *agentusage.Window
			if rl.FiveHour != nil {
				five = agentusage.FromSession(rl.FiveHour.UsedPercent, rl.FiveHour.ResetsAt)
			}
			if rl.SevenDay != nil {
				seven = agentusage.FromSession(rl.SevenDay.UsedPercent, rl.SevenDay.ResetsAt)
			}
			usage.Ingest(agentusage.SourceSession, five, seven)
		},
	})
	if err := hooks.Start(); err != nil {
		slog.Warn("agent hooks: start", "scope", "ade", "err", err)
	}
	tracker.SetHooks(hooks.Compose)
	return tracker, hooks
}

// agentSessionCount is the live Claude Code session count the keep-awake agent reason reads: the
// terminal registry's agent tabs plus ade headless sessions still running (headless runs have no
// terminal). A read failure counts the terminal sessions only.
func agentSessionCount(repositories *repos.Repos, registry *terminal.Registry) func() int {
	return func() int {
		n := len(registry.AgentSessions())
		headless, err := repositories.AdeSessions.CountRunningHeadless()
		if err != nil {
			slog.Warn("keep-awake: count ade sessions", "scope", "keepawake", "err", err)
			return n
		}
		n += headless
		return n
	}
}

// liveWork counts what a quit stops: agent PTYs, running ADE runs, and smart and headless script runs.
func liveWork(repositories *repos.Repos, registry *terminal.Registry, scriptsLive func() int) func() shell.QuitStatus {
	return func() shell.QuitStatus {
		st := shell.QuitStatus{Agents: len(registry.AgentSessions()), Scripts: scriptsLive()}
		n, err := repositories.AdeTasks.CountRunning()
		if err != nil {
			slog.Warn("quit: count ade runs", "scope", "lifecycle", "err", err)
		}
		st.Runs = n
		return st
	}
}

// closeTaskReviewWindows returns the hook that closes every review window of an archived task.
func closeTaskReviewWindows(repositories *repos.Repos, closeWindow func(key string) bool) func(taskID string) {
	return func(taskID string) {
		keys, err := repositories.AdeReview.KeysByTask(taskID)
		if err != nil {
			slog.Warn("ade: list review windows", "scope", "ade", "task", taskID, "err", err)
			return
		}
		for _, k := range keys {
			closeWindow(k)
		}
	}
}

// wireAdeTask builds the v2 task board engine (P144): its own Conn, the workflow reader over
// <home>/config/workflows, and the cached git discovery gating merge-tree checks. It recovers rows a
// restart left running, then starts the board.
func wireAdeTask(
	repositories *repos.Repos, events *bridge.Events, git gitWired, tracker *ade.Tracker, closeTerminal func(string) error,
	closeReviewWindows func(taskID string), credentials *gitcred.Relay, keepAwake *bridge.KeepAwakeService,
	notifier *agentnotify.Notifier, usage *agentusage.Service, rebaseTimeout, branchRequestWait time.Duration, updates ade.UpdatesConfig, runs *scriptruns.Service,
	agents harness.Selector, promptSink prompts.Sink, approvalQueue *approvals.Queue, repoFiles *repocfg.Store,
	proofs ade.ProofLinks, dbMcp ade.DbMcpSessions,
) *ade.TaskBoard {
	userHome, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("ade: user home", "scope", "ade", "err", err)
	}
	gitPath := adeGitPathSetting(repositories)
	board := ade.NewTaskBoard(ade.TaskBoardDeps{
		Agent:  agents,
		Proofs: proofs, DbMcp: dbMcp, DbAudit: repositories.DbAudit,
		ContextFile: runs.ContextFile,
		AgentConfig: runs.AgentConfig, Servers: runs.Servers,
		Ops:     git.opLog,
		Prompts: runs.LlmPrompts, PromptSink: promptSink, Approvals: approvalQueue,
		Tasks: repositories.AdeTasks, Statuses: repositories.AdeStatuses, RepoConfig: repositories.AdeRepoConfig,
		CodeRepos: repositories.CodeRepos, GitRepoSettings: repoFiles.Settings, RepoFile: repoFile(repoFiles),
		Registry:  git.registry,
		Trackers:  git.trackers,
		GitStatus: func(ctx context.Context) gitclient.GitStatus { return git.discovery.Status(ctx, gitPath()) },
		Askpass:   git.askpassBroker,
		Workflows: &adeflow.Reader{Dir: configfs.ModuleDir(config.KiraSpaceHome(), adeflow.WorkflowsFolder), Store: configfs.TableProjection{DB: repositories.CustomScripts.DB, Folder: adeflow.WorkflowsFolder},
			Steps: &adeflow.StepLibrary{Dir: configfs.ModuleDir(config.KiraSpaceHome(), adeflow.StepsFolder), DB: repositories.CustomScripts.DB}},
		OnBoard:      func() { bridge.AdeTaskBoardChanged(events) },
		OnUpdates:    func() { bridge.AdeTaskUpdatesChanged(events) },
		TrackedItems: repositories.AdeTrackedItems, Updates: updates,
		OnWorkflows: func() { bridge.AdeTaskWorkflowsChanged(events) },
		OnRepos:     func() { bridge.AdeTaskReposChanged(events) },
		Facts:       repositories.AdeFacts,
		Runner:      git.runner,
		Credentials: credentials,
		Logs:        repositories.AdeLogs,
		Sessions:    repositories.AdeSessions,
		Sandbox:     runs.Sandbox, RunSandbox: repositories.AdeRunSandbox, SessionSandbox: repositories.AdeSessionSandbox,
		Tracker:       tracker,
		CloseTerminal: closeTerminal,
		Windows:       repositories.Windows, ReviewWindows: repositories.AdeReview,
		CloseReviewWindows: closeReviewWindows,
		OnRuns: func(ev adewire.RunsChangedEvent) {
			bridge.AdeTaskRunsChanged(events, ev)
			notifier.HandleRuns(ev.Runs)
		},
		OnLog:    func(ev adewire.LogEvent) { bridge.AdeTaskLogAppended(events, ev) },
		OnReview: func(ev adewire.ReviewEvent) { bridge.AdeTaskReviewChanged(events, ev) },
		OnRateLimits: func(rl harness.RateLimits) {
			var five, seven *agentusage.Window
			if rl.FiveHour != nil {
				five = agentusage.FromSession(rl.FiveHour.UsedPercent, rl.FiveHour.ResetsAt)
			}
			if rl.SevenDay != nil {
				seven = agentusage.FromSession(rl.SevenDay.UsedPercent, rl.SevenDay.ResetsAt)
			}
			usage.Ingest(agentusage.SourceRun, five, seven)
		},
		OnSessions: func() {
			bridge.AdeTaskSessionsChanged(events)
			bridge.KeepAwakeRecompute(keepAwake)
		},
		CustomScripts: runs.Scripts, ScriptsHome: runs.Home,
		ScriptRunsOf: func(taskID string, limit int) ([]scriptruns.Run, error) { return runs.List(limit, taskID) },
		AgentDir:     adeRunsDir(config.KiraSpaceHome()),
		TasksDir:     filepath.Join(config.KiraSpaceHome(), "ade", "tasks"),
		HeadlessSettingSources: func() string {
			settings, err := repositories.Settings.GetAll()
			if err != nil {
				return ""
			}
			return settings.Ade.HeadlessSettingSources
		},
		Hermetic: func() bool {
			settings, err := repositories.Settings.GetAll()
			return err != nil || settings.Agent.Hermetic
		},
		AutofetchMinutes:  adeAutofetchMinutes(repositories),
		HomeDir:           userHome,
		RebaseTimeout:     rebaseTimeout,
		BranchRequestWait: branchRequestWait,
		Now:               time.Now,
	})
	tracker.SetStoppedHandler(board.OnTUIStopped)
	tracker.SetAgentStartedHandler(board.OnTUIAgentStarted)
	tracker.SetStartedHandler(board.OnTUIStarted)
	if err := board.Recover(); err != nil {
		slog.Warn("ade: recover task board", "scope", "ade", "err", err)
	}
	board.Start()
	return board
}

// adeGitPathSetting reads git.gitPath verbatim ("" by default). Not an executable: only
// Discovery.Status resolves it to one, so the board opens repos with GitStatus().Path.
func adeGitPathSetting(repositories *repos.Repos) func() string {
	return func() string {
		settings, err := repositories.Settings.GetAll()
		if err != nil {
			return ""
		}
		return settings.Git.GitPath
	}
}

// adeAutofetchMinutes is TaskBoardDeps.AutofetchMinutes' own construction — echoes
// git.fetchAutoIntervalMinutes into the board, the same leaf gitRegistry.Settings (wireGit)
// already reads.
func adeAutofetchMinutes(repositories *repos.Repos) func() int {
	return func() int {
		settings, err := repositories.Settings.GetAll()
		if err != nil {
			return 0
		}
		return settings.Git.FetchAutoIntervalMinutes
	}
}

// adeCloseTerminal is the board's CloseTerminal seam — terminal.Registry.Close never errors, so
// this just satisfies the func(id string) error signature.
func adeCloseTerminal(registry *terminal.Registry) func(id string) error {
	return func(id string) error {
		registry.Close(id)
		return nil
	}
}

// shutdownTracker is wireTracker's teardown half: flushes the tracker's still-in-memory lastActive
// values, then stops the hooks listener.
func shutdownTracker(tracker *ade.Tracker, hooks *harness.Hooks) {
	if err := tracker.Close(); err != nil {
		slog.Warn("ade: close", "scope", "ade", "err", err)
	}
	if err := hooks.Stop(); err != nil {
		slog.Warn("agent hooks: stop", "scope", "ade", "err", err)
	}
}

// newOpLog opens the persisted op log: interrupted rows become errors, old rows are pruned, the
// foreground lane is seeded from disk and rows other processes write are ingested. A storage failure leaves a memory-only log.
func newOpLog(repositories *repos.Repos) *oplog.Log {
	store := repositories.OpLog
	if err := store.ReconcileInterrupted("Kira Space quit while this ran."); err != nil {
		slog.Warn("op log: reconcile", "scope", "startup", "err", err)
	}
	if err := store.Prune(time.Now().Add(-oplog.Retention), oplog.MaxRows); err != nil {
		slog.Warn("op log: prune", "scope", "startup", "err", err)
	}
	if err := oplog.SweepDeadOwners(store, opOwnersDir()); err != nil {
		slog.Warn("op log: sweep foreign", "scope", "startup", "err", err)
	}
	cur, curErr := oplog.NewForeignCursor(store)
	l := oplog.New(oplog.WithStore(store))
	recent, err := store.Recent(oplog.FgCapacity)
	if err != nil {
		slog.Warn("op log: load", "scope", "startup", "err", err)
	}
	l.Seed(recent)
	if curErr != nil {
		slog.Warn("op log: foreign cursor", "scope", "startup", "err", curErr)
		return l
	}
	oplog.WatchForeign(l, store, opOwnersDir(), oplog.FlushInterval, cur)
	return l
}

// wireGit builds the git runner, discovery (over locator), session registry and router.
func wireGit(repositories *repos.Repos, locator gitclient.Locator, hosts *githost.Registry, crSync gitsession.CRSyncConfig, repoFiles *repocfg.Store) gitWired {
	opLog := newOpLog(repositories)
	gitRunner := oplog.NewRecordingRunner(gitclient.NewExecRunner(), opLog)
	gitDiscovery := gitclient.NewDiscovery(locator, gitRunner, gitclient.NewRealClock())
	gitRegistry := gitsession.NewRegistry(gitRunner)
	if hosts != nil {
		gitRegistry.Hosts = githost.Selector{Registry: hosts}
	}
	gitRegistry.OpLog = opLog
	gitRegistry.Settings = func() (protectedBranches []string, autoFetchMinutes int, gitPath string) {
		s, err := repositories.Settings.GetAll()
		if err != nil {
			slog.Warn("read git settings", "scope", "git", "err", err)
			return nil, 0, ""
		}
		return s.Git.ProtectedBranches, s.Git.FetchAutoIntervalMinutes, s.Git.GitPath
	}
	gitRegistry.RepoSettingsGet = repoFiles.Settings
	gitRegistry.RepoSettingsSet = repoFiles.Patch
	gitRegistry.CRStore = repositories.GitHostChangeRequests
	gitRegistry.CRSync = crSync

	askpassBroker, err := gitaskpass.New(gitaskpass.Options{})
	if err != nil {
		slog.Warn("start askpass broker", "scope", "startup", "err", err)
		askpassBroker = nil
	}

	gitRouter := gitrpc.New(gitrpc.Deps{
		Discovery: gitDiscovery, Runner: gitRunner, Registry: gitRegistry, ServerVersion: buildinfo.Version,
		Askpass: askpassBroker,
		DateFormat: func() string {
			s, err := repositories.Settings.GetAll()
			if err != nil {
				slog.Warn("read date format", "scope", "git", "err", err)
				return "relative"
			}
			return s.Appearance.DateFormat
		},
	})
	return gitWired{
		runner: gitRunner, discovery: gitDiscovery, registry: gitRegistry,
		askpassBroker: askpassBroker, router: gitRouter, opLog: opLog,
	}
}

// wireTermBroker arbitrates phone-attached agent terminals (P212 Part 2); a phone reaches only a
// running TUI task session, by ADE session id.
func wireTermBroker(tracker *ade.Tracker, registry *terminal.Registry, emitter appevent.Emitter) *mobileterm.Broker {
	return mobileterm.New(mobileterm.Deps{
		Registry: registry,
		Sessions: func(sessionID string) (string, bool) {
			tid, ok := tracker.TerminalOf(sessionID)
			if !ok {
				return "", false
			}
			rec, err := tracker.Get(sessionID)
			if err != nil || rec == nil || rec.Mode != model.AdeSessionModeTUI || rec.State != model.AdeSessionStateRunning || rec.TaskID == "" {
				return "", false
			}
			return tid, true
		},
		OnChange: func(h []mobileterm.Hold) { emitter.Emit(bridge.ChannelMobileTerminals, h) },
	})
}

// usageEnabled reads agent.usageEnabled fresh; a read failure counts as on (the default).
func usageEnabled(repositories *repos.Repos) func() bool {
	return func() bool {
		s, err := repositories.Settings.GetAll()
		if err != nil {
			return true
		}
		return s.Agent.UsageEnabled
	}
}

// notifyPrefs reads the agent.notify* leaves; a read failure turns notifications off.
func notifyPrefs(repositories *repos.Repos) func() agentnotify.Prefs {
	return func() agentnotify.Prefs {
		s, err := repositories.Settings.GetAll()
		if err != nil {
			slog.Warn("notify: read settings", "scope", "notify", "err", err)
			return agentnotify.Prefs{}
		}
		c := s.Agent
		return agentnotify.Prefs{
			Enabled: c.NotifyEnabled, OnFinished: c.NotifyOnFinished, OnNeedsInput: c.NotifyOnNeedsInput,
			OnRunEnded: c.NotifyOnRunEnded, IncludeMessage: c.NotifyIncludeMessage,
		}
	}
}

// describeAgent names what a hook event belongs to: the ADE task, else the code repository whose
// root holds cwd, else cwd's basename. Terminal tabs are not persisted, so no tab row exists.
func (w *Wired) describeAgent(terminalID, cwd string) agentnotify.Target {
	t := agentnotify.Target{}
	t.WindowKey, _ = w.TermRegistry.WindowOf(terminalID)
	if recordID, ok := w.Tracker.RecordOf(terminalID); ok {
		t.RecordID = recordID
		if rec, err := w.Tracker.Get(recordID); err == nil && rec != nil {
			t.TaskID = rec.TaskID
			if p, err := w.Agents.For(rec.Provider); err == nil {
				t.Agent = p.Label()
			}
			if title := w.taskTitle(rec.TaskID); title != "" {
				t.Name = title
			}
		}
	}
	if t.Name == "" {
		t.Name = w.repoNameFor(cwd)
	}
	return t
}

func (w *Wired) taskTitle(taskID string) string {
	task, err := w.Repos.AdeTasks.GetTask(taskID)
	if err != nil {
		return ""
	}
	return task.Title
}

// repoNameFor is the name of the code repository with the longest root holding cwd.
func (w *Wired) repoNameFor(cwd string) string {
	if cwd == "" {
		return "Claude session"
	}
	repoList, err := w.Repos.CodeRepos.List()
	best, name := -1, ""
	if err == nil {
		for _, r := range repoList {
			if r.Root != "" && (cwd == r.Root || strings.HasPrefix(cwd, strings.TrimRight(r.Root, "/")+"/")) && len(r.Root) > best {
				best, name = len(r.Root), r.Name
			}
		}
	}
	if name == "" {
		return filepath.Base(cwd)
	}
	return name
}

// revealNote answers a notification click: focus the owning window and tell it what to show.
func (w *Wired) revealNote(n agentnotify.Note) {
	focus := w.AdeTask.FocusWindow
	if focus == nil {
		focus = w.Windows.Focus
	}
	if n.RecordID != "" {
		if ok, err := w.AdeTask.FocusSession(context.Background(), adewire.FocusSessionArgs{SessionID: n.RecordID}); err == nil && ok {
			return
		}
	}
	if n.TerminalID != "" {
		if key, ok := w.TermRegistry.WindowOf(n.TerminalID); ok && focus(key) {
			w.Emitter.EmitTo(key, bridge.ChannelAgentRevealTerminal, map[string]string{"terminalId": n.TerminalID})
			return
		}
	}
	if key, ok := w.Windows.MainKey(); ok {
		focus(key)
		if n.ScriptRunID != "" {
			w.Emitter.EmitTo(key, bridge.ChannelAgentRevealScriptRun, map[string]string{"runId": n.ScriptRunID, "label": n.Title})
			return
		}
		if n.TaskID != "" {
			w.Emitter.EmitTo(key, bridge.ChannelAgentRevealTask, map[string]string{"taskId": n.TaskID})
		}
	}
}

// wireRepoConfig builds the repo config store over <home>/config/repos, moves the legacy rows into files
// once and loads the folder. The change hooks and the watch start in watchRepoConfig, once the git
// router and the board exist.
func wireRepoConfig(repositories *repos.Repos, hosts *githost.Registry) *repocfg.Store {
	var ids []string
	if hosts != nil {
		for _, info := range hosts.Infos() {
			ids = append(ids, info.ID)
		}
	}
	store, err := repocfg.New(repocfg.Options{DB: repositories.CustomScripts.DB, Dir: configfs.ModuleDir(config.KiraSpaceHome(), repocfg.Folder), GitHosts: ids})
	if err != nil {
		panic(fmt.Sprintf("appwire: repo config (static schema): %v", err))
	}
	repositories.AdeRepoConfig.Files = store
	if err := store.Start(); err != nil {
		slog.Warn("repo config: start", "scope", "startup", "err", err)
	}
	return store
}

// watchRepoConfig routes repo file changes to the git clients, the board and the renderer, then
// watches the folder.
func (w *Wired) watchRepoConfig(events *bridge.Events) {
	w.RepoConfig.OnChange = func(changes []repocfg.Change) {
		for _, c := range changes {
			if !c.FromPatch && !reflect.DeepEqual(c.Old.Settings, c.New.Settings) {
				w.Git.router.EmitRepoSettingsChanged(c.Path, c.New.Settings)
			}
		}
		w.AdeBoard.RepoConfigChanged(changes)
	}
	w.RepoConfig.OnReload = func() { bridge.RepoConfigChanged(events) }
	w.RepoConfig.Watch()
}

// repoFile names the file applying to a repo (its stem) and its error.
func repoFile(store *repocfg.Store) func(repoID string) (string, *adewire.RepoProblem) {
	return func(repoID string) (string, *adewire.RepoProblem) {
		file, p := store.FileInfo(repoID)
		if file == "" {
			return "", nil
		}
		var problem *adewire.RepoProblem
		if p != nil {
			problem = &adewire.RepoProblem{Line: p.Line, Message: p.Message}
		}
		return strings.TrimSuffix(file, ".yaml"), problem
	}
}

// promptsPath is the user prompt overrides file under an app home.
func promptsPath(home string) string {
	return filepath.Join(configfs.ModuleDir(home, llmprompts.Folder), llmprompts.File)
}

// configFolders maps every config module the file service edits to its folder.
func configFolders(home, repoDir string) map[string]string {
	folders := map[string]string{repocfg.Folder: repoDir}
	for _, f := range []string{connfiles.Folder, apifiles.Folder, scripts.Folder, adeflow.WorkflowsFolder, adeflow.StepsFolder, agentctx.Folder} {
		folders[f] = configfs.ModuleDir(home, f)
	}
	return folders
}

// adeRunsDir holds the per-run MCP files; runtime state, not config.
func adeRunsDir(home string) string { return filepath.Join(home, "ade/runs") }
