# P497 plan: task "+" launch menu shares the Runs dropdown

Row: `SPEC.md` P497. Frontend only, IPC unchanged, so no backend test.

Paths under `apps/kira-space/frontend/src/ade/v2/` unless full.

## Today (verified with `codegraph_explore`, then grep for specs)

- `sessions/AdeTaskLaunchMenu.vue`: own shadcn `DropdownMenu` + `DropdownMenuTrigger` around a
  `TooltipIconButton` (`ade-task-launch`, `:pending`), content `ade-task-launch-menu` `align="end"`.
  Four hand-written branches (several targets x provider choice). Disabled reasons render as an
  inline muted `<span data-testid="ade-task-launch-reason">`. No worktree anywhere: a disabled
  `TooltipIconButton` with `disabled-trigger` and label `space.ade.taskLaunch.reason.noWorktree.label`.
  Only caller: `sessions/AdeSessionsTab.vue:96`.
- `sessions/useTaskLaunch.ts`: `targets` (from `board/taskLaunch.ts` `launchTargets`, each with
  `terminal`/`agent` `Gate {disabled, reason}`), `pending`, `openTerminal(target)`,
  `startAgent(target, provider = '')`.
- Runs dropdown: `packages/workbench/src/automations/AutomationsPanel.vue:375-381` header
  `TooltipIconButton` `automations-add` with `@click="onAddClick"`; `onAddClick` (line 171) calls
  `useContextMenuStore().openContextMenuAt(rect.left, rect.bottom + 2, newScriptItems(createScript))`.
  The singleton `packages/workbench/src/components/ContextMenu.vue` (testid `context-menu`, mounted in
  `apps/kira-space/frontend/src/App.vue:181` and kira-studio) renders items through
  `components/MenuItems.vue`: icon column, `menu-item-<id>` testids, `hint` as tooltip plus
  `sr-only` text (`aria-describedby`), disabled-with-hint keeps pointer events, `submenu` as
  `DropdownMenuSub` (`context-submenu`), `label` as `DropdownMenuLabel`.
- Tab strip "+": `automations/AutomationsNewTab.vue` over `components/TabStripNewButton.vue`
  (title-size button with its own wrapper, emits its element) also anchors with
  `openContextMenuAt(rect.left, rect.bottom + 2, ...)`. Same in `apps/kira-space/frontend/src/repo/GitNewTab.vue`.
- `automations-add` is in `packages/workbench`, so both apps use it. Specs clicking it:
  `apps/kira-space/tests/ui/automations-scripts.spec.ts`, `apps/kira-studio/tests/ui/automations-module.spec.ts`,
  `packages/workbench/src/testing/ui/scenarios/automationsSmart.ts`, `automationsRuns.ts`, and two
  `e2e-real/prompts-two-windows-real.spec.ts`. All click it, then `menu-item-new-*`.
- Task "+" specs: `apps/kira-space/tests/ui/ade-v2-task-launch.spec.ts` (all tests),
  `apps/kira-space/tests/ui/agent-provider.spec.ts:127-129`. `ade-v2-launch.spec.ts` does not touch
  `ade-task-launch*`; not changed.

## Which "+" the task "+" matches

The task "+" is a toolbar `TooltipIconButton` in a bar, same as `automations-add`. The tab strip
"+" is a different trigger (`TabStripNewButton`, title size, strip wrapper). So `automations-add`
adopts the shared component. Tab strip "+" (`AutomationsNewTab`, `GitNewTab`) stays: its look is
the tab strip's, not a toolbar icon; it already opens the same singleton menu.

## Design

### 1. Shared component: `packages/workbench/src/components/MenuIconButton.vue` (new)

A toolbar icon button that opens a `MenuItem[]` dropdown under itself through the context-menu
store. One home for the anchoring that `onAddClick` hand-writes today.

```vue
<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { type MenuItem, useContextMenuStore } from '../state/contextMenu';

// A toolbar icon that opens its items in the shared dropdown, anchored under the button.
// Every other attribute (data-testid, aria-label, pending, class) falls through to the button.
defineOptions({ inheritAttrs: false });
const props = withDefaults(defineProps<{ label: string; items: () => MenuItem[]; icon?: string }>(), {
  icon: 'add',
});
const menu = useContextMenuStore();

function open(e: MouseEvent): void {
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
  menu.openContextMenuAt(rect.left, rect.bottom + 2, props.items());
}
</script>

<template>
  <TooltipIconButton :icon="icon" :label="label" aria-haspopup="menu" v-bind="$attrs" @click="open" />
</template>
```

- `items` is a function: built at click time, so gates (session open, setup state) are fresh.
- `+ 2` and `rect.left` copy `onAddClick` exactly, so both buttons place the menu the same way.
- `pending` falls through to `Button` (it already reads it: disabled + spinner), as today.
- No `disabled` handling inside: the no-worktree fallback keeps its own `TooltipIconButton`
  (needs `disabled-trigger` and a different label).

### 2. Runs adopts it: `packages/workbench/src/automations/AutomationsPanel.vue`

- Replace the `automations-add` `TooltipIconButton` with
  `<MenuIconButton :label="t('automations.panel.add.tooltip')" :aria-label="t('automations.script.new.label')" :items="() => newScriptItems(createScript)" data-testid="automations-add" />`.
- Delete `onAddClick` and its comment. Keep `newScriptItems` import (still used). Keep
  `TooltipIconButton` import (search/about buttons). `contextMenuStore` stays (row menus).
- Testid unchanged, so its specs need no edit; they must still pass.

### 3. Task "+" items: `sessions/useTaskLaunch.ts`

Move provider data into the composable and add `items(): MenuItem[]`, the `useRunAutomation.menuItems`
pattern (composable returns the menu model; component stays thin). Imports: `useAgentInfo`,
`useAgentList` from `@workbench/agent/agentInfo`; `type MenuItem` from `@workbench/state/contextMenu`.

Add to the composable:

- `providers = computed(() => list.value.filter((e) => e.available))`, `choose = providers.length > 1`.
- `usable = computed(() => targets.value.some((x) => x.worktree !== ''))` (moved from the component).
- Helpers: `where(x) = t('common.dotPair.label', {first: x.repo, second: x.branch})`,
  `agentLabelOf(product) = t('space.ade.taskLaunch.agent.label', {agent: product})`,
  `hint(g) = g.reason ? t(g.reason) : undefined`.
- Item factories (ids double as testids via `menu-item-<id>`; keep today's suffixes):
  - `terminalItem(x, label)`: `{type:'item', id:'ade-task-launch-terminal-<branchId>', label, icon:'terminal-bash', disabled:x.terminal.disabled, hint:hint(x.terminal), run:() => openTerminal(x)}`.
  - `agentItem(x, label, provider?)`: id `ade-task-launch-agent-<branchId>` without provider,
    `ade-task-launch-agent-<provider>-<branchId>` with one; `icon:'robot'` (the ADE agent glyph,
    `AdeView.vue`, `AdeAgentsStatusItem.vue`); `disabled:x.agent.disabled`, `hint:hint(x.agent)`,
    `run:() => startAgent(x, provider ?? '')`.

`items()` maps today's four template branches one to one:

| Today | `MenuItem[]` |
|---|---|
| several, label "Terminal" + one item per target (`repo · branch`) | `{type:'label', label:t('common.module.terminal.label')}`, `terminalItem(x, where(x))` per target |
| several, separator | `{type:'separator'}` |
| several + choose: `DropdownMenuSub` per provider (`ade-task-launch-provider-<id>`) with one item per target | `{type:'submenu', id:'ade-task-launch-provider-<p.id>', label:agentLabelOf(p.productLabel), icon:'robot', items: targets.map(x => agentItem(x, where(x), p.id))}` per provider |
| several, one provider: label `agentLabel` + item per target | `{type:'label', label:agentLabelOf(info.productLabel)}`, `agentItem(x, where(x))` per target |
| one target: "Terminal" item | `terminalItem(x, t('common.module.terminal.label'))` |
| one target + choose: item per provider, gated by the target's agent gate | `agentItem(x, agentLabelOf(p.productLabel), p.id)` per provider |
| one target, one provider: agent item | `agentItem(x, agentLabelOf(info.productLabel))` |
| inline reason span `ade-task-launch-reason` | `hint` (tooltip on hover, `sr-only` text in the row, `aria-describedby`) |

"One target" uses `targets[0]` exactly as today (several = `targets.length > 1`). Return
`{ targets, usable, pending, items, openTerminal, startAgent }`. No new i18n keys; every key used
today stays used.

### 4. `sessions/AdeTaskLaunchMenu.vue`: thin

```vue
<MenuIconButton v-if="usable" :label="t('space.ade.taskLaunch.trigger.tooltip')" :items="items"
  :pending="pending" class="shrink-0" data-testid="ade-task-launch" />
<TooltipIconButton v-else ...unchanged disabled fallback... />
```

Drop every `DropdownMenu*` import, `Gate`/`LaunchTarget` imports and local computeds now in the
composable. Keep the one-line file comment.

Placement: the menu opens at the button's left edge, `align="start"`, like Runs. The "+" sits at
the bar's right end; `ContextMenu.vue`'s `prioritize-position` flip/shift keeps it in the viewport
(spec asserts `toBeInViewport`). Accepted as the Runs behaviour, which the row asks for.

Focus: the store captures focus at open and returns it to the "+" on close (same as Runs). An
agent pick that opens a dialog keeps focus there (`ContextMenu.onCloseAutoFocus`).

### Libraries

shadcn-vue `DropdownMenu` stays the primitive (via `ContextMenu.vue`/`MenuItems.vue`); Pinia
`useContextMenuStore` is the shared menu state; Tailwind only, no `<style>`. `<script setup lang="ts">`.
Nothing hand-rolled: the change removes a second dropdown rendering path.

## Tests (frontend only, no backend flow: IPC unchanged)

No unit test: item mapping is a flat branch, below the CLAUDE.md bar.

`apps/kira-space/tests/ui/ade-v2-task-launch.spec.ts`:
- `openMenu`: assert `[data-testid="context-menu"]` `toBeInViewport()` (was `ade-task-launch-menu`).
- Rename testids: `ade-task-launch-terminal-*` to `menu-item-ade-task-launch-terminal-*`,
  `ade-task-launch-agent-*` to `menu-item-ade-task-launch-agent-*`. A `t()`-level helper
  `item = (id) => t(\`menu-item-ade-task-launch-${id}\`)` keeps lines short.
- One worktree test: replace `ade-task-launch-reason` count 0 with: terminal and agent items have
  no `aria-describedby`.
- Reason assertions (`toContainText('Session open' | 'No worktree yet' | 'Setup failed' | 'Preparing worktree')`)
  keep passing via the `sr-only` hint text; keep them. `data-disabled` checks unchanged.
- New test `a disabled item explains itself on hover, like the Runs menu`: several-worktree fixture
  (as in the existing busy test), hover `menu-item-ade-task-launch-agent-b_auth`, expect
  `getByRole('tooltip')` to contain `Session open`; trigger has `aria-haspopup="menu"`.
- Archived/no-card tests unchanged (`ade-task-launch` testid kept on the trigger).

`apps/kira-space/tests/ui/agent-provider.spec.ts:128-129`: `ade-task-launch-agent-<p>-b_cart` to
`menu-item-ade-task-launch-agent-<p>-b_cart`.

Runs "+" (adopted, testid unchanged, must still pass): `apps/kira-space/tests/ui/automations-scripts.spec.ts`,
`apps/kira-studio/tests/ui/automations-module.spec.ts`, and the shared scenarios `automationsSmart`/`automationsRuns`
through `automations-smart.spec.ts` and `automations-runs.spec.ts` in both `apps/kira-space/tests/ui/` and `apps/kira-studio/tests/ui/`.

## Commits

1. `feat: shared MenuIconButton for the Runs add menu` — `MenuIconButton.vue`, `AutomationsPanel.vue`.
2. `feat: P497 task launch menu uses the shared dropdown` — `useTaskLaunch.ts`, `AdeTaskLaunchMenu.vue`,
   both specs.
3. `docs: P497 result` — `SPEC.md` P497 row Done + Result.

## Checks

- Fast: `bun run typecheck:space-web`, `bun run typecheck:space-tests`, `bun run typecheck:web:studio`
  (workbench is shared), `bun run lint`. Pre-commit hook runs these anyway.
- Own specs:
  `bun run build:test:space && npx playwright test --config=apps/kira-space/playwright.config.ts --project=ui ade-v2-task-launch agent-provider automations-scripts automations-smart automations-runs`
  and `bun run build:test:studio && npx playwright test --config=apps/kira-studio/playwright.config.ts --project=ui automations-module automations-smart automations-runs`.
- Grep after: `grep -rn "ade-task-launch-menu\|ade-task-launch-reason\|onAddClick" apps packages`
  returns nothing (outside `docs/`).
- Full UI suite not run here (once per chapter, after P502).

## File ownership and overlap

P497 owns:
- `packages/workbench/src/components/MenuIconButton.vue` (new)
- `packages/workbench/src/automations/AutomationsPanel.vue`
- `apps/kira-space/frontend/src/ade/v2/sessions/useTaskLaunch.ts`
- `apps/kira-space/frontend/src/ade/v2/sessions/AdeTaskLaunchMenu.vue`
- `apps/kira-space/tests/ui/ade-v2-task-launch.spec.ts`
- `apps/kira-space/tests/ui/agent-provider.spec.ts` (two lines, 128-129)
- `docs/v0.2.30/SPEC.md` P497 row only
- `packages/theme/src/i18n/en.json`: no change planned. If lint's unused-key check flags a
  `space.ade.taskLaunch.*` key, touch that key only.

Read, not edited: `board/taskLaunch.ts`, `state/contextMenu.ts`, `components/ContextMenu.vue`,
`components/MenuItems.vue`, `AdeSessionsTab.vue`, `AutomationsNewTab.vue`, `TabStripNewButton.vue`.

Overlap:
- P496 (`panel/AdeTaskTab.vue`, `en.json` key `space.ade.taskTab.handover`, `ade-v2-handover` spec;
  committed plan `3cd4c065`): no common code file, no common spec. `en.json`: P497 plans no edit,
  any edit stays in `space.ade.taskLaunch.*`, disjoint. `SPEC.md`: row-local hunks. No ordering
  dependency either way; safe as parallel streams.
- P500 (`plan/AdeTaskCard.vue`, `AdeActionCell.vue`, `AdeBranchRow.vue`, `board/actions.ts`, card
  specs): no common file. P500 may add card/branch-row menus through `useContextMenuStore`; P497
  does not change the store or `MenuItems.vue`, so no conflict. `ade-v2-task-launch` asserts no
  "+" on cards (`ade-card ade-task-launch` count 0); P500 must not add a `ade-task-launch` testid
  to the card. No ordering dependency. SPEC keeps P500 sequential for its own reasons (shares
  `board/actions.ts` with P495), not because of P497.
