# P639 Toasts top right

Spec row: toasts anchor top right, same placement as prompts. No overlap between a prompt and a
toast: stack them. One shadcn-vue toast primitive, no new hand-rolled one. Update affected UI specs
and screenshots. User: "The toasts should be top right like the prompts."

## Current placement

- Toast primitive: shadcn-vue Sonner wrapper `packages/theme/src/components/ui/sonner/Sonner.vue`
  over `vue-sonner` 2.0.9 (MIT). Unstyled, `floatingSurface` look, class `z-(--kira-z-popover)`.
  Prop surface: `position` only. `index.ts` re-exports `toast` from `vue-sonner`. Not hand-rolled.
- One mount: `apps/kira-space/frontend/src/App.vue:255` `<Toaster position="bottom-right" />`.
  vue-sonner default offset 24px (16px under 600px viewport width). Callers: `adeOps.ts`,
  `adeLeases.ts`, `AdePlanView.vue`, `AdeTuiPane.vue`, `AdeProofEntry.vue`, `ProofReadyPrompt.vue`.
- "Prompts" at top right: `packages/workbench/src/approvals/ApprovalsToast.vue`, the minimised
  approvals prompt (P520). Root classes `fixed top-[calc(var(--kira-titlebar-h)+8px)] right-4
  z-(--kira-z-dialog)`. So prompt placement = 8px under the 34px title bar, 16px from right edge.
  Shown by `ApprovalsSurface.vue` when the approvals dialog is minimised or a `PromptHost` prompt is
  on screen.
- Routed prompts (`PromptHost` kinds: schedule, update, quit, handover, proof, masking, pairing) are
  centred modal `DialogContent` (`fixed top-1/2 left-1/2`, z dialog). Not corner surfaces.
- Second hand-rolled corner surface: `apps/kira-space/frontend/src/configsync/ConfigSyncToast.vue`,
  `fixed right-4 bottom-10 z-(--kira-z-dialog)`, persistent until Review or Dismiss, main window only.
  ARCHITECTURE "Config sync" says "Toast is bottom-right in workbench windows".
- Top-right chrome: title bar (`TitleBar.vue`, `h-titlebar` = 34px) holds panel toggles, Settings,
  keep-awake, New window at its right end. Anything at `top >= titlebar-h + 8px` clears it. No other
  fixed top-right element (grep `fixed top-`/`fixed right-`: only these two plus the dialog).
- z rungs (`tokens.css`): dialog 100, popover 200. Sonner stays above dialogs: an error toast raised
  from a modal (ProofReadyPrompt open failure) must stay readable. vue-sonner's unlayered
  `[data-sonner-toaster]{z-index:999999999}` likely beats the layered Tailwind utility; out of scope,
  unchanged by this phase.

## Design

One top-right column, top to bottom: persistent prompt chips, then Sonner toasts. Same anchor as
ApprovalsToast today: `top: calc(var(--kira-titlebar-h) + 8px)`, `right: 16px`.

1. New `packages/workbench/src/components/CornerStack.vue` (`<script setup lang="ts">`, Tailwind
   only). Renders:
   - a fixed container `fixed top-[calc(var(--kira-titlebar-h)+8px)] right-4 z-(--kira-z-dialog)
     flex flex-col items-end gap-2 pointer-events-none *:pointer-events-auto`,
     `data-testid="corner-stack"`, default slot inside;
   - `<Toaster position="top-right" :offset :mobile-offset />` after it.
   Container height via VueUse `useElementSize(el, undefined, { box: 'border-box' })`. Toaster
   offset top = `calc(var(--kira-titlebar-h) + 8px + Hpx + 8px)` when `H > 0`, else
   `calc(var(--kira-titlebar-h) + 8px)`; right `16px`. `mobileOffset` same top, left and right
   `16px` (vue-sonner switches to full-width under 600px viewport; a narrow desktop window hits it).
   vue-sonner applies `offset` in a reactive style binding, so a chip appearing or leaving moves the
   toast column. Constants: one `GAP_PX = 8` and `EDGE_PX = 16` in the component.
   `pointer-events-none` on the container keeps the gap strips and the empty area left of a narrower
   chip click-through; chips keep pointer events. Empty container (only comments, e.g. v-if false or
   a teleported dialog's placeholder) measures 0.
   No Pinia store: the height is local to this component; nothing else reads it.
   Why not render ApprovalsToast/ConfigSyncToast as `toast.custom(..., { duration: Infinity })`:
   both must stay fully visible and actionable while transient toasts come and go; Sonner's collapsed
   deck shows only the front toast in full and drops toasts past `visibleToasts` (3) from view. Named
   requirement, so the chips stay their own surfaces above the Sonner list.
2. `Sonner.vue`: widen props to `Pick<ToasterProps, 'position' | 'offset' | 'mobileOffset'>` and
   bind them. No other change; it stays the one toast primitive.
3. `ApprovalsToast.vue`: drop `fixed top-[…] right-4 z-(--kira-z-dialog)` from its root; keep
   `floatingSurface flex items-center gap-3 p-3`. Placement now comes from CornerStack.
4. `ConfigSyncToast.vue`: drop `fixed right-4 bottom-10 z-(--kira-z-dialog)`; same as step 3. It is a
   toast, so it moves top right with the rest. Order in the stack: approvals first (agent blocked
   waiting), config sync second.
5. `App.vue`: replace `<ApprovalsSurface />`, `<Toaster position="bottom-right" />`,
   `<ConfigSyncToast v-if="mainWindow()" />` with

   ```vue
   <CornerStack>
     <ApprovalsSurface />
     <ConfigSyncToast v-if="mainWindow()" />
   </CornerStack>
   ```

   Drop the `Toaster` import. `ApprovalsDialog` inside `ApprovalsSurface` portals to body through
   `DialogContent`, so nesting it in the fixed container changes nothing for the dialog. Container
   keeps z dialog, so a modal's scrim (teleported later, same rung) still covers the chips as today.

Accepted overlap: a `size="full"` dialog (approvals dialog, top 48px, right edge 32px from window
edge) sits under the toast column's top-right, as it sat under the bottom-right column before. Toasts
auto-dismiss; z above dialogs stays so modal errors remain readable. Not a prompt/toast overlap in
the spec's sense: ApprovalsDialog and ApprovalsToast never show together.

No Go flow half: placement is frontend-only. No IPC, bridge, contract or backend state changes, so
there is no backend result to assert. UI spec alone proves it.

## Files

Owned by this stream (P639):

- `packages/theme/src/components/ui/sonner/Sonner.vue` (edit)
- `packages/workbench/src/components/CornerStack.vue` (new)
- `packages/workbench/src/approvals/ApprovalsToast.vue` (edit: root classes)
- `apps/kira-space/frontend/src/configsync/ConfigSyncToast.vue` (edit: root classes)
- `apps/kira-space/frontend/src/App.vue` (edit: mount block, imports)
- `apps/kira-space/tests/ui/toast-placement.spec.ts` (new)
- `apps/kira-space/tests/visual/studio/approvals.spec.ts-snapshots/approvals-toast-visual-linux.png`
  (re-record only if it diffs; expected unchanged)
- `docs/ARCHITECTURE.md` (edit, see below)
- `docs/v0.3.14/SPEC.md` (landing step only: P639 Status, `### P639` Result)

Shared-file risk for the batch table:

- `packages/workbench/src/approvals/ApprovalsToast.vue`, and `App.vue` near `<ApprovalsSurface />`:
  likely touched by P637 (mobile approval: "desktop reflecting the result" may edit the approvals
  surface or its mount). P639 edits only ApprovalsToast's root class string and App.vue's
  ApprovalsSurface/Toaster/ConfigSyncToast block. Additive or one-line hunks; resolve in rebase.
  P639 does not touch `ApprovalsSurface.vue`, `ApprovalsDialog.vue`, `state/approvals.ts`,
  `approvalsQueries.ts` or anything under `apps/kira-space/frontend/mobile/`.
- `docs/ARCHITECTURE.md`: shared with P637 (approvals/mobile sections) and P638 (Known open items,
  API collections). P639 edits three lines: "Approvals surface (P520)" bullet on ApprovalsToast,
  "Config sync" bullet "Toast is bottom-right…", and the frames bullet (line ~1408) for the corner
  stack. Distinct hunks.
- `apps/kira-space/tests/ui/approvals.spec.ts`: P637 may extend it; P639 does not edit it (new
  spec file instead).
- No overlap expected with P638 code (apifiles, API module UI).
- No i18n keys added.

## Implementer steps

1. Sonner.vue props (design step 2).
2. CornerStack.vue (design step 1). Import `Toaster` from `@theme/components/ui/sonner`,
   `useElementSize` from `@vueuse/core`, `useTemplateRef` and `computed` from `vue`. Short comment on
   why chips are not Sonner toasts (one line, the named requirement).
3. ApprovalsToast.vue and ConfigSyncToast.vue root classes (design steps 3, 4). Keep `role`,
   `aria-live`, testids.
4. App.vue mount block (design step 5). Import CornerStack from
   `@workbench/components/CornerStack.vue`.
5. ARCHITECTURE.md:
   - frames bullet (~1408): add "Toasts and corner prompt chips share one top-right column
     (packages/workbench CornerStack): chips (ApprovalsToast, ConfigSyncToast) first, Sonner toasts
     below, 8px under the title bar, 16px from the right edge; the toast offset follows the chips'
     measured height."
   - Approvals surface (~3282): note ApprovalsToast sits at the top of that column.
   - Config sync (~3765): "Toast is bottom-right in workbench windows" becomes top-right, in the
     corner column under any approvals chip.
6. New UI spec `apps/kira-space/tests/ui/toast-placement.spec.ts` (`test` from `./fixtures`).
   Toast trigger: held branch removal from `ade-v2-background-ops.spec.ts` (`openPlan` with
   `dirty` risk plus `{ channel: IPC.adeTaskRemoveTaskBranch, hold: true }`, `openTask(page,
   'T_cart')`, `pick(page, 'b_cart', 'ade-task-branch-remove')`, confirm). The "Removing" loading
   toast stays while held: a stable target. Title bar bottom: bounding box of
   `[data-testid="titlebar-app-title"]` parent (`locator('..')`). Viewport 1400x900 unless stated.
   Cases:
   - toast alone: `[data-sonner-toast]` box top = titlebar bottom + 8 (±1), right edge =
     viewport width − 16 (±1). Toaster `[data-sonner-toaster]` has `data-y-position="top"` and
     `data-x-position="right"`.
   - stacked: also push approvals (`pushApprovals(page, [liveItem(item)])` with an item from
     `contract<ApprovalItem[]>('approvals-studio', 'ApprovalsService.List')`, then Escape to
     minimise) and config sync status `{ channel: IPC.configSyncStatus, response: { ...status,
     showToast: true } }` (reuse the `config-sync` contract shape from `config-sync.spec.ts`).
     Assert order and no overlap: approvals-toast top = titlebar bottom + 8 (±1);
     config-sync-toast top ≥ approvals bottom + 8 − 1; sonner toast top ≥ config-sync bottom +
     8 − 1; all three right edges = viewport width − 16 (±1); pairwise boxes do not intersect.
   - chip leaves: Review on the approvals chip (dialog opens, chip gone) or `pushApprovals(page,
     [])`; then sonner toast top moves up (poll its box top until ≤ config-sync bottom + 8 + 1).
   - narrow window: viewport 520x800; sonner toast top ≥ titlebar bottom + 8 − 1, left ≥ 0, right
     ≤ 520.
   - pointer pass-through: with approvals chip and config-sync toast of different widths, a click at
     a point inside the corner-stack box but outside both chips reaches the page (e.g.
     `document.elementFromPoint` returns an element outside `[data-testid="corner-stack"]`).
   No mock runtime or contract changes expected. If `openPlan`'s fixture lacks the needed channels,
   extend the spec's own control list, not shared support.
7. Run the specs that show toasts or corner chips (the column now covers the top-right of the tab
   strip area under the title bar; a toast could intercept a click there): see Verification. Fix any
   that a moved toast now blocks by waiting for the toast to clear or clicking a target it does not
   cover, never by hiding the toast.
8. Commits: `feat: toasts and corner prompts share one top-right column` (steps 1–4),
   `docs: corner toast column` (step 5), `test: toast placement UI spec` (step 6), fixes as
   follow-ups.

## Oracles

- Sonner toast anchored top right: toaster `data-y-position="top"`, `data-x-position="right"`; box
  top = title bar bottom + 8px, right gap 16px, same as ApprovalsToast's anchor before this phase.
- Stack: approvals chip, config-sync toast, Sonner toasts in that order, ≥ 8px apart, no
  intersecting boxes, right edges aligned.
- Offset follows chips: chip removal moves toasts up.
- Below 600px: toast stays under the title bar and inside the viewport.
- One toast primitive: `grep -rn "from 'vue-sonner'" apps packages --include=*.vue --include=*.ts`
  lists only `packages/theme/src/components/ui/sonner/` files; `grep -rn "<Toaster"` lists only
  CornerStack.vue. No new `fixed` toast-like element: `grep -rn "fixed top-\|fixed right-"
  --include=*.vue apps packages` shows only CornerStack.vue and DialogContent.vue (plus proto Hud).
- Existing behaviour kept: `approvals.spec.ts`, `prompts.spec.ts`, `config-sync.spec.ts` pass
  unchanged.

## Visual baselines

- `tests/visual/studio/approvals.spec.ts-snapshots/approvals-toast-visual-linux.png`: element shot
  of ApprovalsToast. Expected pixel-identical (same surface, shrink-to-fit width in both layouts). If
  it diffs, re-record only that spec: check `fc-match sans-serif` prints `Inter`
  (docs/DEV_ENVIRONMENT.md "tests/visual/* pixel diffs"), then
  `bunx playwright test --config=apps/kira-space/playwright.config.ts --project=visual
  visual/studio/approvals.spec.ts --update-snapshots` after `bun run build:test`. Never from a
  mismatched font environment.
- No other visual or `tests/screenshots/` shot captures a toast or the config-sync toast (grep
  `toast|sonner` under `tests/visual` and `tests/screenshots`: only approvals.spec.ts). Visual suite
  runs in PR CI; a CI diff there is fixed on the chapter branch per CLAUDE.md PR CI rules.

## Verification

- `bun run typecheck:web`, `bun run lint`.
- `bun run build:test`, then
  `bunx playwright test --config=apps/kira-space/playwright.config.ts --project=ui
  ui/toast-placement.spec.ts ui/approvals.spec.ts ui/prompts.spec.ts ui/config-sync.spec.ts
  ui/ade-v2-agent-ops.spec.ts ui/ade-v2-background-ops.spec.ts ui/ade-v2-repo-required.spec.ts
  ui/ade-v2-task-branches.spec.ts ui/ade-v2-leases.spec.ts ui/ade-v2-needs.spec.ts
  ui/ade-v2-proof-chain.spec.ts ui/ade-v2-force-push.spec.ts ui/ade-v2-handover.spec.ts
  ui/ade-v2-sessions.spec.ts ui/ade-v2-task-launch.spec.ts ui/ade-v2-run.spec.ts
  ui/ade-v2-sessions-menu.spec.ts ui/ade-v2-review-step.spec.ts ui/ade-v2-op-kinds.spec.ts
  ui/studio/dbmcp-unreachable.spec.ts` (every spec referencing `sonner`, `approvals-toast` or
  `config-sync-toast`).
- `bunx playwright test --config=apps/kira-space/playwright.config.ts --project=visual
  visual/studio/approvals.spec.ts` once, to confirm the baseline holds.
- Oracle greps above.
