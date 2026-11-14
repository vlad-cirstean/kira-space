# P631 Stream C: HTTP send and curl, git, config sync, secrets, Kubernetes, restart

Implementer plan (Sonnet). Decisions: `synthesis.md` §6. Journey text: `journeys-http-grpc*.md`,
`journeys-repos-git*.md`, `journeys-infra-misc*.md`, `journeys-kubernetes.md`, `journeys-bug-classes.md`,
`journeys-cross-module.md`. Read each journey's Steps and Oracle before writing its test.

Committed set: 49 primary journeys (18 P1, 31 P2) plus 39 merged as variations. Work in item order: P1 items first.
Nothing here is optional.

## Rules for this stream

- Worktree: own git worktree off chapter branch head at stream start. Commit per work item, or per product fix plus
  its test. Conventional Commits.
- Split rule: `NEW both` means one scenario, two halves.
  - Flow half asserts far end:
    - wire recorder bytes (`flowharness.HTTP/HTTPS/GRPC` `Requests()`);
    - bare remote refs (`Bare.Git`);
    - files on disk;
    - `app.DB()`;
    - `fakehost.Forge.Calls()`;
    - `NewKubeFake().Requests()`.
  - Flow half writes `app.Contract(t, "<scenario>", key, v)`. UI half asserts IPC payload and screen against
    `contract(...)`.
  - Contract scenarios here start `git-`, `repo-`, `host-`, `review-`, `api-curl`, `api-http-`, `configsync-`,
    `kube-`, `space-`, `secret-`.
- Git checks:
  - Gitflow uses `app.OpenGitStream` / `GitStream.Request` (bound `GitService`), `app.NewRepo`, `app.NewBare`.
  - Oracle is `git` run in the repo or bare (refs, status porcelain, stash list, worktree list), never the app's own
    read.
- Time:
  - `flowharness.WithClock`; `WithListLimits` for paging boundaries.
  - No `time.Sleep`, no `waitForTimeout`.
- G5:
  - Kubernetes credential tests use fake kube server responses (401/403/exec-plugin error). Never read kubeconfig
    tokens or exec plugin output in Kira code or tests.
  - `githost/` tests use `fakehost` only.
- Clipboard: import `installClipboardSpy` from `tests/ui/studio/support/clipboard.ts` (Stream A owns it; never
  edit).
- Product bugs:
  - Root-cause and fix in this stream, test and fix in one commit group.
  - Design decisions become follow-ups below.
  - Cross-stream defects: a C13/C9 assertion may hit code owned by A or B. Fix in owning package only when the
    owning stream file lists no work on that file. List it under "Cross-stream fixes" below. Otherwise record
    follow-up and keep assertion to agreed part.
- Run own tests only:
  - `go test -p 1 ./apps/kira-space/internal/flows/<pkg>/ -run <Test>`.
  - `bun test packages/api-core/test` for curl cases.
  - UI: `bunx playwright test --config=apps/kira-space/playwright.config.ts --project=ui <spec>` after
    `bun run build:test`.
- Pre-commit hook must pass on every shipped commit.

## C1 Git destructive ops and undo (P1/P2)

- Journeys:
  - Primary: J-GIT-020, J-GIT-024, J-GIT-026, J-GIT-027, J-GIT-029, J-GIT-030 (P1), J-GIT-031 (P2).
  - Merged: J-GIT-003, J-GIT-025, J-BC-062.
- Files:
  - EXT gitflow `bound_writes_test.go`, `ops_local_test.go`, `concurrency_test.go`.
  - New gitflow `undo_test.go`.
- J-GIT-020: switch branch with dirty tree and auto-stash; come back.
  - Status porcelain byte-equal to before (staged vs unstaged kept, untracked kept).
  - Stash list empty after return; no stash leaked.
- J-GIT-024 (+ J-GIT-025; OQ-GIT-1 ADOPT):
  - Stash save, apply, pop with conflict, drop, undo drop.
  - Pop restores index state (`--index`): status byte-equal pre-stash.
  - Pop conflict keeps stash entry.
  - Undo drop restores same stash sha (`git stash list --format=%H`).
  - Global stash across two repos; removal undo. Fix in `gitsession` if pop drops index state.
- J-GIT-026 (+ J-GIT-003): hard, mixed, soft reset with typed confirmation.
  - Stale dialog: HEAD moved by `git commit` outside app after preflight, so reset refused.
  - Undo restores HEAD and index per mode.
- J-GIT-027 (OQ-GIT-2 ADOPT, + J-BC-062):
  - Undo of reset after new local edits that collide: refused with file list. Working tree, index and HEAD
    unchanged.
  - Non-colliding edits: undo succeeds and keeps edits.
  - Undo slot is per repo: op in repo B does not clear repo A's slot, or does by documented rule (`gitpreflight/
    undo.go` `UndoSlot.Set` "called for EVERY op"). Pin per-repo scope.
  - `ExpectedTips` refusal when ref moved outside app (`UndoRecord.ExpectedTips`).
- J-GIT-029: delete branch that is stack parent; undo. Branch ref back at same sha. Stack config entries restored
  (`Replay` config writes).
- J-GIT-030 (OQ-GIT-3 ADOPT): delete annotated and lightweight tag; undo both. Annotated tag object sha equal (not
  re-created lightweight). Tag moved again since delete: undo refused.
- J-GIT-031: undo attribution across two windows on one repo. Window 2 sees label naming window 1's op; undo from
  window 2 allowed or refused per rule; never double replay (`UndoSlot.Take` id check).
- Expected defects: index state lost on pop; annotated tag restored as lightweight; undo replaying over new commits.

## C2 Remote operations (P1/P2)

- Journeys: J-REM-011 (P1); J-REM-010, J-REM-012, J-REM-014 (P2).
- Files: EXT gitflow `remote_test.go`.
- J-REM-011: force push with lease when remote moved (`Bare.PushFrom` from second clone). Refused; bare ref equals
  other pusher's sha. After fetch, push succeeds; bare ref equals local.
- J-REM-010: push branch whose upstream has another name. Bare gets upstream name, not local name; no extra branch
  created.
- J-REM-012: pull strategies (merge, rebase, ff-only) with dirty tree and conflict.
  - Dirty tree preserved (auto-stash restored or refusal).
  - Conflict state reported.
  - ff-only refusal leaves refs unchanged.
- J-REM-014: delete remote branch; credential prompt cancelled. Bare ref still present; no local ref change.

## C3 Worktrees and stacks (P1/P2)

- Journeys:
  - Primary: J-WT-011 (P1), J-WT-010, J-STACK-001 (P2).
  - Merged: J-WT-012, J-WT-013, J-GIT-004, J-GIT-005, J-STACK-002, J-STACK-003.
- Files:
  - EXT gitflow `worktree_state_test.go`.
  - New gitflow `stack_test.go`.
- J-WT-011 (+ J-WT-012):
  - Remove worktree with blockers: dirty, locked, current.
  - Worktree becomes dirty between dialog and confirm: removal refused at confirm (re-check). Files intact on disk.
- J-WT-010 (+ J-WT-013, J-GIT-004):
  - Create worktree in each mode (new branch, existing, detached, remote-only) with every blocker.
  - Prunable worktree after directory deleted outside: shown prunable; prune removes only metadata.
- J-STACK-001 (+ J-STACK-002/003, J-GIT-005):
  - 4-level stack; amend bottom; restack; undo. Each level rebased onto new parent tip; undo restores all refs.
  - Hand-edited stack config with cycle, dangling parent, 70-deep chain: refused with message, no infinite loop
    (test bounded by context deadline).
  - Base remote-tracking pruned: stack reports missing base, no ref rewrite.

## C4 Checkout, cherry-pick, preflight staleness, bound service lifecycle (P2)

- Journeys:
  - Primary: J-GIT-021, J-GIT-023, J-GIT-028, J-BC-045.
  - Merged: J-GIT-022, J-GIT-002.
- Files:
  - New gitflow `preflight_test.go`.
  - EXT gitflow `ops_local_test.go`, `bound_test.go`.
- J-GIT-021 (+ J-GIT-022): preflight computed, then tree goes clean, or merge starts outside app before confirm. Op
  re-checks at execute: refuses on new in-progress op; proceeds on clean tree. Refs and status oracle.
- J-GIT-023: checkout tag (detached), sha, remote-only branch (creates tracking). HEAD and upstream config oracle.
- J-GIT-028 (+ J-GIT-002): cherry-pick series with conflict at 2nd, empty pick at 3rd, `--no-commit`.
  - Continue, skip, abort each leave expected refs.
  - Quit (`app.Restart()`) mid-sequence: in-progress state detected after restart.
- J-BC-045: git bound service after window close and reopen (`app.CloseWindow`, `OpenGitStream` again). Old holds
  released (`RepoClose`); new requests served; no leaked repo entry.

## C5 HTTP send, redirects, curl (P1/P2)

- Journeys:
  - Primary: J-HTTP-041, J-HTTP-048, J-HTTP-065, J-MCP-011 (P1); J-HTTP-002, J-HTTP-040, J-HTTP-044, J-HTTP-045,
    J-HTTP-049, J-HTTP-060 (P2).
  - Merged: J-HTTP-005, J-HTTP-042, J-HTTP-043, J-HTTP-003, J-HTTP-006, J-HTTP-007, J-HTTP-010, J-HTTP-030,
    J-HTTP-066, J-HTTP-061..064.
- Files:
  - New httpflow `redirect_test.go`, `unresolved_test.go`.
  - EXT httpflow `send_test.go`, `wire_test.go`, `curl_test.go`.
  - EXT dbmcpflow `api_test.go`.
  - EXT `packages/api-core/test/http-curl.spec.ts`, `curl-cases.json`.
  - EXT ui `tests/ui/studio/http-curl.spec.ts`, `tests/ui/studio/http-variables.spec.ts`.
- J-HTTP-041 (+ J-HTTP-005/042/043; security):
  - Redirect A to B to B with custom secret header (`X-Api-Key`), `Authorization`, `Cookie`.
  - Recorder on B: none of them present after origin change. Origin change covers host, port and https-to-http on
    same host.
  - Same-origin hop keeps them.
  - Fix in `internal/httpclient` if strip covers only `Authorization`.
- J-MCP-011: DbMcp `api_request` follows same redirect rules (shared client). Rules and credentials never leak to
  other origin; recorder oracle.
- J-HTTP-048 (+ J-HTTP-003; OQ-API-2 ADOPT; contract `api-http-unresolved`):
  - Unresolved `{{name}}` in URL, header and body: send blocked, unresolved names listed. Recorder receives zero
    requests.
  - UI half asserts no `HttpService.Send` IPC call and names listed.
  - Precedence globals, collection, environment resolved per rule.
  - Fix in send path if literal `{{x}}` reaches wire.
- J-HTTP-065 (+ J-HTTP-030, J-HTTP-066; contract `api-curl` extended):
  - Copy as curl for every body mode (none, raw, code, urlencoded, formdata, file).
  - UI half: spy bytes equal contract `curl:generated:<mode>`.
  - Flow half replays each command with `sh -c` against recorder: method, query, headers, body sha equal app's own
    send (pattern of existing `TestCurlReplayMatchesSend`).
  - Masked secrets (J-HTTP-066): clipboard shows mask, never secret bytes, until gated reveal. Switching environment
    after copy changes nothing already copied.
- J-HTTP-040 (+ J-HTTP-006; OQ-API-1 hint is follow-up F-C1): cookie jar default off. Login flow: recorder sees no
  `Cookie` on second request. Jar on: cookie sent. Incognito tab: none.
- J-HTTP-044: redirect limits 0, 1, 10, 100, 101. Hop count on recorder equals limit; limit+1 gives error naming
  limit.
- J-HTTP-045 (+ J-HTTP-007/010):
  - Timeout extremes via `flowharness.Silent`: error after deadline.
  - 5 MiB response cap: body truncated flag set, stored body length equals cap.
  - Odd bodies (binary, gzip, invalid UTF-8) shown with marker.
- J-HTTP-002, J-HTTP-049: every body mode wire bytes on recorder (multipart boundaries, urlencoded escaping). File in
  form-data changed between sends: second send carries new bytes.
- J-HTTP-060 (+ 061..064; OQ-API-3 ADOPT):
  - Parser cases in `curl-cases.json`: basic auth with unicode and `{{var}}`; `@file` refs warned not inlined;
    `-G` with `--data-urlencode` and existing query; unknown flags warned; non-curl and broken curl give error.
  - `-G` expected query bytes recorded from real curl `--trace-ascii` once into testdata, not run live.
- Expected defects: custom header leak across origin; unresolved variable sent literally.

## C8 Config sync (P1/P2)

- Journeys:
  - Primary: J-CFG-010, J-CFG-012, J-CFG-013 (P1); J-CFG-002, J-CFG-011 (P2).
  - Merged: J-CFG-001, J-X-010.
- Files: EXT configsyncflow `configsync_test.go`.
- J-CFG-010 (+ J-CFG-001):
  - Pull with conflicts; choose per row (mine/shared).
  - Files on disk equal chosen versions.
  - Backup dir `backups/configsync-<ts>` holds replaced files byte-equal.
  - Kept count matches.
  - Two applies within same second under `WithClock` must not overwrite earlier backup (`apply.go:127` names backup
    with second precision). Fix if they collide.
- J-CFG-012:
  - Crash mid-apply. Write journal `applying` with half the files written (stage files plus journal through test
    setup mirroring `commit`), then `app.Restart()`.
  - `Recover` rolls back: files equal pre-apply bytes; journal gone.
  - Committed journal: finish swaps base; state applied commit set.
  - Extend existing `TestRecoverRollsBackInterruptedApply` with committed case and module store reads after restart.
- J-CFG-013:
  - Write failure mid-apply: pre-create a directory at one target file path, so write fails even as root.
  - Every earlier file rolled back byte-equal; journal removed; status shows no interrupted problem.
  - Error surfaced.
- J-CFG-011 (+ J-X-010): preview goes stale (target edited, or newer fetch). Apply refused "changed", nothing
  written. Modules (ADE workflows, API files) reload after successful apply.
- J-CFG-002: `${NAME}` and `${secret:NAME}` references across modules. Applied file keeps reference text; resolved
  value only at use; secret never written to disk.

## C9 Secret hygiene (P1)

- Journeys:
  - Primary: J-SEC-001.
  - Merged: J-X-004.
- Files: new `journeyflow/secret_hygiene_test.go`.
- Scenario:
  - Seed distinct secret markers: connection password, API secret variable, `${secret:}` config ref, agent env
    secret, Runs script secret param.
  - Use each: connect, send request, launch fake agent step, run script, mobile read of task.
- Oracle:
  - Walk every file under `app.Root`; no marker bytes, including SQLite DB, WAL, logs, run logs, op log, contract
    temp files.
  - Excluded: the secret store's own file. Tests use insecure `basic_text` fallback (see `dbflow/sqlite_test.go`
    `TestSecretsStatusAndReveal`). Assert exactly that one path holds markers.
  - Fake agent argv/env record holds secret only where step declared it.
  - Mobile read and op log mask it.
- Expected defects: secret in op log command text or run log.

## C11 Kubernetes (P1/P2)

- Journeys:
  - Primary: J-K8S-025 (P1); J-K8S-012, J-K8S-021, J-K8S-022, J-K8S-024 (P2).
  - Merged: J-K8S-010, J-K8S-011, J-K8S-020, J-K8S-001, J-MCP-012, J-BC-024.
- Files: EXT dbmcpflow `kube_test.go`, kubeflow `clusters_test.go`, `list_test.go`, `logs_test.go`.
- J-K8S-025 (+ J-K8S-011, J-MCP-012; OQ-INF-1 ADOPT):
  - Agent kube read via MCP needs approval per cluster.
  - Approval in cluster A does not cover B.
  - Secret values masked `<N bytes>` through MCP, same as UI.
  - `NewKubeFake().Requests()` shows only GET; no request before approval.
  - Fix mask bypass if MCP returns data.
- J-K8S-021 (+ J-BC-024, J-K8S-010; OQ-K8S-2 ADOPT):
  - 5,000 pods; list truncated at cap.
  - Result carries truncation flag and banner text "showing first N; sort and filter cover loaded rows only".
  - Continue token paging returns each pod once.
- J-K8S-024 (+ J-K8S-020/001): follow logs across container restarts (`AppendLog`, restart). Lines in order, no
  duplicates at reconnect. Tail limit honoured. Copy evidence equals lines (UI spy, EXT
  `tests/ui/studio/kubernetes-module.spec.ts` is A-owned path, so add new `tests/ui/kubernetes-logs-copy.spec.ts`).
- J-K8S-022: expired credentials, RBAC 403, exec plugin failure, unreachable, all from fake server or fake plugin
  exit code. Each gives distinct message; no retry storm (request count bounded).
- J-K8S-012: kubeconfig edge cases (multiple files in `KUBECONFIG`, missing current context, duplicate names).
  Clusters list equals expected; malformed file reported.

## C13 Restart checklist and crash (P1/P2)

- Journeys:
  - Primary: J-BC-070 (P1), J-BC-071 (P2).
  - Merged: J-GRID-005, J-CELL-003, J-SHELL-001, J-X-007.
- Files:
  - EXT `journeyflow/restart_test.go`.
  - New `journeyflow/crash_test.go`.
  - New `flowharness/crash.go`.
- J-BC-070 (+ J-X-007, J-SHELL-001, J-GRID-005, J-CELL-003): one checklist test.
  - Set state in every module through bound services: windows and layout; tabs; grid projection and widths settings;
    cell dock height; console saved query; ADE board order; API open tabs and active environment; git panel repo;
    docker restore preference.
  - Start running things: fake agent run, schedule, HTTP send in flight.
  - Then `app.Restart()`.
  - Oracle:
    - each persisted item equal;
    - console results and Discover hits not restored, no auto-run (pin ADOPT);
    - interrupted run shown interrupted;
    - no duplicate schedule fire.
- J-BC-071: crash instead of quit.
  - `flowharness/crash.go` `func (a *App) Crash()` drops the app without shutdown hooks.
  - Preferred: run app via `flowharness/cmd` as subprocess and SIGKILL it, if that host supports bound calls in
    harness.
  - Otherwise: snapshot `SpaceHome` (DB, WAL, shm, config files) at a quiesced point, `Quit`, restore snapshot,
    rebuild.
  - Oracle:
    - DB opens without corruption (`PRAGMA integrity_check` = ok);
    - config sync journal recovered;
    - no half-written config file (atomic writes);
    - interrupted run resumable.
  - If neither mechanism is deterministic in seconds, do not ship a flaky test. Record F-C2, drop J-BC-071 in
    this file with reason, report to orchestrator.

## C15 Repos, host, review, git paging and copy values (P2)

- Journeys:
  - Primary: J-REPO-002, J-GRAPH-004, J-WS-001, J-HOST-001, J-REV-001, J-BC-009, J-BC-023, J-BC-025.
  - Merged: J-REV-002, J-X-009.
- Files:
  - EXT repoflow `configfile_test.go`, `edit_test.go`; githostflow `sync_test.go`, `comments_test.go`; reviewflow
    `incremental_test.go`; gitflow `graph_test.go`.
  - New gitflow `copy_values_test.go`.
  - EXT ui `git-graph-plane.spec.ts`, `git-bound-review.spec.ts`, `copy-coverage-git.spec.ts`.
- J-REPO-002: repo config through form and YAML. File bytes equal expected; ADE reads new config (bound read).
  Invalid YAML refused, file unchanged.
- J-GRAPH-004 (+ J-X-009):
  - PR badges follow host state: `fakehost.Forge.SetHead`, then sync.
  - Badge state equals host in graph, review window and host view reads.
  - Contract `git-graph-plane` gets UI reader in `git-graph-plane.spec.ts`.
- J-WS-001: edit file in workspace; file changed on disk outside app before save. Save refuses overwrite, offers
  reload. Disk bytes equal outside version until user chooses.
- J-HOST-001 (+ J-REV-002): PR comment round trip. `Forge.Calls()` shows exact comment body and anchor. Reply
  appears after sync. Comment forwarded to agent lands once.
- J-REV-001: incremental review across rebases. Reviewed-file marks survive rebase where content same; reset where
  changed. Contract `git-review-edit` gets UI reader in `git-bound-review.spec.ts`.
- J-BC-009 (contract `git-copy-values`):
  - Flow writes real values from fixture repo: full 40-char sha, message, branch, path, patch.
  - Patch equals `git format-patch -1 --stdout` minus header lines that vary.
  - UI half copies each (spy) and compares to contract.
- J-BC-023 (`WithListLimits`): graph Load more at exact page multiples and after refs change. Each commit once; no gap
  at boundary.
- J-BC-025: host comments and checks past caps. Paging fetches all; cap message when beyond hard cap.

## File ownership (Stream C)

| Path | Owner |
|---|---|
| `apps/kira-space/internal/flowharness/crash.go` (new) | C |
| `apps/kira-space/internal/flows/gitflow/**`, `repoflow/**`, `reviewflow/**`, `githostflow/**`, `windowflow/**`, `appflow/**` | C |
| `apps/kira-space/internal/flows/httpflow/**`, `configsyncflow/**`, `configflow/**`, `kubeflow/**`, `memoryflow/**`, `captureflow/**`, `llmpromptsflow/**` | C |
| `apps/kira-space/internal/flows/dbmcpflow/api_test.go`, `kube_test.go` | C |
| `apps/kira-space/internal/flows/journeyflow/**` except B's new `ade_task_test.go` | C |
| `apps/kira-space/tests/ui/git-*.spec.ts`, `repo-*.spec.ts`, `copy-coverage-git.spec.ts`, `kubernetes-logs-copy.spec.ts` (new) | C |
| `apps/kira-space/tests/ui/studio/http-curl.spec.ts`, `tests/ui/studio/http-variables.spec.ts` | C |
| `packages/api-core/src/http/curl/**`, `packages/api-core/test/http-curl*.spec.ts`, `curl-cases.json` | C |
| `apps/kira-space/tests/contract/git-*.json`, `repo-*`, `host-*`, `review-*`, `api-curl.json`, `api-http-*`, `configsync-*`, `kube-*`, `space-*`, `secret-*` | C |
| Product: `internal/httpclient`, `internal/git*`, `internal/graphplane`, `internal/repocfg`, `internal/githost` (G5), `internal/issuetracker` (G5), `internal/configsync`, `internal/kubernetes`, `internal/appshell`, `internal/appcore` | C |
| Product: `frontend/src/studio/api/state/curl.ts` and HTTP send path, git frontend, `packages/git-*/**` | C |

## Shared files (additive hunks only)

| File | A edit | B edit | C edit |
|---|---|---|---|
| `packages/theme/src/i18n/*.json` | `studio.*`, `backend.studio.*` | `ade.*`, `backend.ade.*`, `docker.*`, `automations.*`, `mobile.*` | `api.http.*`, `git.*`, `backend.space.*`, `kubernetes.*` |
| `docs/ARCHITECTURE.md` "Known open items" | append own | append own | append own |
| `apps/kira-space/internal/flows/coverage/exempt.txt` | append only if guard demands | same | same |
| `apps/kira-space/tests/ui/support/ipcChannels.ts` | append own block if a fix adds channel | same | same |
| `apps/kira-space/tests/ui/studio/support/clipboard.ts` | owner | import only | import only |
| `apps/kira-space/internal/flows/dbmcpflow/` | owns `approvals_test.go`, `tables_test.go`, `review_test.go`, `mcp_test.go` | none | owns `api_test.go`, `kube_test.go` |
| `apps/kira-space/internal/flows/journeyflow/` | none | new `ade_task_test.go` only | owns rest |

## No ordering dependency

- C tests and fixes touch only C-owned paths. The one exception is the listed cross-stream fixes, each in a file no
  other stream's plan touches.
- J-MCP-011 sits here, so the httpclient fix and its MCP consumer land together.
- C13 restart checklist reads A/B state through bound services only. It needs no A/B fix to pass for state those
  streams leave unchanged.
- Shared-file edits are append-only in separate namespaces.

## Cross-stream fixes

- None planned. Record here any fix C13 or C9 makes in A- or B-owned code: file, journey id, one-line cause. The
  landing step then resolves it.

## Follow-ups (design decisions, not built here)

- F-C2: real SIGKILL crash harness (app out of process). C13 ships a byte-snapshot crash instead (`flowharness/crash.go`; models lost post-snapshot writes and WAL recovery, not a torn single-file write or a mid-operation kill), deterministic in under a second; a true kill needs the composition root hosted in a subprocess.
- F-C3: versioned old-schema DB fixtures for migration and backup journey (J-SEC-002).
- F-C4 (J-REM-011): a fetched-but-not-integrated remote branch shows no state beyond ahead/behind; a design call on surfacing it.
- F-C5 (J-HTTP-064): `wget https://h` through the import dialog parses as URL `wget` with a `multiple-urls` warning; only the paste path is guarded by `looksLikeCurlCommand`.
- F-C6 (J-BC-025): host detail caps keep the newest 100 comments and first 100 checks with a truncation flag; there is no paging to fetch the rest. Paging needs a per-provider fetch design.
- F-C7 (J-REV-001): review marks stay `full` with `changedSinceReview` after a content change; whether a changed file should reset to unreviewed is a UX decision.

## Done when

- Every item's tests pass in a normal run, and with `KIRA_FLOW_COMPLETE=1 KIRA_FLOW_DOCKER=require` where gated.
- Contracts `git-graph-plane`, `git-review-edit` each have Go writer and UI reader.
- No `time.Sleep` / `waitForTimeout` in new files.
- Hook green on every commit; worktree rebases cleanly.

## Result

Commits on `stream/v0.3.13-c` (see `git log 6dd1c122c..`): C1 `996a5b09d`, C2 `48498fcce`, C3 `3165ad006`, C4
`40959941d`, redirect fix `e9e54e16e`, OQ-API-2 block `f7ae0f3eb`, `-G` encoding `49dc0394e`, C5 `b977589ba`, C8
`adfd93df1`, C9 `c9e980293`, C11 `169493fea`, C13 `d6459d188`, C15 `2c64118ef` and `28546a073`; continuation: J-CFG-002 and
configsync `c47dba59c`, C9/C13 `9fe89040e`, C15/HTTP/kube `e55b1643c`.

Bugs found and fixed:

- `httpclient.checkRedirectFor`: `>=` allowed only max-1 redirects (J-HTTP redirect limits).
- OQ-API-2: unresolved `{{var}}` was sent literally. Go `apivars.ResolveRequest` and the renderer `send()` now block with the names listed
  (`backend.studio.http.unresolvedVariables.body`).
- curl `-G` with `--data-urlencode`: pieces were not encoded and non-literal pieces leaked into the query (J-HTTP-060..064).
- `configsync.Apply`: two applies in one second shared one backup dir and overwrote the first backup (J-CFG-010). Now `-2`, `-3` suffixes.
- Kubernetes truncation banner now reads "...sort and filter cover loaded rows only" (OQ-K8S-2). Kube log view gained Copy log (J-K8S-024).
- C1 to C4 fixes are in their own commit messages (gitsession ops, gitops reset, gitpreflight worktree, git-ui ops state).

Pinned to existing behaviour: J-REM-011, J-REM-012, J-GIT-026, J-WT-013, J-GIT-020 (C1 to C4); J-REPO-002 invalid save is kept and flagged
with last good values in use (not refused); J-REV-001 marks persist with `changedSinceReview`; J-BC-009 context-menu Copy message copies the
subject only, the full message rebuilds from subject, body and trailers; J-BC-025 caps with truncation flags, no paging.

Deviations and decisions (continuation pass closed every earlier gap):

- J-HTTP-063: decided per D6, flags that change nothing on the wire (`-s`, `-S`, `-L`, `--compressed`, `-o`, `--retry N`) stay silent; only
  unknown flags warn, by raw text, and never eat the URL. Pinned in `curl-cases.json` and `http-curl.spec.ts` (raw text asserted).
- J-HTTP-045: timeout clamp 99,999,999 to 3,600,000 is unobservable from outside (no test can wait an hour, and no wire field reports
  the clamped value), so it is dropped; 0 = no timeout, size cap edges and override precedence are tested.
- J-HTTP-066: UI assertion added in `http-curl.spec.ts` (copy, switch environment, reopen shows new value, clipboard unchanged).
- J-HTTP-040: UI half added (`http-request.spec.ts`, contract `api-cookie-jar` written by `TestCookieJar`). F-C1 done: the response
  Cookies pane shows "No cookies were sent. The cookie jar is off for this request." (reflects the current effective setting).
- C8: J-CFG-002 flow (`TestAgentSettingsReferencesStayText`) and UI row assertion (`config-sync.spec.ts`) added.
- C9: mobile half added (`mobileflow/secrets_test.go`): phone reads of board, workflows, sessions, repos, run log and agent sessions
  carry no secret plaintext. Oracle made truthful: the secret store file is obfuscated with a hardcoded key (basic_text fallback), so a
  byte scan of it proves nothing. `TestSecretsNeverReachDisk` now proves the values are live through the store API
  (`AgentSecrets.Store.Open`, `Connections.Reveal`) and scans every other file, log and export for markers.
- C11: exec-plugin failure class stays in `kubeflow/status_test.go`; `kubernetes-logs-copy.spec.ts` became
  `tests/ui/studio/kubernetes-hardening.spec.ts` (fixtures live under `studio/`). F-C8 done: Copy log writes the filtered view
  (`LogsView.vue`), asserted in the same spec.
- C13: checklist now also covers ADE plan order (`TestRestartChecklist`). Schedule no-duplicate-fire is `termflow`
  `TestScheduleNoDuplicateFireAfterRestart`; Docker restore preference (a dismissed offer stays dismissed) is `dockerflow`
  `TestRestoreDismissSurvivesRelaunch`; in-flight HTTP send is `httpflow` `TestRestartDropsInFlightSend`. Quit does not cancel HTTP
  ops (process exit closes the socket); the test stands a cancelled call context in for the exit and asserts no history entry and a
  working send after relaunch.
- F-C2 (crash model): `flowharness.Snapshot`/`CrashTo` model writes after the snapshot point lost, SQLite recovery from main file plus
  WAL and shm without checkpoint, and each config file whole as of that point. Not modeled: a torn write of one file and a mid-operation
  process kill (children, sockets). Torn config writes are covered where they matter by `configsyncflow` journal tests
  (`TestRecoverFinishesCommittedJournal`, `TestMidApplyFailureRestoresReplacedFiles`). A true SIGKILL harness stays F-C2.
- C15: net-new tests are copy values (flow plus UI), review marks across rebase, host caps. UI readers added: `git-graph-plane.spec.ts`
  reads `git:graph.loadMore#bound`; `ade-v2-review.spec.ts` reads the `git-review-edit` list and edit contract. Already covered, confirmed
  per item: J-REPO-002 `repoflow/configfile_test.go` (Save writes file, edit on disk reloads, ADE `adewire.Repo` read, invalid kept and
  flagged); J-WS-001 `repoflow/edit_test.go` ("conflict leaves disk untouched", E_CONFLICT contract); J-BC-023 `gitflow/graph_test.go`
  (`TestGraphPagesToEnd` at exact multiples 2500/100 and 50000/5000, `TestGraphRefreshAfterExternalCommit`); J-GRAPH-004
  `githostflow/sync_test.go` (change request index follows host head) plus `repo-graph-change-request-badge.spec.ts`; J-HOST-001
  `githostflow/comments_test.go` (`Forge.Calls()` exact bodies and anchors).
- Disk filled mid-run (shared host). Freed Go and node caches; another stream's go-build temp dir may have been removed.

Tests run: full non-UI Go flows once at the end (`bun run test:flows`, 31 packages, exit 0, no flakes), after targeted runs per item.
Playwright `ui`, targeted only: `config-sync` (6), `git-graph-plane`, `ade-v2-review`, `http-curl`, `http-request`, `kubernetes-hardening`,
`kubernetes-module`, `repo-commit-detail`, the C5 studio specs. api-core `bun test` (`http-curl.spec.ts` 93 pass).
