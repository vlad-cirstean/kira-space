# P627 journeys: ADE, second pass

Continues `journeys-ade.md` (ids continue). One scenario per journey. Conventions: `journeys-studio-db.md` head.
Far end: `app.DB()` rows (`ade_tasks`, `ade_task_branches`, `ade_runs`, `ade_sessions`, `ade_task_handovers`,
`ade_task_artifacts`, `ade_lease_queues`), `Repo.Git(...)`, worktree folders under `~/kira-space-wts`, task folder
`<home>/ade/tasks/<taskId>`, per-run `<home>/ade/runs/<runId>.mcp.json`, fake `claude` transcript, `notifysink`.

Code and architecture facts used (CodeGraph reads, `docs/ARCHITECTURE.md` 2503-2984):

- `ade/rebase.go`: rebase run has built-ins `Read, Edit, Write, Grep, Glob` plus validated git tool, no Bash;
  default timeout 20 min; refusals: not a live task branch, review task, not own branch, branch not created,
  merged; blocker `dirty` passes only with autostash; base change stored before run (`storeRebaseBase`), failed
  rebase keeps it; no-op rebase clears pending base and starts no run; `AbortRebase` refuses "stop the run first"
  while a run is live, "no rebase is in progress" otherwise; abort keeps base changed and logs "aborted by you".
- `ade/archive.go`: archive drains launches, refuses when any worktree remove is blocked ("cannot archive:
  worktree of <name> (<blocker>)"), abandons branch and task requests, stops runs and setups, closes terminals,
  removes worktrees (dirty: confirmed with folder base name), snapshots change requests, removes task folder.
  `ArchiveRisk` lists dirty entries and unmerged count per linked branch.
- `leases/leases.go`: one holder per queue, never preempted; repeat acquire keeps place and counts as heartbeat;
  rank = task order then arrival; unknown queue error lists queue names; `Release` with empty name leaves every
  queue; `ForceRelease` grants next and tells old holder `revoked`; `RequestRelease` only flags; removing a queue
  revokes holder and tells waiters `removed`; sweep expires holder silent past TTL (10 min) unless turn is busy.
- Run engine: run ending without `finish_step` fails "ended without finish_step"; restart marks running runs stuck
  "interrupted by restart", nothing auto-resumes; `ResumeRun` second click refuses; loop counts cumulative per
  task, step, branch against `max_returns`; task with no branch refuses start `E_TASK_NO_REPO`.
- Branch requests: 1 to 20 entries; one open request per task (concurrent calls merge); ends approve, deny,
  timeout 30 min, caller gone, archive, app close; unchecked items `denied`; phase 3 re-reads task.
- Shared branches: `existing: true` adopts; donor busy or mid git operation refuses before prompt and at apply;
  dirty files do not block; donor row shows detached with holder.
- Handover: draft at most one per task and stage; agent notes at most 16 KiB; call after stage moved replies "The
  task moved on to <name>; nothing was recorded"; user note row up to 1 MiB; stage move closes other-step
  terminal sessions when `closeSessions` set; mobile step move never closes sessions.
- Orchestrator: depth 1; `create_task` raises `task.create` approval; child finish mirrored into parent chain;
  `list_tasks` at most 100; `NextPass` only at `done` with no live run.
- Artifacts: 20 per call, 200 per task, title 200, URL 2048 absolute http(s); dedup by URL else kind+repo+ref;
  detected row wins over stored; failed source becomes gap, not error.

## H. Tasks and board

### J-ADE-040 Day plan from scratch to evening shelve

- Status: partial (`journeyflow` firstrun; UI `ade-v2-plan`).
- Persona: solo developer planning Monday.
- Pre: 2 registered repos, default workflow, empty board.
- Steps:
  1. Capture 6 tasks by quick capture (2 with pasted Jira link, 1 with GitHub issue link, 1 title of 300 chars
     with emoji, 1 blank title attempt).
  2. Drag 4 into today, order them; park 1 in Later, 1 in Backlog.
  3. Add repo and branch name to first; start step 1 session; work; move stage with notes.
  4. Change status set: rename "Doing", remove "Blocked" reassigning to "Todo".
  5. At evening shelve unfinished 2 to tomorrow; quit.
  6. Relaunch next morning (clock advanced).
- Oracle: `ade_tasks` 5 rows (blank refused with message, no row); order column matches drag order; tasks on
  removed status now carry reassign target; after relaunch yesterday's shelved tasks appear on new day in same
  relative order; notes row exists in `ade_task_handovers` state `note`; link classification stored (Jira key,
  issue number).
- Risk: order lost across day roll, status reassignment missing tasks in Later/Backlog.

### J-ADE-041 Archive with dirty, unmerged and locked worktrees

- Status: partial (`adeflow` archive tests; blocked variant new).
- Pre: task with 3 branches: A clean merged, B dirty with 2 unmerged commits, C worktree locked by hand.
- Steps: 1. Archive: dialog lists B dirty files and "2 unmerged", C blocked. 2. Confirm anyway. 3. Unlock C;
  archive again; confirm. 4. Open archived view. 5. Restore.
- Oracle: step 2 refused "cannot archive: worktree of C (locked)", nothing stopped or removed (B folder intact,
  sessions alive); step 3 removes all 3 folders, branches refs kept (`git branch --list`), B's commits reachable
  from ref, uncommitted files gone as warned; archived view shows change request snapshot offline; restore puts
  task in Later, worktrees not recreated until next start.
- Risk: partial archive (some worktrees removed, task still live).

### J-ADE-042 Archive while agent run, setup script and branch request are live

- Status: new.
- Steps: 1. Start headless step (fake claude sleeps). 2. Second branch running prepare script. 3. Agent calls
  `request_branches` (dialog open). 4. Press archive and confirm. 5. Try to start step during archive (second
  window).
- Oracle: run ends cancelled with archive note, setup cancelled, agent tool call returns abandoned, approvals
  dialog closes, start in step 5 refused; process list has no `claude` child; `<runId>.mcp.json` deleted;
  `ade_tasks.archived_at` set once.
- Risk: launch racing archive leaves orphan process in removed worktree.

### J-ADE-043 Restore task whose branch is now held elsewhere

- Status: partial (`adeflow` archived_test).
- Steps: 1. Archive task T1 (branch `feat/x`). 2. Task T2 adopts `feat/x`. 3. Restore T1. 4. Start T1's step.
- Oracle: restore reports branch held (`BranchHeldError` text names T2) or restores with row shown shared and
  detached (pin); never two live owner rows for one name (unique live-name index); start in step 4 follows
  shared-branch rule.
- Risk: unique index violation surfacing as raw SQL error.

### J-ADE-044 Two windows edit same task concurrently

- Status: new.
- Steps: 1. Window A edits title; window B edits notes and moves stage at same time. 2. A changes workflow while B
  starts run. 3. A archives while B types note.
- Oracle: both title and notes survive (field-level patch); workflow change during live run refused or deferred
  with message; B's note after archive refused "archived tasks refuse"; both windows converge after board event.
- Risk: whole-row overwrite.

## I. Branches, rebase, stacks

### J-ADE-050 Rebase stack of 3 branches with conflict, abort, retry

- Status: partial (`adeflow` rebase_test; real claude `ade_test`).
- Pre: task branches `a` < `b` < `c` created with worktrees; `main` advanced with change conflicting in `b`.
- Steps:
  1. Open Rebase on `a`: preview shows prompt, stack of 3 with onto refs, no blockers.
  2. Dirty file in `c` worktree: preview shows `dirty` blocker; tick autostash.
  3. Start. Fake agent rebases `a`, stops inside `b` with conflict, calls no finish.
  4. Run ends failed. Branch `b` shows rebase in progress. Press Abort rebase while a retry run is live.
  5. Stop run; Abort rebase.
  6. Rebase again; agent completes; push ticked.
- Oracle: step 4 abort refused "stop the run first"; after 5 `b` worktree has no `rebase-merge` dir, HEAD equals
  pre-rebase tip, run log has "aborted by you", outcome facts `aborted`; base stays changed and pending marker
  shown; after 6 `git merge-base` chain correct for all 3, dirty file in `c` restored from autostash, bare remote
  tips equal local, pending marker cleared.
- Risk: abort while agent still writes; autostash entry lost.

### J-ADE-051 Rebase refusals

- Status: partial.
- Steps: try Rebase on: review-task branch; teammate (non-mine) branch; draft row without created branch; merged
  branch; branch of task being archived; already up to date branch.
- Oracle: first five refuse with texts above, no run row; last returns no-op, `ade_runs` unchanged, pending base
  cleared, UI says nothing to do.
- Risk: run started for no-op wastes agent tokens.

### J-ADE-052 Rebase run tries Bash and git outside stack

- Status: new (tool limits read from code).
- Steps: 1. Fake agent in rebase run calls Bash. 2. Calls git tool with `-C /other/repo`. 3. Calls git `push` without
  push grant. 4. Calls git `rebase` inside stack worktree.
- Oracle: 1-3 denied with actionable text, nothing executed (other repo reflog unchanged, remote unchanged); 4
  executes.
- Risk: rebase agent escaping its worktrees.

### J-ADE-053 Agent requests 20 branches, user approves subset, task changes meanwhile

- Status: partial (`adeflow` TestBranchRequest).
- Steps: 1. Agent calls `request_branches` with 20 entries over 3 repos (2 same repo with bases stacked, 1 bad
  base, 1 existing name without flag). 2. Dialog lists items; failed ones shown with reason. 3. While open, user
  removes one repo from task in other window. 4. Tick 5, approve.
- Oracle: tool result: 5 created, unticked `denied`, bad base and existing-name failed with text, removed-repo
  entry handled by phase 3 re-read (created with re-added row or failed, pin); `git worktree list` has 5 new;
  rows origin `agent` with chip; second concurrent request merged, not "already waits" dead end.
- Variations: 21 entries refused before prompt; timeout after 30 min creates nothing; minimise dialog never
  answers.
- Risk: creating branches for state read before wait.

### J-ADE-054 Branch request survives 65 s but dies with session

- Status: partial (real claude `TestAdeBranchRequest`).
- Steps: 1. Agent requests; user waits 70 s; approves. 2. Second request; user closes agent session. 3. Third
  request; quit app.
- Oracle: 1 succeeds (MCP timeout 35 min); 2 dialog closes, nothing created; 3 nothing created, after relaunch no
  ghost approval item.
- Risk: orphan approval item approving into dead call.

### J-ADE-055 Two tasks share one branch (adopt), work alternates

- Status: partial (`adeflow` TestBranchAdopt).
- Steps: 1. T1 owns `feat/x` with dirty file. 2. T2 agent requests `feat/x` with `existing: true`. 3. Approve adopt.
  4. T2 commits. 5. T1 starts step (needs branch back). 6. T2 archived.
- Oracle: after 3 T1 worktree HEAD detached at old tip with dirty file intact, T2 worktree on `feat/x`; T1 row
  shows detached glyph naming T2; step 5 refused or prompts while T2 holds (pin); after 6 T1 reattaches at new
  tip with dirty file preserved or conflict surfaced.
- Variations: donor has open terminal session: refused before prompt.
- Risk: dirty work lost on detach or reattach.

### J-ADE-056 Rename and switch branch via agent tools

- Status: partial (`adeflow` branchrename, branchchange).
- Steps: 1. Agent `rename_branch` to protected name. 2. To name existing. 3. To valid name; approve. 4. Agent
  `change_branch` to other local branch; approve. 5. Check sessions.
- Oracle: 1, 2 refused before prompt; 3 `git branch --list` shows new name, upstream config moved, worktree
  folder unchanged; 4 row keeps id and folder, HEAD on other branch; running session ids unchanged.
- Risk: rename breaking session cwd.

### J-ADE-057 Force push from task panel with protected remote name

- Status: partial (`adeflow` agentwork).
- Steps: 1. Branch tracks protected remote branch. 2. Force push from panel: dialog with lease and confirm token.
  3. Remote moved meanwhile. 4. Confirm.
- Oracle: rejected by lease, bare unchanged, panel shows remote error with fetch hint; after fetch second attempt
  works.
- Risk: plain force used silently.

## J. Runs, stages, handover

### J-RUN-020 Workflow with loop back and max returns

- Status: partial (`adeflow` run engine tests).
- Pre: workflow implement, review (result `changes` loops to implement, `max_returns: 2`), done.
- Steps: 1. Run. Fake review returns `changes` 3 times.
- Oracle: `ade_runs`: implement attempt 1 fresh, attempts 2 and 3 launched with `--resume <same session id>`
  (transcript shows one session); third `changes` marks run failed with loop spent; counts cumulative after
  manual retry (still spent); progress strip shows returns 2/2.
- Risk: loop counter reset per attempt, new session per fix run losing context.

### J-RUN-021 Run ends without finish_step, with timeout, with crash

- Status: partial.
- Steps: 1. Fake agent exits 0 without calling finish. 2. Agent hangs past step timeout. 3. Agent killed (SIGKILL).
  4. Agent calls finish twice (failed then done).
- Oracle: 1 failed "ended without finish_step"; 2 failed timeout, process group gone (no child left); 3 failed,
  step takes failed result route; 4 last call wins (`done`); each case `<runId>.mcp.json` removed and token no
  longer accepted on loopback.
- Risk: stale MCP token usable after run.

### J-RUN-022 Quit during 3 parallel runs, relaunch, resume one

- Status: partial (`adeflow` recovery).
- Steps: 1. Three tasks running agent steps, one script step, one rebase. 2. Quit (confirm dialog lists live
  work). 3. Relaunch. 4. Resume agent run A twice quickly. 5. Retry script run. 6. Look at rebase branch.
- Oracle: after relaunch all five `stuck` with note "interrupted by restart"; nothing auto-starts; A's first
  resume queues attempt+1 with `--resume <agent session id>` and no `--session-id`, second click refused; script
  and rebase runs offer Retry not Resume; rebase facts re-verified (in-progress shown with Abort).
- Risk: double resume, resume of unknown session id (CLI exit 1) shown as success.

### J-RUN-023 Task without repo: start refused, then outside-repo step works

- Status: covered (`adeflow` TestTaskStartNeedsRepo, TestRepolessWorkflow); combined journey new.
- Steps: 1. Task with no branch: Run. 2. Workflow with `outside_repo: true` first step: Run. 3. Agent calls
  `declare_repos` and `request_branches`; approve. 4. Next step runs.
- Oracle: 1 refused `E_TASK_NO_REPO` with picker; 2 cwd is task folder; 4 cwd is new worktree, prepared before
  start; prompt of outside step contains no `{repo}`/`{branch}`.
- Risk: fallback to task folder for repo-aware step.

### J-RUN-024 Stage move with sessions to close, one already gone

- Status: partial (`adeflow` TestStageMoveSessions).
- Steps: 1. Sessions open: step1 terminal, branch session, review agent, headless run. 2. Kill step1 terminal
  process from OS. 3. Move stage with Close selected and notes.
- Oracle: move succeeds (gone session skipped); branch and headless stay; review agent closed only when leaving
  review; handover row `handed` with notes; closed session resumable by Take over.
- Variations: mobile step move leaves all sessions open.
- Risk: move aborted by already-dead session.

### J-RUN-025 Handover chain across 4 stages with edits and late agent call

- Status: partial (`adeflow` TestStageHandover).
- Steps: 1. Stage 1 agent calls `handover` (finished) with 10 KiB notes; prompt opens. 2. User dismisses; later
  moves stage from menu editing notes. 3. Stage 1 session (still open) calls `handover` again. 4. Stage 2 run
  finishes with summary. 5. Stage 3 manual: user types notes with code block, 17 KiB. 6. Open history; edit an old
  row; delete another.
- Oracle: step 3 reply "moved on ... nothing was recorded", no row; exactly one draft per (task, stage) at any
  time; step 5: handed row limit 16 KiB enforced with message, or note row 1 MiB (pin which applies to stage
  notes); next-stage prompt contains newest handed row only; history order and authors (user, agent, run) right.
- Risk: draft duplication, late call overwriting next stage.

### J-RUN-026 Notes chain: 1 MiB note, copy, selection, restart

- Status: partial (`adeflow` TestUnifiedNotes; ui `ade-v2-notes`).
- Steps: 1. Paste 1 MiB + 1 byte note. 2. Paste exactly 1 MiB. 3. Copy entry via copy icon. 4. Select text in
  read-only entry with mouse and Ctrl+C. 5. Restart; reopen.
- Oracle: 1 refused with length message, editor keeps text; 2 stored byte-identical (`length(body)`); clipboard
  equals markdown source (pin: markdown vs plain); selection copy equals visible text; untouched editor emits no
  write on open (row `updated_at` unchanged).
- Risk: normalization rewrite on open.

### J-RUN-027 Orchestrator spawns children over two passes

- Status: partial (`adeflow` TestOrchestrator*).
- Steps: 1. Orchestrator run calls `create_task` 3 times (1 invalid workflow, 1 unregistered repo). 2. Approve 1,
  deny 1. 3. Run child to done with handover. 4. Orchestrator `list_tasks`, `task_chain`. 5. Next pass. 6. Archive
  parent; open child.
- Oracle: invalid calls fail before any prompt; denied returns denial text; child has `parent_id`, first
  handover = brief (source `orchestrator`); parent chain gets `child` row on finish; next pass refused while a
  run is live, allowed at done; child shows `parentArchived`; child cannot itself call `create_task`.
- Risk: depth > 1, approval reuse.

## K. Review, proof, links

### J-REV-020 Review agent rounds with forwarded comments

- Status: partial (`reviewflow`; `adeflow` review).
- Steps: 1. Open review window for branch. 2. Add 8 comments (2 on deleted lines, 1 on renamed file). 3. Preview
  forward text. 4. Forward to agent session; agent pushes fix. 5. Round 2: 3 comments now outdated. 6. Finish
  review; close window; reopen.
- Oracle: forward text equals text delivered into terminal (pty capture) with paths and line refs; comments
  stamped sent only after forward; round 2 diff base = round 1 head; outdated flagged; window title follows
  branch rename; archive snapshot keeps comments.
- Risk: forward sent twice, comments stamped without send.

### J-REV-021 Uncommitted review of agent work in progress

- Status: partial.
- Steps: 1. Agent edits 30 files (1 binary, 1 deleted, 1 new 2 MB). 2. Open working changes view. 3. Agent keeps
  writing while diff tab open. 4. Open diff of file agent just deleted.
- Oracle: list equals `git status --porcelain` of worktree at refresh; diff tab refreshes or shows stale marker;
  deleted-file open gives clear state, not error toast; large file gated.
- Risk: stale diff reviewed as final.

### J-PRF-010 Validation proof lifecycle with rejected and re-submitted proof

- Status: partial (`journeys-studio-views` J-PRF; flows proof).
- Steps: 1. Agent submits proof with screenshots and command output. 2. `proof.review` approval appears; user
  rejects with note. 3. Agent re-submits. 4. Approve. 5. Archive task; open archived proof.
- Oracle: approval ids distinct per submission; rejection note delivered to agent tool result; files stored
  under task proof dir and readable from archived view; answering first item after second exists gives NotFound.
- Risk: approving stale proof.

### J-LINK-010 Links strip: paste, classify, edit, dedupe, open

- Status: partial (`adeflow` TestTaskArtifacts, TestTaskPanelOrigins).
- Steps: 1. Paste Jira URL, GitHub PR URL, GitLab MR URL, plain doc URL, `ftp://` URL, 3,000-char URL, same PR URL
  again with trailing slash and fragment. 2. Agent reports same PR via `report_artifacts`. 3. Host detects PR for
  branch. 4. Edit link kind; remove stored one; try remove detected one. 5. Click each pill.
- Oracle: `ade_task_artifacts` holds accepted rows only (ftp and over-long refused with reason); one PR pill
  (detected wins); origin session shown for agent row; remove of detected refused; `Browser.Opened()` exact URLs;
  host down gives gap chip with reason, list still renders.
- Risk: duplicates by URL variant (OQ-ADE-1: normalisation rule).

### J-LINK-011 Agent reports 201 artifacts

- Status: new.
- Steps: 1. Agent sends 21 in one call. 2. Sends 10 calls of 20. 3. Sends 1 more.
- Oracle: 1 refused whole (count limit text); after 2 exactly 200 rows; 3 refused with per-task limit; reply of
  `finish_step` lists at most 100 plus "more" line.
- Risk: partial insert on refused call.

## L. Leases and agents

### J-LEASE-001 Three agents contend for one queue across task reorder

- Status: partial (`leases_test`; UI new).
- Steps: 1. Queues `port-3000`, `e2e`. 2. Agent of T1 acquires `e2e`; T2, T3 queue. 3. User drags T3 above T2 in
  plan. 4. T1 releases. 5. T2 calls acquire again while waiting. 6. Leases tab watched throughout.
- Oracle: T3 granted (rank by task order, not arrival); T2 position 1; repeat acquire keeps place; grant message
  delivered to T3 session when idle (pty capture) or at next Stop hook; cards show `Holds e2e` / `#1 in e2e`.
- Risk: arrival order used; grant never delivered to busy session.

### J-LEASE-002 Holder dies, goes silent, or is force-released

- Status: partial.
- Steps: 1. Holder session SIGKILLed. 2. New holder idle 11 min with no tool calls. 3. Holder mid long tool run
  12 min. 4. User Request release; then Force release (confirm). 5. User deletes queue with holder and 2 waiters.
- Oracle: 1 lease freed at terminal-gone reconcile without waiting TTL; 2 expired, next granted, old holder told
  `expired`; 3 not expired (turn busy); 4 request only flags (holder unchanged, message delivered), force grants
  next and tells old `revoked`; 5 holder `revoked`, waiters `removed`, `ade_lease_queues` row gone.
- Risk: lease stuck forever after hard kill.

### J-LEASE-003 Leases across app restart

- Status: new.
- Steps: 1. Holder plus 2 waiters. 2. Quit, relaunch. 3. Sessions resumed call `lease_status`.
- Oracle: queue names persist; holds and waits gone (memory only); agents told nothing held; UI shows empty
  queues; resumed agent that believes it holds must re-acquire (OQ-ADE-2: notify on resume).
- Risk: two agents both believing they hold.

### J-HAR-010 Take over headless session, send, restart session

- Status: partial (`adeflow` sessions).
- Steps: 1. Headless run finished. 2. Take over: terminal opens with `--resume`. 3. Type message over 1 MiB limit;
  then valid. 4. Restart session. 5. Close session; reopen from Sessions list; mark seen.
- Oracle: launch argv has resume id and recorded cwd; over-limit refused "message too long"; unseen marker clears
  once; session row states follow (running, stopped).
- Risk: take over in wrong cwd after worktree moved.

### J-ADE-060 Full feature day: 6 hours, 3 tasks, restart, review, merge

- Status: new.
- Persona: developer running agents in parallel.
- Steps:
  1. Morning: create T1 (2 repos), T2, T3 from issue links; plan order T1, T2, T3.
  2. T1 step plan (outside repo) runs; agent declares repos, requests 3 branches; approve 2.
  3. T1 implement runs headless; T2 implement runs; both need `e2e` lease; T2 waits.
  4. Laptop sleeps 20 min (clock jump); wake.
  5. T1 finishes; handover; review step: review agent round; forward 4 comments; fix run loops once.
  6. Force quit app mid T2 run. Relaunch. Resume T2.
  7. `main` moved: rebase T1 stack with push; conflict; abort; rebase again.
  8. Create change requests from panel; links appear; copy PR URL.
  9. Mark merged (RecordMerge) for T1 branch A; archive T1 (branch B unmerged, dirty): read risk, cancel; commit;
     archive.
  10. Evening: usage view shows tokens per task; history shows T1.
- Oracle: DB: runs with attempts and outcomes in order; sessions with resume ids; leases granted T1 then T2 with
  no expiry of busy holder across sleep (pin: sleep jump vs TTL, OQ-ADE-3); bare remotes hold rebased tips;
  `fakehost.Forge.Calls()` one create per branch; clipboard PR URL; worktrees of T1 removed, T2/T3 intact;
  history row for T1 with snapshot.
- Risk: clock jump expiring leases and timeouts; resume after kill; archive data loss.
