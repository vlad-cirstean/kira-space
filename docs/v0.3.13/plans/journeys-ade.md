# P627 journeys: ADE (board, tasks, branches, workflows, runs, sessions, agents, harness)

Imagined usage and definition of correct. Conventions: `journeys-studio-connections.md` head. Inputs:
`inventory.md` sections 24-27, 30; `coverage-map.md` (adeflow, harnessflow, claudeflow, usageflow); source read
through CodeGraph (`ade/branchrequest.go`, `internal/approvals/queue.go`).

Far ends: worktree and bare remote git state, files under `<home>` (workflows, steps, task folders, agent launch
files), app DB rows (`app.DB()`), fake agent records in `FakeDir`, `fakehost` calls, PTY bytes, clipboard spy,
state after `app.Restart()`.

## A. Board and plan

### J-ADE-001 Plan a week from empty board

- Status: partial (`adeflow` NewTaskLandsTodayWithoutRepos, Shelves, EstimateSetChangeClear; ui `ade-v2-plan`).
- Persona: developer on Monday morning.
- Steps:
  1. Add 6 tasks via Add popover: 2 new (today), 1 "next work day", 1 from existing branch, 1 from GitHub PR link
     (fork PR), 1 orchestrator kind.
  2. Set estimates 2h, 1d, clear one. Drag tasks across day bands, onto weekend hatch, into Later, into Backlog.
  3. Mark Wednesday off day: its tasks move. Toggle history.
  4. Restart app. Advance clock 8 days (`WithClock`).
- Oracle: board read after restart equals pre-restart plan (day, order, estimate, shelf per task id); after clock
  jump tasks beyond 7-day window sit in Later, overdue unfinished tasks roll to Today (assumption; pin rule);
  fork PR task has branch `<owner>/<head>` fetched (`git rev-parse` in repo); phone `Plan` endpoint returns same
  order.
- Variations: 300 tasks; drag while board push arrives (coalesced `BoardChangedEvent`); two windows drag same
  task; DST change day; weekend start per locale.
- Risk: order lost on restart, drag vs push race.

### J-ADE-002 Keyboard-only task handling

- Status: partial (ui `ade-v2-task-menu`).
- Steps: focus card with Tab/arrows; Shift+F10 menu; move to Later; set status; archive (confirm, Escape cancels);
  copy title, id, branches, session id; open Jira/GitHub.
- Oracle: clipboard per copy action exact (id with prefix handle, branches newline-separated, assumption);
  `Browser.Opened` URLs; cancelled archive changes nothing.
- Risk: menu anchor off-screen (P461, P601), copy wrong field.

### J-ADE-003 Archive, find, restore

- Status: partial (`adeflow` ArchivedTaskKeepsData, ArchiveRisk, ResumeArchivedSession; ui `ade-v2-archived`).
- Steps: archive task with dirty worktree and unpushed commits (risk confirm lists both); find in archived finder
  by partial title; open read-only (notes, sessions, logs); restore; meanwhile another task took branch name
  (restore refused); rename; restore.
- Oracle: archive keeps run/step logs and notes (DB rows, log files); worktree removal only as confirmed (`git
  worktree list`), unpushed commits still reachable from branch ref; restore refusal names live branch; restored
  task lands on board with history intact.
- Risk: data loss on archive, branch collision.

### J-ADE-004 Task statuses editor and manual move

- Status: partial (`adeflow` TaskStatuses; ui `ade-v2-task-status`).
- Steps: Workflows page Statuses: add "Blocked", reorder, delete status in use (reassign required); set task to
  "Blocked" manually; run step completes (status follows step slot again).
- Oracle: statuses persisted; deleted status absent from every task (DB); manual status sticks until step change
  exactly.
- Risk: orphan status ids.

### J-ADE-005 Conflict pairs and repo updating chip

- Status: partial (`adeflow` QueueMergeTree, RepoUpdatingWhileGitBusy; ui `ade-v2-pair-check`).
- Steps: two tasks touch same file lines; pair chip shows conflict; one merges to main; other shows behind and
  "merged/stale" via patch-id after squash-merge; long fetch shows repo updating chip.
- Oracle: pair result equals `git merge-tree` of the two heads; stale detection equals `git cherry` output;
  rename/delete conflicts (open item) pinned.
- Risk: false "no conflict".

## B. Tasks, branches, worktrees

### J-ADE-010 Task across two repos from start to merged

- Status: partial (`adeflow` TaskReposAndBranches, Branching, AgentCommitReachesBoard). New: full chain to remote
  and PR.
- Persona: developer shipping API plus web change.
- Pre: repos `api`, `web` with bare remotes, fake host, workflow implement -> review -> finish.
- Steps:
  1. Create task "Add export"; add repos api and web; branch names suggested; base `main`.
  2. Start: worktrees created, prepare scripts run (web fails first: reason on card; fix script; retry).
  3. Agent (fake) commits in both worktrees, calls `report_artifacts` with PR URL.
  4. Push from task; PR line and checks glyph appear from host index.
  5. `main` moves: Change base / Rebase run; conflict in web: Abort rebase; retry after agent fix; force push
     (with lease).
  6. PR merged on host; task shows merged; finish; archive.
- Oracle: `git worktree list` in each repo; `Bare.Git rev-parse refs/heads/<branch>` equals worktree HEAD after
  push and after force push; rebase abort leaves branch at pre-rebase sha; artifacts list has PR and commits;
  after merge board marks merged via host state not local guess.
- Variations: branch name with `/`, unicode, 200 chars, existing remote branch of same name; repo removed from
  task while in use (refused); worktree checked out to other branch in terminal (mismatch chip P617).
- Risk: late-branch attach (P397), force push without lease.

### J-ADE-011 Agent branch requests: create, adopt, rename, change

- Status: partial (`adeflow` BranchRequest, BranchAdopt, BranchRename, BranchChange, BranchRequestsTwoTasks; ui
  `ade-v2-branch-*`).
- Steps:
  1. Repoless task starts; agent `declare_repos` then `request_branches` for 3 repos with one optional base and one
     existing checkout to adopt.
  2. User unticks one repo, approves. Agent `rename_branch` with remote rename ticked. Agent `change_branch`.
  3. Second task requests same branch name concurrently.
  4. User ignores a request past wait timeout (`WithBranchRequestWait`).
  5. Undo adopt.
- Oracle: worktrees exist only for approved repos; adopted checkout untouched content-wise (`status` equal before/
  after), undo restores donor; remote branch renamed on bare (`ls-remote`) only when ticked; name collision gives
  second task a distinct name or refusal, never two tasks on one branch unless shared-branch rule (P583) applies;
  timeout returns clear tool error, item removed from queue.
- Risk: name collision (P199, P406), unseen approval.

### J-ADE-012 Multiple worktrees per repo and shared branches

- Status: partial (`adeflow` MultiWorktree; ui `ade-v2-multi-worktree` 1 test).
- Steps: task with two branches in same repo (two worktrees); second task shares one branch (detached worktree);
  remove one branch while session runs there (refused); remove after stop with dirty confirm.
- Oracle: `git worktree list --porcelain` matches board; detached worktree HEAD at branch tip; removal deletes
  folder and prunes; run history kept after detach (`TaskBranchDetachKeepsRunHistory`).
- Risk: removing worktree under live session.

### J-ADE-013 Repoless task and repo gate

- Status: covered (`adeflow` RepolessWorkflow, OutsideRepoStep, TaskStartNeedsRepo; ui `ade-v2-norepo`,
  `ade-v2-repo-required`); extend: files.
- Steps: start repoless task with outside-repo step; agent writes file in `<home>/ade/tasks/<id>`; start a
  repo-needing step: gate dialog; add repo; continue; archive; restore.
- Oracle: file exists under task folder after restart and after archive/restore; `E_TASK_NO_REPO` over phone API
  is 409.
- Risk: task folder deleted on archive.

## C. Notes, links, details

### J-ADE-020 Notes chain as working log

- Status: partial (`adeflow` UnifiedNotes, StageHandover; ui `ade-v2-notes`, `ade-v2-handover`).
- Steps:
  1. Write note with Markdown (table, code fence with backticks, task list, 30 KiB, emoji, `<script>`); draft kept
     when switching task and back, and across module switch; not across restart (assumption: draft store is
     renderer; pin).
  2. Save. Agent handover lands in same chain. Select text across two entries with mouse, Cmd+C; copy per entry.
  3. Edit note while TUI agent runs: send/discard bar; Send forwards diff.
- Oracle: clipboard per-entry copy equals stored Markdown source; mouse selection copy equals rendered text; DB
  note row equals input bytes; agent PTY received diff text once; HTML not executed.
- Variations: headless run (no forward, stays pending until Take over); two edits before send (one combined diff).
- Risk: notes not selectable (P582), forward duplicates.

### J-ADE-021 Links: add, classify, open in app or browser

- Status: partial (`adeflow` detailslinks, DetailsOpenExternal, HostLink; ui `ade-v2-details-links`,
  `ade-v2-links-strip`).
- Steps: add links: Jira key `ABC-1`, Jira URL, GitHub PR URL with `/changes`, GitHub issue, Azure PR, plain URL,
  `file://` path, malformed text; edit one; remove one; click in strip (in-app view), Cmd-click (browser); copy
  link card.
- Oracle: groups Issues/Change requests/Misc per classifier; in-app click opens host-view/issue window
  (`WindowManager.OpenedKeyed`), Cmd-click `Browser.Opened` with exact URL; non-Jira URL never in Jira slot;
  malformed input refused or stored as Misc text, never crashes.
- Risk: misclassification (P503, P594).

### J-ADE-022 Tracked updates indicators

- Status: partial (`adeflow` TrackedUpdates, TrackedIssueBatch; ui `ade-tracked-updates`).
- Steps: task tracks PR and Jira issue; new PR comment, check failure, issue status change, assignee change arrive;
  open task (indicator clears per concept); turn tracking kind off in settings.
- Oracle: indicator set equals changes fed to fakes; seen state persisted across restart; off kinds raise none;
  Azure comment/assignee (open) pinned.
- Risk: indicator never clears or clears all.

### J-ADE-023 Proof chain and DB audit in Details

- Status: partial (`adeflow` ProofChain, DbAuditTrail; ui `ade-v2-proof-chain`, `ade-v2-db-audit`).
- Steps: step with `db_mcp` runs 3 queries (one denied, one masked); agent `attach_proof`; open Details: audit rows,
  proof entry; open proof window from chain; copy audit statement.
- Oracle: `ade_db_audit` rows: 3, statements masked form, codes only for errors; clipboard statement has no
  masked literal; proof opens same session id.
- Risk: audit leaks values.

## D. Workflows and steps

### J-WF-001 Author workflow in three editors

- Status: partial (`adeflow` WorkflowsYaml, workflow_editor, StepLibrary, WorkflowSaveWritesAtomicallyOnce; ui
  `ade-v2-workflows`, `ade-v2-workflow-steps`).
- Persona: tech lead defining team flow.
- Steps:
  1. Steps tab: create steps "plan" (agent), "build" (script ref to Runs script), "check" (smart_script), "verify"
     (manual), "review" (review). Set sandbox block, `db_mcp`, `prompt_extra`, `max_returns` 2.
  2. Workflows tab graph: connect ok/not-ok routes, loop limit; try route to skipped step (refused).
  3. Switch to YAML tab: edit; introduce error; Save disabled or last-valid kept; fix; Save.
  4. Rename step: file renamed with journal; workflows referencing follow.
  5. Switch module with unsaved edits (open item: lost; leave guard assumption: none; pin).
- Oracle: `<home>/config/workflows/*.yaml` and `<home>/steps/*.yaml` equal editor state; one atomic write per save;
  rename journal entry present; graph layout stable after reload; invalid YAML never replaces last valid on disk.
- Risk: stale references after rename, invalid route accepted (P302).

### J-WF-002 Edit workflow while task runs on it

- Status: partial (`adeflow` snapshot; outdated badge).
- Steps: start task on workflow v1; edit workflow (add step); task shows outdated badge, continues on snapshot;
  new task uses v2; add harmless wire field (no false outdated).
- Oracle: running task step list equals v1 snapshot (DB); second task v2.
- Risk: snapshot hash false positive (P510).

## E. Runs, stages, sessions

### J-RUN-001 Headless run through all outcomes

- Status: partial (`adeflow` RunLifecycle, RunOutcomes, ReturnLimit, StageHandover; ui `ade-v2-run`).
- Steps:
  1. Start step: fake agent `needs_input`; reply via stage dialog; agent `failed`; Retry; agent `done` with handover.
  2. Stage dialog: handover notes, Keep open vs Close; Next stage.
  3. Review step sends back (not ok route) twice; third hits `max_returns`.
  4. Take over (TUI resume) mid-run; finish by hand; Finish.
- Oracle: run rows and step outcomes in DB match sequence; agent launch args (file in `FakeDir`) hold workflow
  prompt, `prompt_extra`, `MCP_TOOL_TIMEOUT=2100000`, per-session `--settings` temp path; user `~/.claude/settings`
  byte-identical; handover text in notes chain; return limit stops loop with clear state.
- Variations: `claude` missing (pre-launch detection), exits non-zero, `nofinish`, `lost` conversation.
- Risk: previous-stage session counted as talking (P469).

### J-RUN-002 Quit and crash mid-run, resume

- Status: partial (`adeflow` ResumeAfterRestart, ResumeLostConversation, ResumeRefused; `journeyflow`
  RestartMidRun).
- Steps: 2 headless runs and 1 TUI session live; Quit: confirm "3 agents running"; Cancel; Quit again, confirm;
  relaunch: resume offers; resume one, dismiss one; SIGKILL app mid-run; relaunch.
- Oracle: after quit no agent processes remain; resumed run continues same conversation id (fake record); dismissed
  run marked stopped, not running forever; after SIGKILL stuck runs detected and offered; no duplicate resume when
  pressing Resume twice.
- Risk: concurrent Take over/Restart double resume (P394).

### J-RUN-003 Sessions menu and TUI lifecycle

- Status: partial (`adeflow` InteractiveSessions, RestartSession, CloseSession, AdHocSession; ui
  `ade-v2-sessions*`).
- Steps: open 6 sessions (TUI, headless, ad hoc per worktree); filter menu; tooltip state/age; Restart; Close; Copy
  session id; Copy log over 600 chunks; subagent keeps status running; finished unseen marker clears on view.
- Oracle: clipboard log equals full PTY/log text all chunks in order (length and hash vs file); session id copied
  equals provider id; closed session process gone; status bar counts equal menu.
- Risk: copy log stops at first page (v0.3.5).

### J-RUN-004 Type into TUI: paste, newline, links

- Status: partial (ui `terminal-links`; mobile terminal).
- Steps: in agent TUI paste 3-line text (bracketed paste), Shift+Enter newline, Cmd+click URL and OSC 8 link,
  Cmd+click `file.go:10:2`, select output and copy, resize pane narrow/wide, scroll back 5,000 lines.
- Oracle: PTY received exact bytes (`ESC CR` for Shift+Enter, paste brackets); `Browser.Opened` URL; file opened at
  10:2; clipboard equals selected terminal text without trailing padding.
- Risk: Shift+Enter/Cmd+Enter bytes, link open no-op in webview.

### J-RUN-005 Leases between competing tasks

- Status: partial (`adeflow` Leases; ui `ade-v2-leases`).
- Steps: queue "staging-db"; task A books; B and C wait ranked by task order; reorder board so C above B; A idle
  past 10 min TTL; Force release; A session wakes via `SendIfIdle`.
- Oracle: holder sequence A, C, B; expired lease released exactly once; wake text delivered only to idle session
  (PTY record), not mid-turn.
- Risk: double grant, wake while working.

### J-RUN-006 Orchestrator with children

- Status: partial (`adeflow` Orchestrator*; ui `ade-v2-orchestrator`).
- Steps: orchestrator agent `create_task` x3 (approval each: approve 2, deny 1); children run; child mirror
  read-only; next pass after children finish; archive a child mid-pass.
- Oracle: board has 2 children linked; denied create returns denial to agent; orchestrator pass input lists child
  outcomes; archived child shows by step name.
- Risk: unapproved task creation.

### J-RUN-007 Script and smart_script steps with Runs scripts

- Status: partial (`adeflow` AutomationStep, RunOutcomeAutomation, SmartRunSandboxed).
- Steps: step kind script referencing Runs script with secret param; exit 1 routes not-ok; smart_script outcome
  blocked; edit Runs script file while step queued; delete referenced script.
- Oracle: script ran in worktree cwd with ADE variables (env dump file); secret masked in step log; missing
  script gives clear failure at start, no hang; outcome routes per workflow.
- Risk: dangling script ref.

### J-RUN-008 Background ops feedback

- Status: partial (ui `ade-v2-background-ops`, `ade-v2-op-kinds`; toast-led).
- Steps: close task with 3 branches (background branch removal); press same action twice (singleflight); slow git
  (>2 s) shows chip then toast; failure on one repo; agent cancels operation via MCP.
- Oracle: far end missing in UI specs: `git branch --list` and remote `ls-remote` after op match intent; failed
  repo reported by name, others done; one op row per logical op.
- Risk: toast says done while branch remains.

## F. Harness, providers, settings

### J-HAR-001 Switch provider and settings, verify every launch site

- Status: partial (`harnessflow` AgentSettingsReachEveryLaunch, AgentContextReachesEveryLaunch, OpenCode*).
- Steps: set shared context (30 KiB, near cap), common settings (env var, MCP server with credential), provider
  Claude then OpenCode; launch from: ADE step, ad hoc session, smart script, review agent, proof, terminal agent
  tab; toggle hermetic; set per-step agent key.
- Oracle: launch files in `FakeDir` per site contain context text and settings; MCP credential not in any file as
  plaintext except provider-required env at launch (pin), stored sealed in `secret_values`; user config files
  byte-identical; context over 32 KiB refused at save with count.
- Risk: one launch site missing settings.

### J-HAR-002 Import/export agent config

- Status: partial (`harnessflow` AgentPortClaude/OpenCode; ui `ade-agent-port`).
- Steps: import from user Claude config with hooks, MCP servers with secrets, statusline; preview masked diff;
  apply; export back to provider after edits; backups listed; undo via backup.
- Oracle: backup files exist with original bytes; exported config parses and equals settings; secrets masked in
  preview and never written to `settings.yaml` in clear.
- Risk: secret in diff or YAML.

### J-HAR-003 Hooks drive status across app

- Status: partial (`notifyflow`, `usageflow`, `adeflow` SessionStatusBackgroundAndUnseen).
- Steps: session emits PreToolUse/PostToolUse pairs, Notification (needs input), Stop, SubagentStop, background
  task; watch status bar counts, card indicator, keep-awake (with agents), quit confirm count, usage item.
- Oracle: `KeepAwakeDriver.Held()` true only while agents work; counts equal sessions in each phase; Stop clears
  tool name; subagent stop does not flip main session; usage percent from status line payload survives restart.
- Risk: stuck "working", keep-awake never released.

### J-HAR-004 Sandbox policy per step

- Status: partial (`adeflow` Sandbox* with fake enforcer; real only in `tests/claude`).
- Steps: step sandbox read-only folders plus worktree writes; agent tries write outside (denial surfaces with
  path); network off; prepare fails; manual session under sandbox; toggle default off.
- Oracle: generated profile file lists exact paths (worktree, read-only dirs) and denies; denial shown on step;
  with fake `NotEnforced` UI states "not enforced" plainly.
- Risk: profile omits worktree of second repo.

## G. Review step and capture into ADE

### J-ADE-030 Review-kind task from captured PR

- Status: partial (`adeflow` CaptureChangeRequestForReview, ReviewPlanDay, reviewconvert; `captureflow`).
- Steps: copy PR URL; hotkey; Enter (Later); drag to today; open review; convert review item to work item; start
  work on PR branch.
- Oracle: review branch fetched (`rev-parse` equals PR head from fake host); convert keeps links and marks; unknown
  repo capture yields partial-outcome notice and task without branch.
- Risk: PR head attached wrong repo.

### J-ADE-031 Review step loop with implementer

- Status: partial (`adeflow` ReviewStepWaits, ReviewSendImplementer, ReviewSendNew, ReviewFinish,
  ReviewQuitResume).
- Steps: review step waits; user comments; send to implementer session (same) vs new session; implementer commits;
  review shows changed-since-review; quit mid-round; resume; finish.
- Oracle: commits on branch by fake agent; comments marked sent; after resume round state equal; finish advances
  stage once.
- Risk: comment loss on quit.
