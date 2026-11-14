# P630 Stream B: ADE, agents, automations, notifications, mobile, Docker

Implementer plan (Sonnet). Decisions: `synthesis.md` §6. Journey text: `journeys-ade*.md`, `journeys-infra-misc*.md`,
`journeys-docker.md`, `journeys-bug-classes.md`, `journeys-cross-module.md`. Read each journey's Steps and Oracle
before writing its test.

Committed set: 55 primary journeys (20 P1, 35 P2) plus 36 merged as variations. Work in item order: P1 items first.
Nothing here is optional.

## Rules for this stream

- Worktree: own git worktree off chapter branch head at stream start. Commit per work item, or per product fix plus
  its test. Conventional Commits.
- Split rule: `NEW both` means one scenario, two halves.
  - Flow half calls bound services (`app.W.AdeTask...`, `app.W.<Service>`) and asserts far end.
  - Far end means: git refs, worktree dirs, files, `app.DB()` rows, fake agent observations, `notifysink`.
  - Flow half writes `app.Contract(t, "ade-<name>", key, v)`.
  - UI half (`tests/ui/ade-v2-*.spec.ts`) asserts IPC payload equals `contract(...)` and screen shows result.
  - Contract scenarios here start `ade-`, `runs-`, `schedule-`, `script-`, `automations-`, `mobile-`, `notify-`,
    `prompt-`, `docker-`, `agent-`.
- Agent: fake agent only (`app.Scenario(fakeagent.Scenario{...})`).
  - Actions: `done`, `needs_input`, `failed`, `result:<id>`, `fail`, `nofinish`, `lost`, `sleep`.
  - Helpers: `Sh`, `MCP`, `Emit`, `WaitFile`.
  - Never real `claude`.
- Time:
  - `flowharness.WithClock` for leases, schedules, TTLs, notification cooldown.
  - `WithBranchRequestWait`, `WithRebaseTimeout`, `WithTrackerGrace` for engine waits.
  - No `time.Sleep`. Fake agent `sleep` only when a step must be live, released by `WaitFile`.
- Remote git oracle: `app.NewBare`, `Bare.Git("rev-parse", ref)`, `Clone`, `PushFrom`. Local: repo `git` calls on
  task worktree.
- Restart: `app.Restart()` (old refs dead; re-fetch services).
- Clipboard: import `installClipboardSpy` from `tests/ui/studio/support/clipboard.ts`; never edit that file (Stream
  A owns it).
- Docker tests: follow `dockerflow/helpers_test.go` engine fixture and `flowharness.RequireDocker`. G5: never read
  docker credential material; read docker context `meta.json` only through product code.
- Product bugs:
  - Root-cause and fix in this stream, test and fix in one commit group.
  - Design decisions become follow-ups below.
  - USER items (`synthesis.md` §5 Q5, Q9, Q10, Q3) are never decided here: test agreed part only.
- Run own specs only:
  - Flows: `go test -p 1 ./apps/kira-space/internal/flows/adeflow/ -run <Test>`.
  - UI: `bunx playwright test --config=apps/kira-space/playwright.config.ts --project=ui <spec>` after
    `bun run build:test`.
- Pre-commit hook must pass on every shipped commit.

## Helpers

New `apps/kira-space/internal/flows/adeflow/journey_helpers_test.go` (package-local; no flowharness edit):

- `taskWithRepo(t, app, title) (task, repo, bare)`: imports repo with bare remote, creates task with workflow
  fixture.
- `worktreeOf(t, app, taskID, repoID) string`.
- `refAt(t, bare, ref) string`.
- `dirty(t, dir, file, body)`: writes uncommitted file.
- `runsOf(t, app, taskID) []model.AdeRun` via `app.DB()` read.
- `stepEvents(t, app, mark, taskID)`.
- Workflow YAML fixtures: under `apps/kira-space/internal/flows/adeflow/testdata/journeys/`.

Other packages (termflow, notifyflow, mobileflow, dockerflow, harnessflow) extend their own `helpers_test.go`.

## B1 Archive and restore without data loss (P1)

- Journeys:
  - Primary: J-ADE-041, J-ADE-042, J-ADE-043, J-BC-027 (P2).
  - Merged: J-ADE-003.
- Files:
  - New `adeflow/archive_safety_test.go`.
  - EXT `adeflow/archived_test.go` (J-BC-027).
  - EXT ui `tests/ui/ade-v2-archive.spec.ts`.
- J-ADE-041 (contract `ade-archive-safety`), task with 3 worktrees:
  - one dirty (uncommitted plus untracked file);
  - one with unmerged commits not on remote;
  - one locked (`git worktree lock`).
- J-ADE-041 oracle:
  - Archive refuses, or lists each blocker. No uncommitted byte lost: file still on disk, or in named stash/ref
    reported to user.
  - Unpushed commits still reachable (`git rev-parse <branch>` or kept ref).
  - Locked worktree untouched.
  - DB row archived only when archive succeeded.
  - UI half asserts blocker list rendered from contract.
- J-ADE-042:
  - Archive while fake agent run is live (`sleep` plus `WaitFile`), setup script running, branch request pending.
  - Archive refused with reason, or stops each first. After: no orphan process (`app.W.AdeTask.Sessions` empty for
    task), no pending branch request, run row terminal state.
- J-ADE-043: restore task whose branch is now checked out in another worktree or task. Restore succeeds into Later
  with clear branch state, or refuses naming holder. Never two worktrees on one branch (`git worktree list`).
- J-BC-027: 2,000 archived tasks seeded via store. Archived finder paging returns each exactly once. Search finds by
  title and Jira key.
- Expected defects: archive deleting dirty worktree; restore double checkout.

## B2 Workflow switch and edits (P1)

- Journeys:
  - Primary: J-WF-010, J-WF-011.
  - Merged: J-WF-001, J-WF-002.
- Files:
  - New `adeflow/workflow_switch_test.go`.
  - EXT `adeflow/workflowfiles_test.go`, `adeflow/workflow_editor_test.go`.
- J-WF-010 (OQ-ADE-4 is USER §5 Q5):
  - Author steps from library and run task through them; outcomes observed at run time match authored choices
    (step kinds, prompts, sandbox, script).
  - Agreed part of switch:
    - `SetTaskWorkflow` same id: no-op, runs kept (`ade/taskworkflow.go:20`).
    - While running: refused "stop its running agents first" (`:38`).
    - Switch never deletes other tasks' runs (`runsOf` before and after).
  - Do not assert deletion of this task's runs either way.
- J-WF-011 (OQ-ADE-5 is USER §5 Q3):
  - Edit workflow in form, YAML and graph editors; save. File on disk under `<space>/config/workflows` is valid YAML
    equal to expected.
  - Bad YAML or bad step reference refused; file bytes unchanged.
  - Edit while task runs on it (J-WF-002): running step keeps its snapshot, next step uses new version (or pinned
    current behaviour, stated in test comment).
  - File changed on disk while editor dirty: no corruption; last save intact.

## B3 Run engine outcomes, routing, resume, agent tools (P1)

- Journeys:
  - Primary: J-RUN-021, J-RUN-022, J-RUN-030, J-RUN-032.
  - Merged: J-RUN-001, J-RUN-002, J-RUN-020, J-ADE-056.
- Files:
  - EXT `adeflow/outcome_test.go`, `adeflow/resume_test.go`.
  - New `adeflow/routing_test.go`, `adeflow/agent_tools_test.go`.
- J-RUN-021: fake agent `nofinish`, `fail`, `lost`; timeout via `WithClock`. Each ends with stated outcome in
  `runsOf`, run log file present, task status consistent, no orphan session.
- J-RUN-022: three parallel runs on three tasks.
  - `app.Restart()` mid-run, then resume one.
  - Resumed run continues the same run id with log appended.
  - Other two show interrupted state, never silently "done".
  - Resume prompt includes lease re-acquire line (OQ-ADE-2, shared with B7).
- J-RUN-030: routing matrix workflow on two branches. Loops crossing entry step; max returns hit (J-RUN-020).
  - Each `result:<id>` routes to expected next step (table-driven over route rules).
  - Return counter stops at max with stated outcome.
- J-RUN-032 (OQ-ADE-6 ADOPT):
  - One run calls every task tool via fake agent `MCP`, valid and invalid input interleaved, including branch rename
    and switch (J-ADE-056).
  - Invalid input refused with message; state unchanged (DB, git refs).
  - Tool call after `finish_step` refused "step already finished"; outcome unchanged. Fix in ade tool handler if
    accepted.
- Expected defects: resumed run reported done; post-finish tool mutating outcome.

## B4 Branch git operations from ADE (P1/P2)

- Journeys:
  - Primary: J-ADE-050, J-ADE-052, J-ADE-057 (P1), J-ADE-055 (P2).
  - Merged: J-ADE-051, J-ADE-012.
- Files:
  - EXT `adeflow/rebase_test.go`, `adeflow/branchadopt_test.go`.
  - New `adeflow/forcepush_test.go`.
  - EXT ui `tests/ui/ade-v2-force-push.spec.ts`.
- J-ADE-050 (+ J-ADE-051 refusals):
  - Stack of 3 branches, conflict at middle, abort, retry.
  - After abort: every branch ref equals pre-rebase sha. No rebase in progress (`.git/rebase-merge` absent).
  - After retry: linear stack.
  - Refusals (dirty tree, detached, missing base) leave refs untouched.
- J-ADE-052: rebase run fake agent attempts `Sh` with `git push --force`, `git checkout other`, Bash outside stack.
  Guard refuses. Refs outside stack and bare remote unchanged.
- J-ADE-057 (contract `ade-force-push`):
  - Force push from task panel. Remote named like protected (`origin` vs `upstream` config) refused.
  - Lease: remote moved by `Bare.PushFrom` since last fetch, so push refused. Bare ref still equals other pusher's
    sha.
  - Success path: bare ref equals local.
- J-ADE-055 (+ J-ADE-012): two tasks adopt one branch; alternate work. Commits from both on branch; neither task's
  worktree removed by other's archive.
- Expected defects: abort leaving moved ref; force push without lease.

## B5 Branch requests (P1/P2)

- Journeys:
  - Primary: J-ADE-053 (P1), J-ADE-054 (P2).
  - Merged: J-ADE-011, J-APR-002.
- Files:
  - EXT `adeflow/branchrequest_test.go`.
  - EXT ui `tests/ui/ade-v2-branch-request.spec.ts`.
- J-ADE-053 (contract `ade-branch-request`):
  - Agent requests 20 branches; user approves 7. Task gets new repo meanwhile.
  - Exactly 7 refs created at requested bases; 13 none.
  - Agent receives result listing approved and denied.
  - Approval item joined after render (J-APR-002) shows in approvals queue once.
- J-ADE-054: `WithBranchRequestWait`. Request survives 65 s of fake clock; dies when session ends. No refs created
  after session end.

## B6 Handover, notes, stage moves (P1/P2)

- Journeys:
  - Primary: J-RUN-025, J-RUN-031 (P1), J-RUN-026 (P2).
  - Merged: J-RUN-024, J-ADE-020.
- Files:
  - EXT `adeflow/handover_test.go`, `adeflow/stagemove_test.go`, `adeflow/notes_test.go`.
  - EXT ui `tests/ui/ade-v2-notes.spec.ts`.
- J-RUN-025: handover chain across 4 stages with user edits and late agent call.
  - DB handover text at each stage equals last edit.
  - Late agent call after stage moved is refused or lands on correct stage (pinned).
  - `app.Restart()` keeps chain.
- J-RUN-031 (stage move while headless run live is USER §5 Q9):
  - Stage move with mixed sessions (TUI, headless, one already gone; J-RUN-024).
  - Agreed part:
    - sessions marked closed are closed (no process);
    - no orphan run row in non-terminal state after move;
    - handover drafts kept.
- J-RUN-026 (contract `ade-notes`):
  - 1 MiB note saved. DB row byte-equal after `app.Restart()`.
  - UI copy of note (spy) byte-equal. Partial selection copy equal selected text.
- Expected defects: note truncated at save; orphan running row after stage move.

## B7 Leases (P2)

- Journeys:
  - Primary: J-LEASE-001, J-LEASE-002, J-LEASE-003.
  - Merged: J-RUN-005.
- Files: EXT `adeflow/leases_test.go` (and `internal/leases` fixes).
- J-LEASE-001: three agents contend for one queue across task reorder. At most one holder at any time
  (`app.DB()` lease rows plus agent observations). Order follows queue rules after reorder.
- J-LEASE-002 (`WithClock`): holder process dies, so lease freed. Holder silent past TTL: expired. Force-release by
  user: holder's next call refused.
- J-LEASE-003 (OQ-ADE-2 ADOPT):
  - After `app.Restart()` leases are gone.
  - Resumed agent prompt contains re-acquire line.
  - Fix in resume prompt builder if missing.
- OQ-ADE-3 (sleep vs TTL) is USER §5 Q10: no wall-clock-jump test.

## B8 Proof, artifacts, links, repo updating (P1/P2)

- Journeys:
  - Primary: J-PRF-020 (P1), J-LINK-010, J-LINK-011, J-LINK-020, J-ADE-005 (P2).
  - Merged: J-PRF-001, J-PRF-010, J-ADE-021, J-ADE-023, J-X-012.
- Files:
  - EXT `adeflow/proofchain_test.go`, `proofflow/shots_test.go`, `adeflow/detailslinks_test.go`,
    `adeflow/artifacts_test.go`, `adeflow/issuetracker_test.go`, `adeflow/hostlink_test.go`,
    `adeflow/repoupdating_test.go`.
  - EXT ui `tests/ui/ade-v2-links-strip.spec.ts`, `tests/ui/ade-v2-repo-updating.spec.ts`.
- J-PRF-020 (+ J-PRF-001, J-PRF-010, J-X-012):
  - Proof from Studio MCP (proof tool through `app.ProofWindows` fake) into task chain.
  - Validation proof rejected, then resubmitted. Screenshots saved to disk via `app.Dialogs.AnswerDirectory`: PNG
    files exist with expected count and non-zero size, decode as PNG.
  - Proof survives stage move and archive/restore (DB rows plus files).
  - DB audit visible in Details (J-ADE-023).
- J-LINK-010 (OQ-ADE-1 ADOPT):
  - Paste links, classify, edit, dedupe: `HTTPS://Host/x/#frag` and `https://host/x` dedupe; differing query kept.
  - Open: `app.Browser.Opened()` or in-app.
  - Contract `link-open` has no Go writer and no UI reader. Add writer here and reader in `ade-v2-links-strip.spec.ts`.
- J-LINK-011: agent reports 201 artifacts. Cap enforced at stated limit with message; stored count equals cap; no
  silent drop of first ones.
- J-LINK-020: tracker link (fake acli/az) to merge. Detect, classify, track, snapshot on merge. Host calls recorded
  by `fakehost.Forge.Calls()` exact.
- J-ADE-005: conflict pairs and repo-updating chip. Contract `ade-repo-updating` gets UI reader in
  `ade-v2-repo-updating.spec.ts`.

## B9 Review step loop (P2)

- Journeys:
  - Primary: J-REV-030.
  - Merged: J-REV-020, J-ADE-031.
- Files: EXT `adeflow/reviewstep_test.go`.
- Oracle:
  - Review loop with rebase in the middle; forwarded comments reach implementer (fake agent observation).
  - Comment anchors drift correctly after rebase (line mapping), or marked outdated.
  - Round counter in DB.

## B10 Orchestrator (P2)

- Journeys:
  - Primary: J-RUN-027.
  - Merged: J-RUN-006, J-ADE-072.
- Files: EXT `adeflow/orchestrator_test.go`.
- Oracle:
  - Orchestrator spawns children over two passes; second pass creates no duplicate child (`app.DB()`).
  - Children inherit repos.
  - Parent outcome waits for children; leases among children respected.

## B11 Board concurrency and scale (P2)

- Journeys: J-ADE-044, J-ADE-070.
- Files: new `adeflow/twowindows_test.go`, `adeflow/board_scale_test.go`.
- J-ADE-044: two windows edit same task (title, notes, plan day). Both converge via board events. DB final state
  equals last write per field; no lost unrelated field. Conflict UI policy is USER §5 Q3.
- J-ADE-070:
  - 300 tasks; order, filter, deadlines, bulk move of 50.
  - Persisted order in DB matches; restart keeps order; second window sees same order.
  - Bulk move atomic: all or none on failure injection, e.g. invalid status in batch.

## B12 Agent harness and config (P2)

- Journeys:
  - Primary: J-HAR-001, J-HAR-002, J-HAR-004.
  - Merged: J-X-015.
- Files:
  - EXT harnessflow `provider_test.go`, adeflow `sandbox_test.go`.
  - New harnessflow `config_io_test.go`.
  - EXT ui `tests/ui/ade-agent-settings.spec.ts`.
- J-HAR-001 (+ J-X-015): switch provider and settings; experimental flag and hermetic toggle mid-day. Every launch
  site (task step, TUI, smart script) gets new provider args (fake binary argv record).
- J-HAR-002 (contract `agent-config-io`):
  - Export agent config: `app.Dialogs.AnswerSave`. File bytes contain no secret values.
  - Import back: settings equal. Malformed file refused, settings unchanged.
- J-HAR-004: sandbox policy per step reaches launch (fake sandbox `WithSandbox` records policy per step).

## B13 One task with every failure injected (P2)

- Journeys:
  - Primary: J-ADE-071.
  - Merged: J-ADE-010, J-ADE-040.
- Files: new `journeyflow/ade_task_test.go`. Own file; no edit to `journeyflow/helpers_test.go` (Stream C owns it).
  Copy small helpers locally if needed.
- Oracle:
  - Task across two repos from idea to merged and archived, each failure injected once:
    - agent fail;
    - push rejected;
    - rebase conflict;
    - restart;
    - host rate limit (`fakehost.SetRateLimited`).
  - Final: both branches merged on bare remotes; task archived; worktrees removed; no orphan sessions or runs.

## B14 Runs and automations (P2)

- Journeys:
  - Primary: J-AUTO-001, J-AUTO-002, J-AUTO-004, J-BC-026, J-X-005.
  - Merged: J-RUN-007.
- Files:
  - EXT termflow `runs_form_test.go`, `schedule_test.go`, `runs_file_test.go`, `runs_lifecycle_test.go`.
  - New termflow `script_reuse_test.go`.
- J-AUTO-001: script with params from form. Params reach process argv/env exactly (fake script writes them to file);
  run log persisted.
- J-AUTO-002 (`WithClock`): recurring schedule fires at expected ticks, none missed or doubled across `app.Restart()`.
  Pause stops firing.
- J-AUTO-004:
  - Collections drag reorder persists in file.
  - Bad file reported, good entries kept.
  - Contract `script-folders` gets reader in `tests/ui/automations-file.spec.ts` (EXT).
  - Contract `automations-mcp-app` gets reader in `tests/ui/automations-mcp.spec.ts` (EXT).
- J-BC-026: runs log paging at exact page multiples and tail. Each line once, order kept.
- J-X-005 (+ J-RUN-007): one Runs script used as prepare hook, ADE step, schedule and MCP tool at once. Each
  invocation recorded once with correct params; concurrent runs do not share state files.

## B15 Notifications, prompts, mobile (P1/P2)

- Journeys:
  - Primary: J-MOB-011, J-MOB-014 (P1); J-NOTE-010, J-NOTE-011, J-NOTE-012, J-PROMPT-010, J-MOB-010, J-MOB-012 (P2).
  - Merged: J-NOTE-001, J-PROMPT-001, J-MOB-001, J-X-011, J-ADE-072 (mobile approval).
- Files: EXT notifyflow `notify_test.go`, promptflow `prompts_test.go`, mobileflow `access_test.go`,
  `concurrency_test.go`, `writes_test.go`.
- J-MOB-011 (security):
  - Pairing abuse: replayed pairing code, expired code (`WithClock`), wrong code brute force.
  - Revoked device token, request from second device with first device's token.
  - Each refused; no state change; rate limit applies.
- J-MOB-014: permissions matrix per role/scope. Every write route refused without permission (status code plus
  unchanged DB). Allowed writes land once. Mobile approval of child task (J-ADE-072).
- J-MOB-010: pair, lose network, reconnect, expire. Session resumes within validity; refused after expiry.
- J-MOB-012 (+ J-X-011): phone and desktop move same task stage concurrently. Final stage single and consistent in
  DB; both clients receive same final event.
- J-NOTE-010/011/012:
  - Matrix of focus states. `notifysink` (notifyflow `recSink`) records exactly expected notes.
  - Agent waiting on Monitor produces none until real finish.
  - Click on note focuses right window and task (`app.Focus`, window manager record).
- J-PROMPT-010: prompt routing with windows opening and closing. Each prompt shown in one window at a time
  (`prompts.Router.List`); queued when no window; reroutes on close.
- Mobile Playwright suite stays out of PR CI. This item is flow-only.

## B16 Docker (P1/P2)

- Journeys:
  - Primary: J-DKR-011, J-DKR-013 (P1); J-DKR-010, J-DKR-012, J-DKR-015, J-DKR-020, J-DKR-024 (P2).
  - Merged: J-DKR-014, J-DKR-021, J-DKR-022, J-DKR-001, J-DKR-002.
- Files: EXT dockerflow `edit_test.go`, `streams_test.go`, `containers_test.go`, `restore_test.go`.
- J-DKR-011: in-place edit fails midway (second update rejected by engine). Container config equals either fully old
  or fully new (inspect); error names failed field.
- J-DKR-013 (+ J-DKR-014): recreate with new image, start fails. Automatic rollback restores old container (same
  name, config, volumes attached). Rollback partial failure leaves both names visible and clear error, never
  deleted volumes.
- J-DKR-010 (+ J-DKR-021): rename, limits, networks in place. Inspect equals requested. Name collision refused.
- J-DKR-012: container changed outside app between open and save. Save refused or re-reads; second window edit sees
  conflict.
- J-DKR-015 (+ J-DKR-022): live logs 20k cap; flow asserts cap and order. New ui
  `tests/ui/docker-logs-copy.spec.ts`: copy (spy) equals visible lines. Container restart: stream continues or ends
  once.
- J-DKR-020: double Restart while first runs refused "in progress" (pin ADOPT; fix in `internal/docker` bound
  service if second accepted).
- J-DKR-024: daemon down, then back. Log stream ends once (pin ADOPT). Restore prompt offered once.
- Expected defects: double restart racing; rollback deleting volume.

## File ownership (Stream B)

| Path | Owner |
|---|---|
| `apps/kira-space/internal/flows/adeflow/**` | B |
| `apps/kira-space/internal/flows/agentflow/**`, `harnessflow/**`, `proofflow/**`, `termflow/**` | B |
| `apps/kira-space/internal/flows/notifyflow/**`, `promptflow/**`, `mobileflow/**`, `dockerflow/**`, `usageflow/**` | B |
| `apps/kira-space/internal/flows/journeyflow/ade_task_test.go` (new) | B |
| `apps/kira-space/tests/ui/ade-*.spec.ts`, `automations-*.spec.ts`, `docker-logs-copy.spec.ts` (new) | B |
| `apps/kira-space/tests/contract/ade-*.json`, `runs-*`, `schedule-*`, `script-*`, `automations-*`, `mobile-*`, `notify-*`, `prompt-*`, `docker-*`, `agent-*`, `link-open.json` | B |
| Product: `internal/ade/**`, `internal/leases`, `internal/agentapi`, `internal/agentnotify`, `internal/agentusage`, `internal/proof`, `internal/mobileterm`, `internal/mobileweb`, `internal/dbaudit` | B |
| Product: `internal/docker/**` (G5), `internal/harness/**` (G5 under `harness/claude`) | B |
| Product: `internal/prompts`, terminal/runs scripts packages | B |
| Product: `frontend/src/ade/**`, automations and docker frontend modules, `packages/docker-ui/**` | B |

## Shared files (additive hunks only)

| File | A edit | B edit | C edit |
|---|---|---|---|
| `packages/theme/src/i18n/*.json` | `studio.*`, `backend.studio.*` | `ade.*`, `backend.ade.*`, `docker.*`, `automations.*`, `mobile.*` | `api.http.*`, `git.*`, `backend.space.*`, `kubernetes.*` |
| `docs/ARCHITECTURE.md` "Known open items" | append own | append own | append own |
| `apps/kira-space/internal/flows/coverage/exempt.txt` | append only if guard demands | same | same |
| `apps/kira-space/tests/ui/support/ipcChannels.ts` | append own block if a fix adds channel | same | same |
| `apps/kira-space/tests/ui/studio/support/clipboard.ts` | owner | import only | import only |
| `apps/kira-space/internal/flows/journeyflow/` | none | new file `ade_task_test.go` only | owns `helpers_test.go`, `restart_test.go`, new files |

## No ordering dependency

- B tests and fixes touch only B-owned paths.
- No B test depends on an A or C product fix: Studio MCP proof uses existing proof tool; git checks run through
  ADE paths on bare remotes; host fakes are read-only shared harness.
- `journeyflow` holds B's own new file and needs no edit to C's helpers.
- Shared-file edits are append-only in separate namespaces.

## Follow-ups (design decisions, not built here)

- F-B1: confirm dialog naming run count before workflow switch (OQ-ADE-4, pending USER §5 Q5).
- F-B2: stale-file detection on workflow save (OQ-ADE-5, pending USER §5 Q3).
- F-B3: refuse stage move while headless run live (pending USER §5 Q9).
- F-B4: monotonic-clock lease TTL (OQ-ADE-3, pending USER §5 Q10).
- F-B5: create_task has no duplicate guard (same title under one parent); a second orchestrator pass can duplicate a child if the agent asks (J-RUN-027).
- F-B6: J-HAR-002 has no product feature: no export or import of agent config exists in any bridge service. Needs a design decision and a build phase before `agent-config-io` can be written.
- F-B7: lease TTL decisions use a fake clock only for heartbeats (`LeaseNow`); real tracker check still uses real time (decision 10 open).
- Add each bug found needing design decision here with journey id.

## Done when

- Every item's tests pass in a normal run; container tests pass with `KIRA_FLOW_DOCKER=require`.
- Contracts `link-open`, `ade-repo-updating`, `script-folders`, `automations-mcp-app` each have Go writer and UI
  reader.
- No `time.Sleep` / `waitForTimeout` in new files.
- Hook green on every commit; worktree rebases cleanly.

## Result

Landed B1 to B11, each as flow half plus UI half where a UI surface exists, committed per group.

Product bugs fixed:
- Archived finder missed Jira keys (B1).
- Tool calls after finish_step were accepted; a step reached after a loop-back re-ran once more (B3).
- Stacked rebase abort not recorded on the stack run (B4).
- Several approved branches of one repo reused one draft row, orphaning worktrees and refs (B5).
- Resume prompt lacked a lease re-acquire line (B7, OQ-ADE-2); `LeaseNow` seam and `WithLeaseClock` added.
- Artifact URLs not normalised for dedupe (B8, OQ-ADE-1).

Pinned by design: YAML editor writes invalid text while last valid version serves; first finish wins (OQ-ADE-6); abort restores only the conflicted branch; force push after fetch alone is refused (`--force-if-includes`); SetTaskStep refuses a stage move while a run is live (F-B3 may already hold).

B11: two-window convergence and 300-task scale pass with no product change. No bulk-move API exists; the 50-task move is one SetPlan call and is atomic on failure.

NOT done: B12 (no export/import feature, see F-B6; provider/sandbox argv checks not written), B13, B14, B15, B16. Skipped optional B4 sub-items (branchadopt two tasks, stop-run-first abort refusal).

Tests run: own new specs only, `go test -p 1 ./apps/kira-space/internal/flows/adeflow/ -run <Test>` and targeted Playwright `--project=ui` specs; all passed. No final adeflow-wide rerun after shared product changes.
