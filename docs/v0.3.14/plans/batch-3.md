# Batch 3: P640, P641, P642

User asked to batch these three phases. Each runs as its own stream: own worktree, own Opus plan, own
Sonnet implementer. Base commit `ee9e0d597` on `chapter/v0.3.14`.

| Stream | Phase | Plan | Worktree | Branch |
|---|---|---|---|---|
| B | P640 API tab close dialog | `P640-api-tab-close-dialog.md` | `/home/user/kira-space-stream-b` | `stream-b-v0.3.14-3` |
| C | P641 own Jira and PR links first | `P641-*.md` (Stream C planner) | `/home/user/kira-space-stream-c` | `stream-c-v0.3.14-3` |
| D | P642 task list day styling | `P642-*.md` (Stream D planner) | `/home/user/kira-space-stream-d` | `stream-d-v0.3.14-3` |

No stream commits on `chapter/v0.3.14` directly; all three rebase onto it at landing.

## File ownership

Each file has one owner. A stream touching a file it does not own stops and reports: ownership
boundary was wrong.

Lists for C and D are this planner's estimate from SPEC.md scope and a CodeGraph survey. Their own
plans name exact files. At landing, verify this table against the file lists in `P641-*.md` and
`P642-*.md` and against each stream's real diff (`git diff --stat ee9e0d597..<branch>`). Any file
two streams touch moves into Shared files first, or those streams land sequentially.

Stream B (P640, exact; from its plan):

- `packages/workbench/src/state/confirmDialog.ts`, `components/ConfirmDialogView.vue`,
  `components/ConfirmDialog.vue`
- `packages/workbench/src/tabs/types.ts`, `packages/workbench/src/state/createTabsStore.ts`
- `packages/api-core/src/http/saved.ts`, `packages/api-core/src/grpc/saved.ts`,
  `packages/api-core/src/index.ts`
- `apps/kira-space/frontend/src/studio/api/state/tabClose.ts` (new), `saveRequestDialog.ts`,
  `collections.ts`
- `apps/kira-space/frontend/src/studio/views/shared/request/useRequestTabSave.ts`,
  `views/httprequest/HttpRequestView.vue`, `views/grpcrequest/GrpcRequestView.vue`
- `apps/kira-space/frontend/src/studio/state/tabKinds.ts`, `studioTabs.ts` (only if a typed finder
  is needed)
- `apps/kira-space/internal/flows/apiflow/tabclose_test.go` (new),
  `apps/kira-space/tests/contract/api-tab-close.json` (new)
- `apps/kira-space/tests/ui/studio/api-tab-close.spec.ts` (new),
  `apps/kira-space/tests/ui/studio/support/dialogs.ts`

Stream C (P641, estimate):

- Link order: `apps/kira-space/frontend/src/ade/v2/links/linkGroups.ts` (group build and sort),
  `links/useTaskLinks.ts` if the order is applied there, `links/AdeLinkStrip.vue` only if rendering
  order changes
- Any other sorted list sharing the order (C's plan names it), under `apps/kira-space/frontend/src/ade/v2/`
  outside `plan/AdeDayBand.vue` and `plan/AdePlanView.vue`
- Go side only if the order comes from the backend (`internal/ade` artifacts/links) and its flow test
  under `apps/kira-space/internal/flows/adeflow/` plus contract under `tests/contract/ade-*.json`
- UI spec for links under `apps/kira-space/tests/ui/` (`ade-v2-*links*` or `ade-details-links`)

Stream D (P642, estimate):

- Day bands: `apps/kira-space/frontend/src/ade/v2/plan/AdeDayBand.vue` (`bg-today`,
  `bg-today-tint`, `data-today`, left bar), `plan/AdePlanView.vue` only if band layout moves
- Theme tokens for today and the day bar: `packages/theme/src/styles/**` token CSS and the Tailwind
  theme mapping that defines `today` colors (light and dark)
- Visual spec for the plan (`apps/kira-space/tests/visual/**`) and its screenshot baselines
  (`tests/visual/**-snapshots`, `tests/screenshots/`)

## Shared files

Additive hunks only, each stream in its own section. Rebase conflicts are resolved here and nowhere
else.

| File | Stream B | Stream C | Stream D |
|---|---|---|---|
| `docs/v0.3.14/SPEC.md` | P640 Status `Done`, `### P640` Result | P641 Status, `### P641` Result | P642 Status, `### P642` Result |
| `docs/ARCHITECTURE.md` | UI ConfirmDialog/tabs bullet (`confirmClose`, `choose()`), API module Save section | ADE links order line | UI theme tokens / ADE plan today and day bar line |
| `packages/theme/src/i18n/en.json` (and sibling catalogs the copy checks require) | `studio.api.tabClose.*` | link keys under `space.ade.links`, if any | tooltip/aria keys under `space.ade.plan`, if any |
| Registries (`tests/ui/support/ipcChannels.ts`, `support/mockRuntime.ts`, `frontend/src/bridge/index.ts`, `internal/appwire/appwire.go`) | none (no new IPC) | additive entry only if C adds IPC | none expected |
| Screenshot baselines showing both the link strip and the plan | none | refresh after landing | refresh after landing |

SPEC.md edits happen only in the landing step, by each stream's implementer, never mid-stream.

Screenshot baselines are binary: never merge two streams' versions of one PNG. If C and D both
change one baseline, the later stream to land regenerates it on top of the earlier one.

## No ordering dependency

- B touches the generic tab close path (`createTabsStore`), the shared confirm dialog, and the API
  module (Studio side). C and D touch only ADE (`ade/v2/**`) and, for D, theme tokens. No shared
  symbol, store or bound service between B and C or B and D.
- B and the shared confirm dialog: the change is additive (optional secondary button, new `choose`
  action; `confirm()` signature and testids unchanged). C and D use `ConfirmDialogView` only through
  existing props (AdePlanView day-off confirm); neither needs the new prop. If C or D plans edit
  `ConfirmDialogView.vue` or `confirmDialog.ts`, that file becomes shared and B lands first.
- B and D both theme-adjacent: B adds no token, no CSS, no `<style>`; D's token change does not reach
  the dialog buttons B renders.
- C and D, checked for real overlap (both ADE task UI):
  - C changes link order (`links/linkGroups.ts` and its consumers: `AdeLinkStrip.vue` in Details,
    run and stage dialogs). D changes day-band chrome in `plan/AdeDayBand.vue` and theme tokens.
    `linkGroups` has no caller under `plan/` (survey: only `links/**` imports it), so the plan view
    renders no link strip and D's files do not consume C's order.
  - Overlap risk: if C's plan also reorders link chips on the plan task card (`plan/AdeTaskCard.vue`)
    and D's plan touches the same card, that file becomes shared; land C first, D rebases.
  - Screenshots: D refreshes plan baselines; C may refresh Details/link baselines. A baseline showing
    both (a plan screenshot with Details open) follows the binary rule above.
- No stream reads another's output. Each verifies independently from base `ee9e0d597`.

## Landing

1. Each stream finishes, passes its own verification, commits clean (no `--no-verify`).
2. Verify the ownership table against the C and D plans and every stream's real diff (see File
   ownership).
3. Rebase streams one at a time onto current `chapter/v0.3.14`, in finish order:
   `git -C <worktree> rebase chapter/v0.3.14`. Resolve conflicts only in Shared files above; any other
   conflict stops the landing.
4. After each rebase: `bun run typecheck`, `bun run lint`, `go build ./...` (only if the stream
   touched Go), and the stream's own targeted tests (its plan's Verification list). Then fast-forward
   the chapter branch: `git -C /home/user/kira-space merge --ff-only <stream branch>`.
5. The stream's implementer fills its SPEC.md row and Result, commits on the chapter branch.
6. After all three land: remove stream worktrees (`git worktree remove <path>`), delete the stream
   branches, push `chapter/v0.3.14`.

## Disk

About 5 GB free. Implementers avoid extra caches: no second `node_modules` install per worktree
beyond what the worktree needs, no `bun install --force`, no Playwright browser re-downloads, no
`-count` loops over container suites. If the disk fills, run `go clean -cache -testcache` first, then
retry; report if space stays short.
