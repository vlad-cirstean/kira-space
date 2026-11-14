# Batch 2: P637, P638, P639

User asked to batch these three phases. Each runs as its own stream: own worktree, own Opus plan, own
Sonnet implementer. Base commit `bfc5895d7` on `chapter/v0.3.14`.

| Stream | Phase | Plan | Worktree | Branch |
|---|---|---|---|---|
| B | P637 mobile approval of agent requests | `P637-mobile-approval.md` | `/home/user/kira-space-stream-b` | `stream-b-v0.3.14-2` |
| C | P638 bad item in a collection file | `P638-*.md` (Stream C planner) | `/home/user/kira-space-stream-c` | `stream-c-v0.3.14-2` |
| D | P639 toasts top right | `P639-*.md` (Stream D planner) | `/home/user/kira-space-stream-d` | `stream-d-v0.3.14-2` |

No stream commits on `chapter/v0.3.14` directly; all three rebase onto it at landing.

## File ownership

Each file has one owner. A stream touching a file it does not own stops and reports: ownership
boundary was wrong.

Lists for C and D are this planner's estimate from SPEC.md scope and a CodeGraph survey. Their own
plans name exact files. At landing, check this table against `P638-*.md` and `P639-*.md`; any file
two plans claim moves into Shared files first, or the streams land sequentially.

Stream B (P637, exact; from its plan):

- `apps/kira-space/internal/mobileweb/approvals.go` (new), `routes.go`, `server.go`, `events.go`
- `apps/kira-space/internal/bridge/mobile.go`
- `apps/kira-space/internal/appwire/appwire.go`: one field in the `MobileAccessService` literal
- `apps/kira-space/internal/flows/mobileflow/approvals_test.go` (new), `writes_test.go`
- `apps/kira-space/tests/contract/mobile-approvals.json` (new)
- `apps/kira-space/frontend/src/ade/v2/request/kindDefs.ts` (new), `kinds.ts`, `BranchRequestBody.vue`
- `apps/kira-space/frontend/mobile/**` (`api/approvals.ts`, `api/events.ts`, `approvals/**`,
  `components/PhoneApprovals.vue`, `main.ts`, `App.vue`, `AppShell.vue`, `TabBar.vue`,
  `screens/NeedsScreen.vue`)
- `apps/kira-space/tests/mobile/approvals.spec.ts` (new), `tests/mobile/support/mockServer.ts`
- `apps/kira-space/tests/ui/ade-mobile-approval.spec.ts` (new)

Stream C (P638, estimate):

- API collection file loader and validation: `apps/kira-space/internal/apifiles/**` (`store.go`,
  `lifecycle.go`, `schema/**`, `import.go` as needed) and its `apifilestest` helpers
- Whatever bridge or store surfaces the skipped-item list to the renderer (API module bridge
  service, `internal/apivars` only if touched)
- `apps/kira-space/internal/flows/apiflow/**` new or extended test, plus its contract under
  `apps/kira-space/tests/contract/`
- API module frontend that shows skipped items: under `apps/kira-space/frontend/src/studio/` API
  views (exact paths from C's plan)
- Its UI spec under `apps/kira-space/tests/ui/` (API collection spec)

Stream D (P639, estimate):

- Toast primitive: `packages/theme/src/components/ui/sonner/**`
- Toaster mount: `apps/kira-space/frontend/src/App.vue` (`<Toaster position=...>`)
- Prompt placement and stacking: `packages/workbench/src/prompts/**` (`PromptHost.vue`),
  `packages/workbench/src/approvals/ApprovalsToast.vue`, `packages/theme/src/lib/frames.ts` if the
  floating surface offset moves there
- Toast call sites only if their options change (`ade/v2/state/adeOps.ts`, `adeLeases.ts`,
  `AdePlanView.vue`, `AdeTuiPane.vue`, `AdeProofEntry.vue`, `studio/proof/ProofReadyPrompt.vue`)
- Affected UI specs (`tests/ui/prompts.spec.ts` and any toast spec) and screenshot baselines under
  `tests/visual/**-snapshots` and `tests/screenshots/`

## Shared files

Additive hunks only, each stream in its own section. Rebase conflicts are resolved here and nowhere
else.

| File | Stream B | Stream C | Stream D |
|---|---|---|---|
| `docs/v0.3.14/SPEC.md` | P637 Status `Done`, `### P637` Result | P638 Status, `### P638` Result | P639 Status, `### P639` Result |
| `docs/ARCHITECTURE.md` | "Mobile agents web" bullets, ADE branch request "Desktop only" line, "Approvals surface (P520)" line | API collection files section; delete Known open item "Collection file with one bad item serves no rows on first load" | UI architecture toast/prompt placement bullet |
| `packages/theme/src/i18n/en.json` (and any sibling catalog the copy checks require) | `mobile.approvals.*`, `backend.space.mobileweb.approvalsUnavailable.body` | skipped-item keys under the API namespace | toast keys, if any |
| `apps/kira-space/internal/appwire/appwire.go` | `Approvals: w.Approvals` in `MobileAccessService` literal | only if C wires a new dependency into the API service | none expected |

SPEC.md edits happen only in the landing step, by each stream's implementer, never mid-stream.

`tests/ui/support/ipcChannels.ts`, `support/mockRuntime.ts`, `frontend/src/bridge/index.ts`: B needs
no edit (approvals channels exist). If C or D needs one, it is an additive entry and moves into this
table first.

## No ordering dependency

- B touches the phone server (`mobileweb`), the phone app (`frontend/mobile`), Space approval kind
  defs and their tests. C touches API collection files (Go `apifiles`, API module UI). D touches
  desktop toast and prompt placement (theme sonner, desktop `App.vue`, workbench prompts and
  `ApprovalsToast`). No shared package, symbol, store or bound service between B and C, or C and D.
- B and D, checked for real overlap (both "approvals" and "layout"):
  - B reuses `ApprovalItemCard`, `useApprovals`, `useApprovalActions`, `useApprovalsStore` and the
    kind registry unchanged; it edits none of `packages/workbench/src/approvals/**`. D may move
    `ApprovalsToast` (top-right floating toast) and `PromptHost`; B renders neither on the phone.
  - B edits no desktop `App.vue`, no `packages/theme/src/components/ui/sonner/**`, no prompt
    component. The phone app mounts no `Toaster` (verified: no sonner import under
    `frontend/mobile/`), so D's placement change does not reach the phone.
  - B's desktop spec (`ade-mobile-approval.spec.ts`) asserts the approvals dialog and toast are gone
    after the push, not their position; D's moved toast does not break it. D's prompt/toast specs do
    not read B's contract.
  - Result: no file overlap, no behavioural dependency. If D's plan does edit
    `packages/workbench/src/approvals/ApprovalItemCard.vue` or `registry.ts`, that file becomes shared
    and B rebases after D.
- B and C: both add i18n keys in separate namespaces and an ARCHITECTURE.md hunk in separate
  sections. No code overlap.
- No stream reads another's output. Each verifies independently from base `bfc5895d7`.

## Landing

1. Each stream finishes, passes its own verification, commits clean (no `--no-verify`).
2. Check this ownership table against the C and D plans' file lists (see File ownership).
3. Rebase streams one at a time onto current `chapter/v0.3.14`, in finish order:
   `git -C <worktree> rebase chapter/v0.3.14`. Resolve conflicts only in Shared files above; any
   other conflict stops the landing.
4. After each rebase: `go build ./...`, `bun run build:mobile`, `bun run typecheck`, `bun run lint`,
   and the stream's own targeted tests (its plan's Verification list). Then fast-forward the chapter
   branch: `git -C /home/user/kira-space merge --ff-only <stream branch>`.
5. The stream's implementer fills its SPEC.md row and Result, commits on the chapter branch.
6. After all three land: remove stream worktrees (`git worktree remove <path>`), delete the stream
   branches, push `chapter/v0.3.14`.
