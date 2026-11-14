# P627 journeys: ADE, third pass (long chained sessions)

Continues `journeys-ade.md` and `journeys-ade-2.md` (ids continue). Conventions: `journeys-studio-connections.md`
head. Far ends as in `journeys-ade-2.md` head: `app.DB()` rows, `Repo.Git(...)`, `Bare.Git(...)`, worktree and
task folders, per-run `.mcp.json`, fake `claude` transcript, `fakehost.Forge.Calls()`, `notifysink`.

Every journey runs in seconds. Time is simulated: board `deps.Now`, `TaskBoardDeps.BranchRequestWait`,
`leases.Options{Now, TTL}`, `agentnotify.Cooldown` and `Deps.Now`, step `timeout` set to milliseconds, fake
`claude` scripted to finish, hang or crash on cue. "Hours later" means clock advanced, never waited.

Facts added this pass (CodeGraph reads):

- `ade/taskworkflow.go`: `SetTaskWorkflow` switches workflow and restarts at first step; task's runs and run logs
  go; same id is no-op; empty id clears workflow and position; refused "stop its running agents first" while a
  run is live; unknown id "workflow <id> not found"; step messages cleared.
- `ade/route.go`: result of finished run = reported result id, else step's `failed` result when process failed,
  `done` when it succeeded, else first result of right kind, else stop. Routes: `next`, `end`, `stop`, forward to
  later step, back to earlier step, self (`retry`). Outcome recorded `back` for loop, `done` for forward or end
  even on not-ok result, `failed` for stop or spent loop. Return limit note names step and limit. Back to step
  that never ran on that branch stops with "cannot send back: <step> did not run on <branch>". Loop resumes
  previous agent session of target step with send-back line when session known, else passes line as extra
  prompt. Loop before task's entry step moves entry to target. Loop rounds ordered per branch across steps.
- `ade/stagesessions.go`: stage move closes running interactive sessions of other steps, plus review agent when
  move leaves review step; branch sessions (no step) stay; session that exited meanwhile is skipped.
- `ade/proof.go`: agent attaches closed proof session by name (`report_artifacts` kind proof); user links by id,
  unlinks; link records current step id and name and source (agent or user); archived task refuses link writes
  but lists proofs; choices are closed sessions no task holds, newest first; link and unlink recorded as panel
  edits.

## K. Workflows and routing

### J-WF-010 From step library to finished task: authoring choices observed at run time

- Status: partial (`adeflow` workflow editor, snapshot, routing tests; chain new).
- Persona: tech lead introducing new team workflow, then first task through it.
- Pre: repo `svc` with bare remote; fake `claude` scripted per step; Runs script `lint` exists.
- Steps:
  1. Steps tab: create `plan` (agent, outside repo), `build` (agent, results `done` next, `failed` back to
     `plan`, return limit 2), `lint` (script ref), `judge` (smart_script, results `pass` end, `fix` back to
     `build`), `verify` (manual).
  2. Workflow tab: order plan, build, lint, judge, verify; save.
  3. YAML tab: add variable use in `prompt_extra`; set sandbox block on `build`; save.
  4. Create task T on this workflow with 1 branch; start `plan`.
  5. `plan` agent finishes with `done`; `build` starts automatically or by user (per stage settings).
  6. `build` agent exits non-zero without `finish_step`.
  7. Observe route: back to `plan` with note; `plan` resumes previous session with send-back line.
  8. `plan` done; `build` reports `failed` again (round 2), then third time.
  9. Fix script; `build` done; `lint` script fails (exit 1) with no `failed` result defined.
  10. Edit workflow while T stopped: add `failed` back to `build` on `lint`. T shows outdated badge.
  11. Apply new workflow version to T (switch workflow to same id: no-op; to other id and back).
  12. Run again from first step to `judge`; smart script answers `fix`; `build` reruns; `judge` answers `pass`.
  13. Task ends before `verify` (route `end`); move manually to `verify`; mark done.
  14. Rename step `lint` to `checks` in library; open T history.
- Oracle: step and workflow YAML on disk equal editors after each save (one atomic write); `ade_runs`: step 6
  run outcome `back` with result `failed`, note holds "sent back by build"; resumed `plan` run carries same
  agent session id (fake `claude` argv has `--resume <id>`) and prompt holds send-back line; third failure
  recorded `failed`, route `stop`, note names step `build` and limit 2; step 9 run `failed` route `stop` (no
  result fallback to loop); step 11 same id leaves runs untouched; other id deletes T's runs and logs and
  position is first step (rows gone from `ade_runs`), switching back does not restore them; variable expanded in
  fake `claude` prompt; sandbox flags present in argv of `build` only; history after 14 shows step names of
  snapshot run time.
- Variations: `SetTaskWorkflow` while run live: refused "stop its running agents first".
- Risk: workflow switch silently destroying run history (OQ-ADE-4: confirm before reset).

### J-RUN-030 Routing matrix on two branches with loops crossing entry step

- Status: partial (`adeflow` route tests per case; combined matrix new).
- Pre: task with branches A and B (two repos); workflow s1, s2, s3, s4; s3 results: `ok` next, `redo` back to s2
  (limit 3), `restart` back to s1, `skip` forward to s4, `halt` stop, `finish` end; task entered at s2 (s1 never
  ran).
- Steps:
  1. Run s2 on A and B; both done.
  2. s3 on A reports `redo`: s2 reruns on A only.
  3. s3 on B reports `restart`.
  4. Enter task at s1 (move back); run s1 on both; continue to s3.
  5. s3 on A reports `restart`: entry moves to s1; s1 reruns on A.
  6. s3 on B reports unknown result id `maybe`.
  7. s3 on A crashes (fake `claude` killed); on B times out (timeout 50 ms).
  8. s3 on A reports `redo` 3 more times.
  9. s3 on B reports `skip`; s4 runs on B.
  10. s3 on A reports `halt`; user resumes run; second click on Resume.
  11. s3 on A reports `finish`.
  12. Restart app mid s4 on B.
- Oracle (`ade_runs` rows per branch): 2 only A gets new s2 attempt, B untouched; 3 run `failed`, note "cannot
  send back: s1 did not run on <B branch>"; 5 `ade_tasks` entry step becomes s1, board status follows; 6 falls
  to `done`/`failed` rule by exit state, never crashes; 7 both map to `failed` result of step (or stop when
  absent); 8 fourth `redo` stops with return-limit note "3"; loop round numbers on A strictly increasing across
  s2 and s1 loops; 9 outcome `done` with route s4; 10 second Resume refused; 11 task done on A while B still in
  s4; 12 running run marked stuck "interrupted by restart", nothing auto-resumes.
- Risk: loop counter shared across branches; entry step drift.

### J-WF-011 Workflow and step edits under concurrency and bad input

- Status: partial (`adeflow` WorkflowSaveWritesAtomicallyOnce).
- Steps:
  1. Open same workflow in two windows; edit both; save both.
  2. Edit workflow YAML on disk with external editor while app editor is dirty.
  3. Save YAML with route to missing step, duplicate step ids, cycle without limit, empty workflow.
  4. Delete step used by 2 workflows and by running task.
  5. Delete workflow used by 3 tasks (one running, one archived).
  6. Import workflow file with unknown fields and CRLF line ends.
  7. Kill app during save (fixture hook between temp write and rename).
  8. Relaunch.
- Oracle: disk never holds invalid YAML; second save warns of newer version or last write wins with notice
  (OQ-ADE-5); step delete refused or references listed; running task continues on snapshot; tasks of deleted
  workflow keep snapshot and show missing-workflow state, start refused with reason; unknown fields preserved on
  round trip; after 8 file is old or new version whole, journal recovered, no temp leftovers.
- Risk: torn config write; dangling references.

## L. Stages, sessions, handover

### J-RUN-031 Stage moves with mixed sessions, handover drafts and late agent calls

- Status: partial (`adeflow` stage sessions, handover tests; chain new).
- Pre: task at `build` with: TUI session of `build`, TUI session of earlier step `plan`, branch terminal session
  (no step), headless run of `build` live, review agent session (task later in review).
- Steps:
  1. Open stage-move dialog to `review`: list of sessions that will close shows `plan` and `build` TUI, not
     branch session.
  2. `plan` session exits by itself while dialog open; confirm move with close-sessions on.
  3. Headless `build` run still live: move refused or run stopped first (pin); stop run; move.
  4. Handover note draft appears for `build` to `review`; edit draft; add 1 MiB user note; save.
  5. `build` agent (late) calls handover tool after move.
  6. Start review agent; forward 2 comments; move task back to `build` with close-sessions on.
  7. Move forward again with close-sessions off.
  8. From mobile client move step forward.
  9. Create second draft for same stage pair.
  10. Copy handover chain as text; open chain after restart.
- Oracle: `ade_sessions`: after 2 `build` TUI closed, `plan` skipped without error, branch session running; 5
  tool reply "The task moved on to review; nothing was recorded" and no row written; 6 review agent session
  closed, `build` sessions of `review`-foreign steps handled per rule; 7 nothing closed; 8 nothing closed
  (mobile never closes); 9 still one draft per task and stage; `ade_task_handovers` rows and order; clipboard
  text equals chain in order with authors; agent note over 16 KiB refused.
- Risk: closing user's branch terminal; lost draft on move.

### J-RUN-032 Agent's-eye session: every task tool in one run with invalid input interleaved

- Status: partial (`adeflow` agent tools; `dbmcpflow`).
- Persona: scripted agent (fake `claude`) inside headless run of repoless task.
- Time: `BranchRequestWait` 50 ms for timeout case.
- Steps:
  1. Agent lists task context; declares 2 repos; declares unknown repo.
  2. Requests 21 branches (refused), then 3 with one `existing: true` held by busy donor.
  3. User approves 2 of 3; agent retries the denied one; request times out unattended.
  4. Agent writes notes: empty, 16 KiB, 16 KiB plus 1 byte, with ANSI and NUL.
  5. Agent acquires lease `e2e`, acquires unknown queue, re-acquires `e2e`, releases with empty name.
  6. Agent reports artifacts: 21 in one call, relative URL, 2,049-char URL, duplicate URL, kind proof with name
     of open (not closed) proof session, then closed one, then one held by other task.
  7. Agent calls `create_task` (orchestrator off): refused. Orchestrator on: approval item; deny.
  8. Agent reads DB through Studio MCP with approval `prompt`; user approves; audit row links task.
  9. Agent calls `finish_step` with unknown result, then valid result with summary.
  10. Agent calls any tool after `finish_step`.
  11. Process exits; next step's agent starts; calls handover read.
- Oracle: each refusal is tool error text readable by agent (transcript), never transport failure; DB rows only
  for valid calls; branch rows 2, worktrees on disk 2, donor untouched; timeout ends request `denied` state and
  unblocks agent; lease table holder then empty; artifacts count and dedup per limits; proof attached once with
  step name of current stage, source agent; approval queue items resolved; run outcome uses reported result;
  post-finish calls refused or ignored without changing outcome (pin, OQ-ADE-6).
- Risk: invalid agent input corrupting task state.

## M. Proof, links, review

### J-PRF-020 Proof across stages, tasks and archive

- Status: partial (`adeflow` proof links; `proofflow`).
- Steps:
  1. Agent in `build` records proof session `login-fix` (3 steps with screenshots); closes it; attaches by name.
  2. User opens Details: proof listed under stage `build`, source agent; open proof window; step through.
  3. User records own proof `manual-check`; closes; links from choices list (newest first).
  4. Move task to `verify`; agent attaches `login-fix-2`; listed under `verify`.
  5. Link proof already held by other task.
  6. Unlink `manual-check`; it returns to choices; link to other task.
  7. Agent submits new revision of `login-fix` (pending flag); user reviews; accept.
  8. Copy proof summary; export screenshots to folder.
  9. Archive task; open history: proofs listed read-only; try link and unlink.
  10. Restore task; link again.
  11. Delete proof session files from disk behind app; open Details.
- Oracle: proof link rows with task, step id and name, source, session; panel-edit log has link and unlink
  entries with proof name and summary; 5 refused with reason; 9 writes refused on archived task, list still
  works; export folder holds one PNG per step, names ordered, bytes equal stored; clipboard summary exact; 11
  gap state, no crash.
- Risk: proof attached to wrong stage or task.

### J-REV-030 Review loop with rebase in the middle and comment drift

- Status: partial (`adeflow` review, review_sync; `gitflow`).
- Pre: task with stack of 2 branches, PR open on fake forge with 6 review comments (2 outdated, 1 on deleted
  line, 1 multi-line, 1 with suggestion block, 1 general).
- Steps:
  1. Enter review step: comments sync from forge; counts per file; outdated marked.
  2. Start review agent round: agent adds 5 comments (1 duplicate of human one).
  3. User edits one agent comment, dismisses one, forwards 4 to implementer.
  4. Implement run fixes 3; reports `fix` remaining.
  5. `main` moves; rebase stack with conflict in commented file; resolve; push with lease.
  6. Return to review: comments re-anchored or marked outdated; none lost.
  7. Uncommitted review: agent still editing; open working review; add comment on unstaged hunk.
  8. Agent commits; comment follows to commit or is marked outdated.
  9. Publish selected comments to forge; forge returns 422 for one (line gone).
  10. Retry; resolve threads; second round finds nothing: says so.
  11. Leave review step: review agent session closed.
  12. Copy all open comments as text.
- Oracle: `fakehost.Forge.Calls()`: comment posts once each (no duplicate on retry), exact body and position;
  DB comment rows keep origin (human, agent), state, forwarded flag; after rebase count before = count after;
  failed publish keeps comment local with error; bare remote tips rebased; clipboard lists comments in file and
  line order.
- Risk: duplicate publish; comments lost on rebase.

### J-LINK-020 Task links from tracker to merge: detect, classify, track, snapshot

- Status: partial (`adeflow` links, updates, artifacts).
- Time: board `deps.Now` advanced for update polling; fake tracker and forge answer at once.
- Steps:
  1. Create task from issue URL: tracker facts load (title, state); link classified `issue`.
  2. Paste 6 URLs at once: PR, commit, CI run, doc, duplicate PR URL with fragment, non-http.
  3. Agent reports artifacts for same PR (detected row vs stored row).
  4. Tracker issue changes state and title; advance clock; indicator shows update; open; mark seen.
  5. Forge PR gets new review and failing check; indicators.
  6. Tracker returns 401 (expired credential of driven CLI); then 500; then recovers.
  7. Open link in app (PR view) vs in browser.
  8. Edit link title; reorder; remove; undo remove if offered.
  9. Merge PR on forge; board shows merged; RecordMerge.
  10. Archive: change request snapshot stored; open history offline (forge unreachable).
- Oracle: `ade_task_artifacts` rows deduped by URL; detected row wins; invalid URL refused; failed source shows
  gap, not error dialog, and Kira never reads CLI token (no auth header built by Kira: guard G5); `Browser.
  Opened()` exact URL once; history shows snapshot title, state, URL without network (request log empty).
- Risk: credential handling; stale indicator.

## N. Board and task lifecycle at scale

### J-ADE-070 Board with 300 tasks: order, filter, deadlines, bulk moves, two windows

- Status: partial (`adeflow` board, order; ui `ade-v2-board`).
- Time: `deps.Now` moved across deadline boundaries and midnight.
- Pre: 300 tasks over 6 statuses, 40 with deadlines (past, today, tomorrow), 12 running, 30 archived.
- Steps:
  1. Open board: columns virtualized; counts per column.
  2. Filter by repo, text, status; combine; clear.
  3. Drag task from rank 40 to rank 2; keyboard reorder another (menu move up, to top).
  4. Reorder changes lease queue rank: waiting agent of moved task jumps ahead of lower one, holder kept.
  5. Advance clock past deadline: chip turns overdue without reload; notification once.
  6. Multi-select 10 tasks; move status; archive 3 (one with dirty worktree: refused for that one only).
  7. Second window: same board; move task there; first window updates; conflicting move at same time.
  8. Rename status in statuses editor; delete status holding tasks (reassign prompt).
  9. Search archived; restore one whose branch name is now taken.
  10. Quit with 12 running; relaunch.
  11. Board scroll position, filters, selected task after restart.
- Oracle: `ade_tasks.sort_order` dense and unique after every move; `ade_lease_queues` waiters order equals task
  order then arrival; holder unchanged; `notifysink` one overdue notice per task; bulk archive result lists
  refused task with blocker text, other 2 archived (worktrees gone); concurrent move ends in one consistent
  order in both windows; restore refusal names branch holder; 12 runs marked stuck after relaunch.
- Risk: order corruption under concurrent reorder.

### J-ADE-071 One task from idea to archive with every failure injected once

- Status: new.
- Persona: developer on bad day.
- Steps:
  1. Create task; repo gate: pick repo whose folder was moved on disk.
  2. Fix repo path; create branch: name collides with remote branch; pick new name.
  3. Worktree create: target folder exists non-empty; then disk full (fixture); then succeeds.
  4. Setup script fails (exit 2); read log; retry after fix; start run while setup still running (queued behind
     setup).
  5. Restart app while run is queued behind setup.
  6. Run launches after setup; fake `claude` binary missing; then auth probe fails; then ok.
  7. Agent requests second branch; user away: request times out (wait 50 ms).
  8. Git index lock present in worktree: commit by agent fails; lock removed; retry.
  9. Push: remote rejects non-fast-forward; fetch; rebase; conflict; abort; rebase; push with lease.
  10. Create change request: forge 500, then 422 duplicate, then finds existing PR.
  11. App DB busy during handover save (fixture lock): retried or clear error; note text kept.
  12. Archive while branch request dialog of another window is open.
  13. Verify nothing leaked.
- Oracle: each failure shown inline with cause and next action, no partial state: no worktree row without
  folder, no folder without row, no branch row for refused name; queued run survives restart with launch
  options intact and starts once; forge calls: one PR total; bare remote tip equals local; after archive: no
  worktrees, no task folder, no live sessions, branch request `abandoned`, history row present.
- Risk: orphan worktrees and half-created branches.

### J-ADE-072 Orchestrated epic: parent with children, leases, notifications, mobile approval

- Status: partial (`adeflow` orchestrator; `mobileflow`).
- Time: `agentnotify.Cooldown` 10 ms and `Deps.Now`; leases TTL through `Options.Now`.
- Steps:
  1. Parent task with orchestrator on; agent calls `create_task` 4 times: approvals appear; approve 3 from
     desktop, 1 from mobile.
  2. Child tries `create_task` (depth 1): refused.
  3. Children run in parallel; all need lease `e2e`; order by task order; user reorders children mid-wait.
  4. Child 2 holder goes silent (clock past TTL) while turn idle; sweep expires; next granted.
  5. Child 3 needs input: notification `needs-input`; user watching that task: suppressed.
  6. Child 1 finishes: mirrored into parent chain; notification `finished` once despite two Stop events.
  7. Parent agent calls `list_tasks` with 150 tasks present.
  8. Parent `NextPass` while child 4 still running: refused; after done: second pass spawns 2 more.
  9. Archive parent with live children: risk dialog lists them.
  10. From mobile: move child step, answer branch request, view notes.
  11. Second device connects during 10.
- Oracle: approvals resolved per item with answering source; child rows have parent id; lease grants order in
  DB equals expected under reorder; expired holder told `revoked`; `notifysink` kinds and counts exact; parent
  notes chain holds child finish entries; `list_tasks` returns 100 with truncation note; mobile second device
  gets Conflict close code, first stays; mobile step move closes no session.
- Risk: runaway task creation; notification storms.
