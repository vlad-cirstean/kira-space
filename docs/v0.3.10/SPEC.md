# v0.3.10

UI polish, dialog speed and MCP dialog fixes. Phases P617 to P619 run as parallel streams A, B, C (batch requested by user); P620 and P621 run after they land, in order. Highest prior phase: P616.

| Phase | Title | Status |
|-------|-------|--------|
| P617 | Task list and detail UI | Done |
| P618 | Dialogs: speed, colors, background ops, proof, copy | Done |
| P619 | Settings MCP split and GitHub PR open | Done |
| P620 | Resource leases: named queues, push wake-up | Done |
| P621 | Proof window: screenshots dropdown and copy-all | Done |

## P617 Task list and detail UI (Stream A)

- Task detail shows worktree's current checked-out branch next to the assigned branch; mismatch is visible (agents sometimes switch branches). MCP gains a tool to change the branch, gated by an approval prompt.
- Link card gets a copy button beside open-external.
- "Needs input" indicator sits at the start of the task row, not mid-row. Agent status indicators bigger.
- Task row elements use fixed, identical column sizing across tasks (status first: width independent of its text) so rows scan at a glance.
- Task list "today" left bar: clearly marks today without a bright yellow clashing with the task color. Task color gets a presentation other than, or in addition to, a colored left border.
- Opening Sessions tab in task detail focuses the keyboard into the open agent terminal.

Result: Done. Backend: `BranchDetached.head`, worktree mismatch in `task_info` and `branch_status`, new approval-gated `change_branch` MCP tool (`TaskBoard.ChangeBranch`, switch item, approvals class "change"; adeflow `TestBranchChange`, contract `ade-branch-change`). Review hardened it: refused before any prompt while a script-run claim, a step run, a second open agent session on the row or a task-level session (no `BranchID`, works in every own worktree) uses the folder. UI: mismatch chip and panel checkout line, change request body, link card copy, card head columns, lead attention slot, 16px activity icons, today marker, task tint, terminal focus on Sessions open. Deviations: plan commits 1 and 2 merged (interface change breaks build alone); contracts hold branch rows, not boards; refusals use outcome `failed`; rename path has no `snapshotChangeRequests`, so change clears the row's stored change request directly; permanent U62 exemption for `terminalRenderer.ts`; check icon 16px and `min-h-5` forced by lint guards; Later shelf row follows the card grid (lead, status, flags at card offsets; spec checks it). Tests: Go vet and lint clean; agentmcp, llmprompts, ade, adeflow branch tests, llmpromptsflow pass; typecheck passes; 12 targeted ui specs pass after fixing plan, task-row and adopt specs for tinted cards and `head` fallback.

## P618 Dialogs: speed, colors, background ops, proof, copy (Stream B)

- Audit every MCP-triggered dialog (create branch, create task, handover, others): content renders instantly with the dialog, handover text included. No delayed content.
- Semantic button colors (approve, deny, destructive, confirm) consistent across the whole app via shared shadcn-vue variants.
- Delete branch and close task: dialog dismisses once confirmed; progress shows as status in relevant places (task row, branch row, toast) until done.
- Proof-ready dialog: closes after open or close. Title order: task title first, proof title second.
- Studio MCP dialogs: text selectable and copyable (API host and all other fields).
- MCP prompt dialogs (approval, query, request) use the full dialog space with a fixed layout: approve/deny stay at the same screen position whatever the query or request length (fixed footer, content fills the rest).
- Query/request text gets maximum room: larger dialog, content area flex-1, scroll only on real overflow.
- Exactly one prompt per dialog: others queue and the next shows once the current one is resolved. Root-cause the stacking; UI spec, plus flow test if backend-involved.

Result: Done. Handover: backend emits the board event before the prompt event (flow test asserts order); dialog seeds its draft from the handovers query and gates on it; `AdeNotesEditor` builds its `Editor` synchronously and applies outside changes while empty. `FlushEditor` cap removed, height follows Monaco. Approvals: one item at a time with position badge, fixed footer in `ApprovalItemActions`, full-size dialog; no bulk approve (review round 1 removed "Approve all", it approved prompts never seen; Deny all stays); the stacking root cause was `v-for` over all items. Buttons: `dialog-approve` and `dialog-deny` variants, `dialog-primary` and `dialog-danger` retuned, all callers remapped, guard N17 (not N15, taken). Background ops: vue-sonner `Toaster`, `adeOps` store, `AdeOpStatus` chip on task and branch rows; dialog closes on confirm. Proof ready: title is the task, proof title below, Open hides first, `MarkSeen` withdraws only the ready popup. Copy: `select-text` on dialog content and tag text. First-frame measurement in `plans/P618.md`: every dialog 0 frames late (baseline from code analysis). Specs: ade-v2-background-ops, approvals-first-frame, approvals-one-at-a-time, studio/proof-ready, studio/studio-mcp-copy; alignment and dbmcp/approvals specs adapted to one item at a time.

## P619 Settings MCP split and GitHub PR open (Stream C)

- Settings "database MCP" section is too long: split into several sections. Rename user-facing label to "Studio MCP".
- Opening a GitHub PR from the app does not work: reproduce, root-cause, fix.

Result: Settings "Database MCP" is now "Studio MCP", split into five nav sections: Studio MCP
(server, preview, focus, token), Connections, Kubernetes, Scripts, API tools. Children indent via
`SettingsShell` `indentedSections`; nav scrolls. Panes: `StudioMcpServerPane` (renamed from
`DatabaseMcpPane`), `StudioMcpConnectionsPane`, `StudioMcpApiPane`; Kubernetes and Scripts reuse
their sections. User-facing "database MCP" copy renamed across en.json, Go strings, READMEs, site and
ARCHITECTURE. Specs updated; new `studio-mcp-settings.spec.ts`; five settings visual baselines
re-recorded. The API pane label is "API tools", not "API requests": the copy lint rejects a
near-duplicate of the configSync label. 'Studio MCP' joins the copy lint's proper nouns.
GitHub PR open: root cause was an invalid `detailQuery` (differing-argument field merges on
`comments`, `reviews`, `assignees`; `Team.name` String! vs `User.name` String), which GitHub rejects
whole, so the PR view showed a failure. Fixed with `detail*` and `teamName` aliases. New
`TestGraphQLDocumentsValidate` checks every GitHub document against a vendored schema (CC-BY-4.0,
NOTICES.md; gqlparser MIT, test-only) and fails on the old query with four errors. Flow
`TestOpenGitHubPullRequest` and `github-pr-open.spec.ts` share contract `github-pr-open`. Targeted
runs: githost, githostflow, dbmcpflow (count 2), adeflow `TestDbMcpStep`, 55 UI specs, copy-coverage
settings test, settings visual; typecheck, lint, lint:go, lint:dead clean.

## P620 Resource leases: named queues, push wake-up

Agents sharing an unsharable resource (port, full-suite run, push, merge) wait in order instead of colliding.

- Multiple named queues, each its own resource: one holder, FIFO-by-priority waiters. User names queues. Backend in Go, frontend state in a Pinia store.
- MCP tools: acquire, release, status. No blocking: acquire returns at once, granted or queued with position. Queue entry keyed by session id, so position survives agent re-calls.
- Wake-up: when a lease frees, app injects a short message into the next agent's session once that agent is idle (never types into a busy agent). Fallback: hook on the agent's next turn start or stop reports the grant.
- Release guarantees: all leases of a session release when the session ends (crash, kill, exit); TTL with heartbeat (any holder tool call extends); explicit release as courtesy.
- Priority: next holder is the waiter whose task sits highest in the task list; re-evaluated on each grant. Never preempts a current holder.
- UI: panel lists every queue with holder and waiters. Click holder: "Request release" (injects finish-up message) or separate "Force release" (revokes). Queue state shown on task rows.
- Verify first: how session end and idle detection behave on a hard kill; drop injection to hooks-only if idle cannot be detected safely.
- Tests: backend flow test and UI spec for same scenario (queue, grant, release, force release).

Result: Done. Backend: `internal/leases` (in-memory Book: ranked waiters, no preemption, 10 min TTL with heartbeat, alive only inside an open tool call, capped at 3x TTL, or while a headless run lives; a turn left busy by an interrupt expires), board glue in `ade/leases.go`, tools `acquire_lease`, `release_lease`, `lease_status` (prompt registry, budget), `LeasesService` plus push `kira:ade:leases`, queue names in `ade_lease_queues` (migration 0099). Rank reads the task list order only when a queue has waiters, cached 5 s and dropped by generation on a board change so pushes never wait on a build. Rank uses the task list order: `ade/taskorder.go` is a Go port of the TS `taskListOrder` (extracted to `board/taskOrder.ts`), pinned by one golden file (227 cases) read by a bun spec and a Go test; today is the machine's local date on both sides. Idle kept: tracker turn state (busy, idle, input, unknown) from hook events, `SendIfIdle` at grant, held back while the user left unsent typed text in the terminal prompt box; fallback is the hook reply (Stop block, UserPromptSubmit context), then the idle handler, then lease tool results for headless. Hook replies are new plumbing (`HookReply`, shim `--output -`); OpenCode gets injection only. Waiters of a removed queue hear a removed notice; queue saves are serialized. Session end releases through `Reconcile` (works on SIGKILL, where `SessionEnd` never fires). UI: Leases sub-tab in Agent config (queue editor, holder menu with Request release and confirmed Force release), `adeLeases` store, lease chip on task cards. Deviations: plan commits 6 and 7 merged (interface change breaks the build alone); queue editor saves on add and remove, no draft, removing a held queue asks to confirm; duplicate or empty name disables Add instead of showing an error; fakeagent gained `MCPAfter` and a `.pid` record; no TTL flow step (unit test covers it). Tests: leases -race, `TestTaskListOrder`, ade tracker tests, agentmcp, llmprompts, harness, `TestLeases` flow (hard kill included) and session-status, panel-forward, branch-change, automation flows, bun parity, timeline and board-parity specs, 13 ui specs (leases, session-status, card-icons, agents-tab, task-row, plan), typecheck, lint, lint:go, lint:dead clean; real claude: hook row incl. new `TestHookStopBlock` and the whole ADE row pass (auth probe ok).

## P621 Proof window: screenshots dropdown and copy-all

Brief: `plans/P621-brief.md`.

- Fix the broken "save screenshots" dropdown in the proof window (reproduce, root-cause).
- Default action becomes "copy all screenshots to clipboard" only where the platform can put several images on the clipboard natively (Go side). No stitched single image, ever.
- Where it cannot (e.g. macOS if multi-image clipboard is impossible): default stays the existing save-to-folder action.
- Backend flow test and UI spec for the same scenario.

Result: Done. Dropdown bug: `ProofScreenshotMenu` reused `DropdownMenuLabel` (one-line header: fixed `h-control-sm`, uppercase, centred) for the 3-line location block, so it clipped and the path overprinted "Change location…"; the caller now overrides those classes (primitive untouched), and `proof-window.spec.ts` asserts no clipping, no uppercase, no overlap. Copy-all: macOS can hold several images (`NSPasteboard` items), so `internal/imageclip` (darwin cgo) writes one item per tab, `public.png` plus `public.file-url`; Windows and Linux report `multiImage` false (one image per clipboard format, several only as file lists), so save stays primary there. No stitched image. Backend: `ProofService.ShotClipboard`, `OpenShotClip`, `AddShotClip`, `CommitShotClip`, `CloseShotClip` (temp dir per batch, previous batch removed after the next commit; `decodeShot` shared with `SaveJpegTo`), `appwire.Options.ImageClipboard`, flow fake `ImageClipboard`. UI: `useShotClipboard` query picks the primary action (`data-mode` copyAll or saveAll), `walkTabs` shared by `saveAll` and `copyAll`, status strip handles `copied`, menu gains "Save screenshots" when copy is primary. Deviations: fake clipboard has `SetFail` instead of a `Fail` field (lock-safe); darwin readback helpers live in `readback_darwin.go` behind the `imagecliptest` tag because cgo is not allowed in `_test.go`; review made the darwin shim read PNG files in Go, off the main thread, and clean up batches (committed one at app quit, evicted uncommitted one at once); `staleTime` option dropped from the query (default is already Infinity, lint U53); `threeSteps` moved to `support/proof.ts` (lint N8); `TestProofShotCopyAll` redirects `TMPDIR` to find batch dirs. Not run here: the darwin cgo file and `TestWriteImagesPrivatePasteboard` (no Mac); checked with gofmt, `CGO_ENABLED=0` darwin vet of the fallback file, Linux vet and build. Workflow patch `docs/pending-changes/.github__workflows__pr.yml.patch` adds the package to the macOS `go test` list. Mac checklist steps 11 and 12 and a Known open item cover the real pasteboard. Tests: `TestProofShotCopyAll`, proofflow, `internal/proof`, bridge, 4 `proof-copy-all` specs and the proof-window, image, chart, diagram specs, typecheck, lint, lint:go, lint:dead clean; real claude DB row (`bun run test:claude:db`, auth probe ok) pass.

Chapter review: round 1 found 14 issues (2 High, 7 Medium, 5 Low), all fixed. Round 2 found 7 issues (3 Medium, 4 Low), all fixed. The full non-UI run fixed 3 Go test failures: bound services 61 (`LeasesService`), llmprompts count 277, and G5 scope (first an exemption for the hook listener's bearer check, narrowed in round 2 to request header writes).
