package ade

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adeflow"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitaskpass"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitcred"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitprepare"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/agentsandbox"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// board.go is the ADE engine: tasks hold branches across repos; this type assembles the
// adewire.Board snapshot from the task store plus git facts. It returns wire types directly
// (internal/bridge/adewire is a pure type leaf the layering test allows). Shared git fact helpers
// live in gitfacts.go.

const (
	boardConnID    = gitsession.ConnID("ade-board")
	boardConnLabel = "Kira Space ade board"
	boardFanOut    = 4
)

// TaskBoardDeps is NewTaskBoard's construction seam.
type TaskBoardDeps struct {
	Tasks *repos.AdeTaskRepo
	// PromptSink receives the "move to next stage" prompt a finished handover raises; nil drops it.
	PromptSink      prompts.Sink
	Statuses        *repos.AdeStatusRepo
	RepoConfig      *repos.AdeRepoConfigRepo
	Facts           *repos.AdeFactsRepo
	CodeRepos       *repos.CodeReposRepo
	GitRepoSettings func(repoID string) (model.GitRepoSettings, error)
	Registry        *gitsession.Registry
	// GitStatus is the app's git discovery (cached); every rebase-conflict check consults it first.
	GitStatus func(ctx context.Context) gitclient.GitStatus
	Askpass   *gitaskpass.Broker
	Workflows *adeflow.Reader
	// Runner spawns git for repo identification (folder import).
	Runner gitclient.Runner
	// Ops is the operation log: agent sessions, worktree setups and MCP tool calls land in it.
	Ops *oplog.Log
	// Scripts runs the environment deploy-sha scripts; nil = the OS runner.
	Scripts gitprepare.Runner
	// Logs stores run and worktree-setup logs; OnLog pushes each stored batch.
	Logs  *repos.AdeLogsRepo
	OnLog func(adewire.LogEvent)
	// Sessions stores the headless session rows; OnSessions fires when they change.
	Sessions   *repos.AdeSessionsRepo
	OnSessions func()
	// OnRateLimits receives the rate-limit windows a headless run's stream reports (nil = ignored).
	OnRateLimits func(harness.RateLimits)
	// OnRuns pushes changed runs on kira:adetask:runs.
	OnRuns func(adewire.RunsChangedEvent)
	// OnReview pushes a review step's change on kira:adetask:review.
	OnReview func(adewire.ReviewEvent)
	// AgentDir holds each run's MCP config file (0700 dir, 0600 files).
	AgentDir string
	// TasksDir holds each task's folder for runs and sessions on a task with no branch.
	TasksDir string
	// Prompts resolves the editable LLM prompt texts; nil means the registry defaults.
	Prompts llmprompts.Source
	// Agent resolves the provider of a new run. AgentBin overrides its CLI (a test points it at a fake).
	Agent    harness.Selector
	AgentBin string
	// ContextFile returns the path of the agent context for a run, "" for none; nil = none.
	ContextFile func() (string, error)
	// AgentConfig prepares the app's agent settings for a launch; nil = none.
	AgentConfig agentcfg.PrepareFunc
	// Tracker records and launches interactive (TUI) task sessions.
	Tracker *Tracker
	// CloseTerminal closes a running terminal by id (Archive).
	CloseTerminal func(terminalID string) error
	// Windows and ReviewWindows back review windows; CloseReviewWindows closes a task's native
	// review windows (Archive).
	Windows            *repos.WindowsRepo
	ReviewWindows      *repos.AdeReviewWindowsRepo
	CloseReviewWindows func(taskID string)
	// ScriptRunsOf lists the script runs started for a task, newest first (run_outcome kind automation).
	// CustomScripts and ScriptsHome resolve the smart script of a workflow step and its own folder.
	CustomScripts *scripts.Repo
	ScriptsHome   string
	ScriptRunsOf  func(taskID string, limit int) ([]scriptruns.Run, error)
	// HeadlessSettingSources returns the ade.headlessSettingSources setting, read fresh per run.
	HeadlessSettingSources func() string
	// Servers is where a smart step's ticked MCP servers come from; the zero value reads the CLI.
	Servers agentcfg.ServerSource
	// Hermetic returns the agent.hermetic setting, read fresh per launch; nil = on.
	Hermetic func() bool
	// Sandbox confines headless agents (P287); nil = no sandbox. RunSandbox stores each run's
	// policy and denials.
	Sandbox    agentsandbox.Enforcer
	RunSandbox *repos.AdeRunSandboxRepo
	// Trackers are the issue tracker providers; each adds a task artifact source and fills TaskIssue. nil = none.
	Trackers *issuetracker.Registry
	// ArtifactSources append to the built-in task artifact sources.
	ArtifactSources []ArtifactSource
	// BranchRequestWait is how long an agent's branch request waits for the user; 0 = defaultBranchRequestWait.
	BranchRequestWait time.Duration
	// RebaseTimeout bounds a rebase run; 0 = defaultRebaseTimeout.
	RebaseTimeout time.Duration
	// SetRepoSettings writes git_repo_settings leaves through the path git-ui's repoSettings.set
	// uses, notification included, so git-ui sees a new prepare script.
	SetRepoSettings func(repoID string, patch model.GitRepoSettingsPatch) error
	OnBoard         func()
	OnWorkflows     func()
	OnRepos         func()
	// Credentials receives the remote ops' credential prompts, shown in Kira Space's windows.
	Credentials      *gitcred.Relay
	AutofetchMinutes func() int
	HomeDir          string
	Now              func() time.Time
}

// TaskBoard is the v2 engine: one gitsession.Conn holding every repo a live task touches, a per-repo
// mutex serializing writes and remote ops, and per-repo fact caches.
type TaskBoard struct {
	deps TaskBoardDeps
	// sessionOps holds the op of each live headless agent session until its outcome lands.
	sessionOps sessionOps
	conn       *gitsession.Conn
	checker    *rebaseChecker
	ctx        context.Context
	cancel     context.CancelFunc

	importMu sync.Mutex // serializes repo imports (folder add, folder watch rescans)
	// afterImport runs after each repo a folder scan imports; tests flip state mid-scan with it.
	afterImport func(codeRepoID string)
	folderWMu   sync.Mutex
	folderW     map[string]*folderWatcher
	stopFlowW   func()
	wfTimer     *time.Timer
	startOnce   sync.Once

	mu      sync.Mutex
	repoMus map[string]*sync.Mutex
	// sandboxTargets maps a live sandboxed run id to its sandboxTarget.
	sandboxTargets sync.Map
	byGitRepoID    map[string]string // gitclient RepoID -> codeRepoID, populated on open
	gitRepoIDOf    map[string]string // codeRepoID -> gitclient RepoID
	caches         map[string]*repoCaches
	rebase         map[string]*rebaseCache
	boardTimer     *time.Timer

	runMu            sync.Mutex                 // guards live, setupLive, archiving, taskMus, finishes, stepMsgs
	live             map[string]*liveRun        // run id -> its process
	tuiGrants        map[string]tuiBinding      // TUI record id -> the agent MCP grant its launch registered
	setupLive        map[string]*liveRun        // branch id -> its running prepare script
	archiving        map[string]int             // task id -> archives in progress; blocks launches
	taskMus          map[string]*sync.Mutex     // task id -> serializes that task's run transitions
	envScriptMus     sync.Map                   // code repo id -> *sync.Mutex: one env-script pass per repo at a time
	finishes         map[string]agentmcp.Finish // run id -> last finish_step call
	claims           map[string]string          // branch id -> the smart script working in its worktree
	recoveredRebases []model.AdeRun             // rebase runs Recover interrupted; Start re-reads git for them
	stepMsgs         map[string]string          // task|step -> the Run dialog's edited message
	agent            *agentmcp.Server
	branchReqs       branchRequests // open agent branch requests (P408)
	wg               sync.WaitGroup // background setups and runs; Close waits
	closeMu          sync.Mutex     // orders goTracked's wg.Add before Close's wg.Wait
	closed           bool
}

// prompts is the source of every LLM prompt the board sends.
func (b *TaskBoard) prompts() llmprompts.Source {
	if b.deps.Prompts == nil {
		return llmprompts.Defaults()
	}
	return b.deps.Prompts
}

// NewTaskBoard builds the engine; Close releases it.
func NewTaskBoard(deps TaskBoardDeps) *TaskBoard {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	ctx, cancel := context.WithCancel(oplog.WithOrigin(context.Background(), oplog.Origin{Module: oplog.ModuleADE, Trigger: oplog.TriggerAuto}))
	b := &TaskBoard{
		deps: deps, ctx: ctx, cancel: cancel, folderW: map[string]*folderWatcher{},
		repoMus: map[string]*sync.Mutex{}, byGitRepoID: map[string]string{}, gitRepoIDOf: map[string]string{},
		caches: map[string]*repoCaches{}, rebase: map[string]*rebaseCache{}, live: map[string]*liveRun{}, tuiGrants: map[string]tuiBinding{}, setupLive: map[string]*liveRun{},
		taskMus: map[string]*sync.Mutex{}, archiving: map[string]int{}, finishes: map[string]agentmcp.Finish{}, claims: map[string]string{}, stepMsgs: map[string]string{},
	}
	b.agent = agentmcp.NewServer(agentmcp.Options{Dir: deps.AgentDir, OnFinish: b.recordFinish, Space: b, Outcomes: b, Review: b, SandboxGit: b, Git: b, Handover: b, Prompts: b.prompts(), OnToolCall: b.mcpRecorder})
	if deps.Tracker != nil {
		deps.Tracker.deps.Ops, deps.Tracker.deps.OpSubject = deps.Ops, b.opSubject
	}
	b.conn = gitsession.NewConn(boardConnID, "ade-board", boardConnLabel, b.handleEmit)
	b.conn.Module = oplog.ModuleADE
	b.conn.RouteCredentials(func(ctx context.Context, req gitaskpass.Request) (string, bool) {
		return deps.Credentials.Ask(ctx, boardConnLabel, req)
	})
	b.checker = newRebaseChecker(ctx, deps.GitStatus, b.scheduleBoard)
	return b
}

// track registers one background task with wg; false once Close ran. The caller owes a wg.Done.
func (b *TaskBoard) track() bool {
	b.closeMu.Lock()
	defer b.closeMu.Unlock()
	if b.closed {
		return false
	}
	b.wg.Add(1)
	return true
}

// runTracked runs fn on the caller's goroutine unless Close ran; Close waits for it.
func (b *TaskBoard) runTracked(fn func()) {
	if !b.track() {
		return
	}
	defer b.wg.Done()
	fn()
}

// goTracked runs fn in the background; Close waits for it.
func (b *TaskBoard) goTracked(fn func()) {
	if !b.track() {
		return
	}
	go func() {
		defer b.wg.Done()
		fn()
	}()
}

// Close stops pending work and releases the Conn.
func (b *TaskBoard) Close() {
	b.cancel()
	b.closeMu.Lock()
	b.closed = true
	b.closeMu.Unlock()
	b.abandonAllBranchRequests()
	// Stop the MCP server first: a late finish_step must not start work after wg.Wait returns.
	if err := b.agent.Close(); err != nil {
		slog.Warn("ade: close agent server", "scope", "ade", "err", err)
	}
	b.mu.Lock()
	if b.boardTimer != nil {
		b.boardTimer.Stop()
	}
	if b.wfTimer != nil {
		b.wfTimer.Stop()
	}
	b.mu.Unlock()
	b.stopWatchers()
	b.checker.close()
	b.wg.Wait()
	b.conn.Close()
}

func (b *TaskBoard) handleEmit(method string, payload any) {
	if method != "repo.changed" {
		return
	}
	if _, ok := payload.(gitsession.Event); ok {
		b.scheduleBoard()
	}
}

// scheduleBoard debounces a board invalidation (a burst of ref moves or check completions).
func (b *TaskBoard) scheduleBoard() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.boardTimer != nil {
		b.boardTimer.Stop()
	}
	b.boardTimer = time.AfterFunc(adeRepoChangedDebounce, func() { b.runTracked(b.notifyBoard) })
}

func (b *TaskBoard) notifyBoard() {
	if b.deps.OnBoard != nil {
		b.deps.OnBoard()
	}
}

func (b *TaskBoard) repoMutex(codeRepoID string) *sync.Mutex {
	b.mu.Lock()
	defer b.mu.Unlock()
	m, ok := b.repoMus[codeRepoID]
	if !ok {
		m = &sync.Mutex{}
		b.repoMus[codeRepoID] = m
	}
	return m
}

func (b *TaskBoard) cachesFor(codeRepoID string) (*repoCaches, *rebaseCache) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.caches[codeRepoID]
	if !ok {
		c = newRepoCaches()
		b.caches[codeRepoID] = c
		b.rebase[codeRepoID] = newRebaseCache()
	}
	return c, b.rebase[codeRepoID]
}

// openRepo resolves the code repo to its root and opens it on the board's Conn.
func (b *TaskBoard) openRepo(ctx context.Context, codeRepoID string) (*gitsession.RepoEntry, error) {
	b.mu.Lock()
	gitID, known := b.gitRepoIDOf[codeRepoID]
	b.mu.Unlock()
	if known {
		if entry, ok := b.conn.Entry(gitID); ok {
			// Follow git.gitPath: the entry bakes the path at open time.
			// tooOld still reads through the open entry; only the checks fail.
			switch status := b.deps.GitStatus(ctx); status.Kind {
			case "ok":
				entry.Repo.SetGitPath(status.Path)
			case "tooOld":
			default:
				return nil, fmt.Errorf("ade: git is unavailable: %s", status.Kind)
			}
			return entry, nil
		}
	}
	rec, err := b.deps.CodeRepos.Get(codeRepoID)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, fmt.Errorf("ade: board: code repo %s not found", codeRepoID)
	}
	status := b.deps.GitStatus(ctx)
	if status.Kind != "ok" {
		return nil, fmt.Errorf("ade: git is unavailable: %s", status.Kind)
	}
	summary, err := b.conn.Open(ctx, b.deps.Registry, status.Path, rec.Root)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.byGitRepoID[summary.RepoID] = codeRepoID
	b.gitRepoIDOf[codeRepoID] = summary.RepoID
	b.mu.Unlock()
	entry, ok := b.conn.Entry(summary.RepoID)
	if !ok {
		return nil, fmt.Errorf("ade: board: repo entry missing for %s after open", codeRepoID)
	}
	return entry, nil
}

// boardData is one consistent read of the task store.
type boardData struct {
	tasks      []model.AdeTask
	statuses   []model.AdeTaskStatus
	defaults   model.AdeStatusDefaults
	branches   []model.AdeTaskBranch
	plan       []model.AdeTaskPlanRow
	runs       map[string][]model.AdeRun
	reviews    map[string]model.AdeReview
	setups     map[string]model.AdeWorktreeSetup
	archived   []model.AdeTask
	archBranch []model.AdeTaskBranch
	handovers  map[string]repos.BoardHandover
}

func (b *TaskBoard) load() (boardData, error) {
	var d boardData
	var err error
	if d.tasks, err = b.deps.Tasks.ListLive(); err != nil {
		return d, err
	}
	if d.statuses, err = b.deps.Statuses.List(); err != nil {
		return d, err
	}
	if d.defaults, err = b.deps.Statuses.Defaults(); err != nil {
		return d, err
	}
	if d.branches, err = b.deps.Tasks.BranchesLive(); err != nil {
		return d, err
	}
	if d.plan, err = b.deps.Tasks.PlanRows(); err != nil {
		return d, err
	}
	if d.runs, err = b.deps.Tasks.RunsByLiveTask(); err != nil {
		return d, err
	}
	if d.reviews, err = b.deps.Tasks.Reviews(); err != nil {
		return d, err
	}
	if d.setups, err = b.deps.Tasks.SetupByBranch(); err != nil {
		return d, err
	}
	if d.archived, err = b.deps.Tasks.ListArchived(); err != nil {
		return d, err
	}
	if d.archBranch, err = b.deps.Tasks.BranchesArchived(); err != nil {
		return d, err
	}
	d.handovers, err = b.deps.Tasks.BoardHandovers()
	return d, err
}

// branchesByRepo groups branches by code repo, repos sorted by id for stable output.
func branchesByRepo(branches []model.AdeTaskBranch) (ids []string, by map[string][]model.AdeTaskBranch) {
	by = make(map[string][]model.AdeTaskBranch)
	for _, br := range branches {
		by[br.CodeRepoID] = append(by[br.CodeRepoID], br)
	}
	ids = make([]string, 0, len(by))
	for id := range by {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, by
}

// Board assembles the snapshot. It never blocks on a rebase-conflict check: a miss reports
// `checking` and queues the check.
func (b *TaskBoard) Board(ctx context.Context) (adewire.Board, error) {
	d, err := b.load()
	if err != nil {
		return adewire.Board{}, err
	}
	repoIDs, byRepo := branchesByRepo(d.branches)
	results := make(map[string]*repoResult, len(repoIDs))
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(boardFanOut)
	for _, id := range repoIDs {
		g.Go(func() error {
			res := b.repoFacts(gctx, id, byRepo[id], d.setups)
			mu.Lock()
			results[id] = res
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return adewire.Board{}, err
	}

	var hadCommits []string
	merged := map[string]int64{}
	for _, id := range repoIDs {
		res := results[id]
		hadCommits = append(hadCommits, res.hadCommits...)
		for k, v := range res.merged {
			merged[k] = v
		}
		b.applyChecks(id, res)
	}
	if err := b.deps.Tasks.MarkBranchFacts(hadCommits, merged); err != nil {
		return adewire.Board{}, err
	}
	return b.assemble(d, repoIDs, results), nil
}

// applyChecks fills each visible branch's conflict state from the checker, queueing a miss.
func (b *TaskBoard) applyChecks(repoID string, res *repoResult) {
	_, cache := b.cachesFor(repoID)
	for id, job := range res.jobs {
		br := res.branches[id]
		st, known := b.checker.peek(repoID, cache, job.key)
		if !known {
			b.checker.request(job)
			st = rebaseState{Check: conflictChecking, Paths: []string{}}
		}
		br.ConflictCheck, br.ConflictsIfRebased, br.ConflictCheckReason = st.Check, st.Paths, st.Reason
		res.branches[id] = br
	}
}

func (b *TaskBoard) assemble(d boardData, repoIDs []string, results map[string]*repoResult) adewire.Board {
	branchByTask := make(map[string][]string)
	branches := make([]adewire.Branch, 0, len(d.branches))
	for _, sb := range d.branches {
		wb, ok := results[sb.CodeRepoID].branches[sb.ID]
		if !ok {
			continue
		}
		branches = append(branches, wb)
		branchByTask[sb.TaskID] = append(branchByTask[sb.TaskID], sb.ID)
	}

	tasks := make([]adewire.Task, 0, len(d.tasks))
	liveWf := b.liveWorkflows()
	for _, t := range d.tasks {
		wt := b.toWireTask(t, branchByTask[t.ID], d.runs[t.ID], d.reviews)
		decorateTask(&wt, t, liveWf)
		applyHandovers(&wt, d.handovers[t.ID])
		tasks = append(tasks, wt)
	}
	b.badgeTasks(tasks)

	plan := adewire.Plan{Day: map[string]string{}, Order: make([]string, 0, len(d.tasks)), QueuedAfter: map[string]string{}, Unpushed: map[string]bool{}, Backlog: []string{}}
	live := make(map[string]bool, len(d.tasks))
	for _, t := range d.tasks {
		live[t.ID] = true
	}
	for _, p := range d.plan {
		if !live[p.TaskID] {
			continue
		}
		plan.Order = append(plan.Order, p.TaskID)
		if p.Backlog {
			plan.Backlog = append(plan.Backlog, p.TaskID)
		}
		if p.Day != nil {
			plan.Day[p.TaskID] = *p.Day
		}
	}
	for _, sb := range d.branches {
		if sb.QueuedAfter != "" {
			plan.QueuedAfter[sb.ID] = sb.QueuedAfter
		}
		if wb, ok := results[sb.CodeRepoID].branches[sb.ID]; ok && wb.Upstream != "" && wb.UpstreamAhead > 0 && wb.UpstreamBehind > 0 {
			plan.Unpushed[sb.ID] = true
		}
	}

	pairs := make([]adewire.Pair, 0)
	states := make([]adewire.RepoState, 0, len(repoIDs))
	for _, id := range repoIDs {
		pairs = append(pairs, results[id].pairs...)
		states = append(states, results[id].state)
	}

	autofetch := 0
	if b.deps.AutofetchMinutes != nil {
		autofetch = b.deps.AutofetchMinutes()
	}
	return adewire.Board{
		Tasks: tasks, Statuses: toWireStatuses(d.statuses), StatusDefaults: toWireDefaults(d.defaults), Branches: branches, Plan: plan, Pairs: pairs, History: toWireHistory(d.archived, d.archBranch),
		Repos: states, WorktreeBasePath: filepath.Join(b.deps.HomeDir, "wt"), AutofetchMinutes: autofetch,
	}
}

func (b *TaskBoard) toWireTask(t model.AdeTask, branchIDs []string, runs []model.AdeRun, reviews map[string]model.AdeReview) adewire.Task {
	out := adewire.Task{
		ID: t.ID, Kind: t.Kind, Title: t.Title, Owner: t.Owner, HostURL: t.HostURL, HostLink: b.hostLink(t.HostURL), WorkflowID: t.WorkflowID,
		EntryStepID: t.EntryStepID, Since: map[string]int{}, Est: t.Est, Notes: t.Notes, Color: t.Color, CreatedAt: t.CreatedAt,
		StatusID: t.StatusID, StatusBy: t.StatusBy, StatusStep: t.StatusStep,
		BranchIDs: make([]string, 0, len(branchIDs)), Runs: make([]adewire.Run, 0, len(runs)),
		HandoverDrafts: []adewire.Handover{}, BranchRequestID: b.branchRequestOf(t.ID),
	}
	out.BranchIDs = append(out.BranchIDs, branchIDs...)
	if t.JiraKey != "" || t.JiraURL != "" {
		out.Jira = &adewire.Jira{Key: t.JiraKey, URL: t.JiraURL}
	}
	if t.SinceJSON != "" {
		if err := json.Unmarshal([]byte(t.SinceJSON), &out.Since); err != nil || out.Since == nil {
			out.Since = map[string]int{}
			slog.Warn("ade board: unreadable pass of task", "scope", "ade", "task", t.ID, "err", err)
		}
	}
	live := make(map[string]bool, len(branchIDs))
	for _, id := range branchIDs {
		live[id] = true
	}
	for _, r := range runs {
		if r.Purpose == model.AdeRunPurposeReview { // a round shows through its review run
			continue
		}
		if r.BranchID != "" && !live[r.BranchID] { // history of a detached branch
			continue
		}
		w := toWireRun(r)
		if r.Purpose == "" {
			w.Returns = countReturns(runs, r.StepID, r.BranchID, r.Attempt)
		}
		if rev, ok := reviews[r.ID]; ok {
			w.Review = reviewWire(r, rev, latestRoundOf(runs, r.ID))
		}
		out.Runs = append(out.Runs, w)
	}
	return out
}

func toWireRun(r model.AdeRun) adewire.Run {
	run := adewire.Run{
		ID: r.ID, TaskID: r.TaskID, StepID: r.StepID, BranchID: r.BranchID, Attempt: r.Attempt,
		State: r.State, Loops: r.Loops, Note: r.Note, Summary: r.Summary, SessionID: r.SessionID,
		ExitCode: r.ExitCode, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, Outcome: r.Outcome, Purpose: r.Purpose,
		Resumable: runResumable(r),
	}
	return run
}

func toWireHistory(tasks []model.AdeTask, branches []model.AdeTaskBranch) []adewire.HistoryEntry {
	byTask := make(map[string][]model.AdeTaskBranch)
	for _, br := range branches {
		byTask[br.TaskID] = append(byTask[br.TaskID], br)
	}
	out := make([]adewire.HistoryEntry, 0, len(tasks))
	for _, t := range tasks {
		e := adewire.HistoryEntry{TaskID: t.ID, Title: taskTitle(t, byTask[t.ID]), CodeRepoIDs: []string{}, ArchivedAt: *t.ArchivedAt}
		seen := map[string]bool{}
		var latest int64
		allMerged := len(byTask[t.ID]) > 0
		for _, br := range byTask[t.ID] {
			if !seen[br.CodeRepoID] {
				seen[br.CodeRepoID] = true
				e.CodeRepoIDs = append(e.CodeRepoIDs, br.CodeRepoID)
			}
			if br.MergedAt == nil {
				allMerged = false
			} else if *br.MergedAt > latest {
				latest = *br.MergedAt
			}
		}
		if allMerged {
			e.MergedAt = &latest
		}
		out = append(out, e)
	}
	return out
}

// ChangeRequests resolves each created branch's change request and each used repo's host availability.
func (b *TaskBoard) ChangeRequests(ctx context.Context) (adewire.ChangeRequestsResult, error) {
	branches, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return adewire.ChangeRequestsResult{}, err
	}
	repoIDs, byRepo := branchesByRepo(branches)
	out := adewire.ChangeRequestsResult{Repos: make([]adewire.RepoHost, 0, len(repoIDs)), Branches: map[string]adewire.ChangeRequest{}, BranchURLs: map[string]string{}}
	var mu sync.Mutex
	rows := make(map[string]adewire.RepoHost, len(repoIDs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(boardFanOut)
	for _, id := range repoIDs {
		g.Go(func() error {
			entry, err := b.openRepo(gctx, id)
			if err != nil {
				slog.Warn("ade board: prs open repo", "scope", "ade", "repo", id, "err", err)
				mu.Lock()
				rows[id] = adewire.RepoHost{CodeRepoID: id, Kind: "unavailable"}
				mu.Unlock()
				return nil
			}
			kind, host, found, urls := resolveRepoHost(gctx, entry, byRepo[id])
			mu.Lock()
			rows[id] = adewire.RepoHost{CodeRepoID: id, Kind: kind, Host: host}
			for k, v := range found {
				out.Branches[k] = v
			}
			for k, v := range urls {
				out.BranchURLs[k] = v
			}
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait() // per-repo failures are reported as kind "unavailable", never as an error
	for _, id := range repoIDs {
		out.Repos = append(out.Repos, rows[id])
	}
	return out, nil
}

// resolveRepoHost resolves the repo's host and each branch's change request and web page. kind is
// the first non-ok lookup kind, else "ok".
func resolveRepoHost(ctx context.Context, entry *gitsession.RepoEntry, branches []model.AdeTaskBranch) (kind string, host *githost.Info, found map[string]adewire.ChangeRequest, urls map[string]string) {
	kind = "ok"
	if info, ok := entry.HostInfo(ctx); ok {
		host = &info
	}
	found, urls = map[string]adewire.ChangeRequest{}, map[string]string{}
	for _, br := range branches {
		if br.Name == "" {
			continue
		}
		if u, ok := entry.BranchURL(ctx, br.Name); ok {
			urls[br.ID] = u
		}
		res := entry.ResolveBranchChangeRequest(ctx, br.Name)
		if res.Kind != "ok" {
			kind = res.Kind
		} else if len(res.ChangeRequests) > 0 {
			cr := res.ChangeRequests[0]
			found[br.ID] = adewire.ChangeRequest{Number: cr.Number, Title: cr.Title, State: cr.State, URL: cr.URL}
		}
	}
	return kind, host, found, urls
}

// Refresh fetches the given repos (none = every repo a live task uses; the UI sends the ids it shows), marks PR-merged branches,
// then runs and awaits the rebase-conflict checks so the board it emits is settled.
func (b *TaskBoard) Refresh(ctx context.Context, codeRepoIDs []string) (adewire.RefreshResult, error) {
	if len(codeRepoIDs) == 0 {
		branches, err := b.deps.Tasks.BranchesLive()
		if err != nil {
			return adewire.RefreshResult{}, err
		}
		codeRepoIDs, _ = branchesByRepo(branches)
	}
	rows := make([]adewire.RepoRefresh, len(codeRepoIDs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(boardFanOut)
	for i, id := range codeRepoIDs {
		g.Go(func() error {
			rows[i] = b.refreshRepo(gctx, id)
			return nil
		})
	}
	_ = g.Wait() // refreshRepo reports failures per repo
	b.notifyBoard()
	return adewire.RefreshResult{Repos: rows}, nil
}

func refreshFailure(id string, err error) adewire.RepoRefresh {
	return adewire.RepoRefresh{CodeRepoID: id, MergedInto: []adewire.MergedInto{},
		Error: &adewire.RemoteOpError{Kind: "Unknown", Message: err.Error()}}
}

func (b *TaskBoard) refreshRepo(ctx context.Context, id string) adewire.RepoRefresh {
	// Scripts run beside the fetch, outside the repo mutex: they can be slow and need nothing the
	// fetch brings, so a slow deploy check must not hold the fetch back.
	scriptsDone := make(chan struct{})
	go func() {
		defer close(scriptsDone)
		if err := b.RunEnvScripts(ctx, id); err != nil {
			slog.Warn("ade env scripts", "scope", "ade", "repo", id, "err", err)
		}
	}()
	defer func() { <-scriptsDone }()
	mu := b.repoMutex(id)
	mu.Lock()
	defer mu.Unlock()

	out := adewire.RepoRefresh{CodeRepoID: id, MergedInto: []adewire.MergedInto{}}
	entry, err := b.openRepo(ctx, id)
	if err != nil {
		return refreshFailure(id, err)
	}
	remote, hasRemote := entry.DefaultRemote(ctx)
	before, err := entry.BranchInventory(ctx)
	if err != nil {
		return refreshFailure(id, err)
	}
	marksBefore, err := b.marksOfRepo(id)
	if err != nil {
		return refreshFailure(id, err)
	}
	beforeTips := make(map[string]string, len(before))
	for _, r := range before {
		beforeTips[r.Ref] = r.Tip
	}
	if hasRemote {
		res, err := entry.RunRemote(ctx, b.conn, gitsession.RemoteOpParams{Kind: "fetch", Remote: remote, Prune: true}, gitsession.RemoteDeps{Askpass: b.deps.Askpass})
		if err != nil {
			return refreshFailure(id, err)
		}
		if !res.OK {
			out.Error = remoteOpError(res.Error)
			return out
		}
	}

	live, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return refreshFailure(id, err)
	}
	var mine []model.AdeTaskBranch
	for _, br := range live {
		if br.CodeRepoID == id {
			mine = append(mine, br)
		}
	}
	if hasRemote {
		if out.RefsChanged, err = countBranchRefsChanged(ctx, entry, mine, beforeTips, remote); err != nil {
			return refreshFailure(id, err)
		}
	}

	facts := b.repoFacts(ctx, id, mine, nil)
	out.MergedInto = newlyMerged(mine, facts, marksBefore)
	if err := b.deps.Tasks.MarkBranchFacts(facts.hadCommits, facts.merged); err != nil {
		return refreshFailure(id, err)
	}
	prMerged := b.checkPrMerges(ctx, entry, mine, facts)
	if len(prMerged) > 0 {
		if err := b.deps.Tasks.MarkBranchFacts(nil, prMerged); err != nil {
			return refreshFailure(id, err)
		}
	}

	b.checker.forgetFailures(id)
	waits := make([]<-chan struct{}, 0, len(facts.jobs))
	for branchID, job := range facts.jobs {
		if _, gone := prMerged[branchID]; gone {
			continue
		}
		waits = append(waits, b.checker.request(job))
	}
	for _, w := range waits {
		select {
		case <-w:
		case <-ctx.Done():
			return out
		}
	}
	return out
}

func remoteOpError(e *gitsession.RemoteOpError) *adewire.RemoteOpError {
	if e == nil {
		return nil
	}
	return &adewire.RemoteOpError{Kind: e.Kind, Message: e.Message, RemoteMessage: e.RemoteMessage}
}

// checkPrMerges is the squash/rebase-merge signal: a mine branch with a merged PR counts as merged
// though its tip never became an ancestor of main. Returns branch id -> merge time.
func (b *TaskBoard) checkPrMerges(ctx context.Context, entry *gitsession.RepoEntry, branches []model.AdeTaskBranch, facts *repoResult) map[string]int64 {
	merged := map[string]int64{}
	now := b.deps.Now().UnixMilli()
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(boardFanOut)
	for _, br := range branches {
		wb, ok := facts.branches[br.ID]
		if !ok || br.Name == "" || br.Kind != model.AdeBranchKindMine || wb.MergedAt != nil || wb.Tip == "" {
			continue
		}
		g.Go(func() error {
			r := entry.ResolveBranchChangeRequest(gctx, br.Name)
			if r.Kind == "ok" && len(r.ChangeRequests) > 0 && r.ChangeRequests[0].State == "merged" {
				mu.Lock()
				merged[br.ID] = now
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait() // ResolveBranchChangeRequest reports disabled/unavailable instead of erroring
	return merged
}

// countBranchRefsChanged counts created branches plus main whose resolved tip moved across the fetch.
func countBranchRefsChanged(ctx context.Context, entry *gitsession.RepoEntry, branches []model.AdeTaskBranch, beforeTips map[string]string, remote string) (int, error) {
	after, err := entry.BranchInventory(ctx)
	if err != nil {
		return 0, err
	}
	afterTips := make(map[string]string, len(after))
	for _, r := range after {
		afterTips[r.Ref] = r.Tip
	}
	mainRefName, _, hasMain, err := entry.MainRef(ctx)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, br := range branches {
		if br.Name == "" {
			continue
		}
		if row, found := resolveQueuedRef(after, br.Name, remote); found && beforeTips[row.Ref] != afterTips[row.Ref] {
			changed++
		}
	}
	if hasMain && beforeTips[mainRefName] != afterTips[mainRefName] {
		changed++
	}
	return changed, nil
}

// ForcePush pushes one created branch with lease. A refusal (protected branch, no remote tip)
// returns as the result's error.
func (b *TaskBoard) ForcePush(ctx context.Context, branchID string) (adewire.ForcePushResult, error) {
	br, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return adewire.ForcePushResult{}, err
	}
	out := adewire.ForcePushResult{BranchID: branchID}
	if br.Name == "" {
		out.Error = &adewire.RemoteOpError{Kind: "Unknown", Message: i18n.T("backend.space.ade.branchNotCreated.body", nil)}
		return out, nil
	}
	mu := b.repoMutex(br.CodeRepoID)
	mu.Lock()
	defer mu.Unlock()
	entry, err := b.openRepo(ctx, br.CodeRepoID)
	if err != nil {
		return adewire.ForcePushResult{}, err
	}
	remote := forcePushRemote(ctx, entry, br.Name)
	pf, err := entry.PushPreflight(ctx, remote, br.Name)
	if err != nil {
		out.Error = &adewire.RemoteOpError{Kind: "Unknown", Message: err.Error()}
		return out, nil
	}
	res, err := entry.RunRemote(ctx, b.conn, gitsession.RemoteOpParams{
		Kind: "forcePush", Remote: remote, Branch: br.Name, ExpectedRemoteTip: pf.RemoteTip,
	}, gitsession.RemoteDeps{Askpass: b.deps.Askpass})
	if err != nil {
		out.Error = &adewire.RemoteOpError{Kind: "Unknown", Message: err.Error()}
		return out, nil
	}
	out.Error = remoteOpError(res.Error)
	b.notifyBoard()
	return out, nil
}

// lastFetchAt reads the worktree's own FETCH_HEAD: git writes it per linked worktree, not in the common dir.
func lastFetchAt(entry *gitsession.RepoEntry) *int64 {
	info, err := os.Stat(filepath.Join(entry.Summary.GitDir, "FETCH_HEAD"))
	if err != nil {
		return nil
	}
	v := info.ModTime().UnixMilli()
	return &v
}
