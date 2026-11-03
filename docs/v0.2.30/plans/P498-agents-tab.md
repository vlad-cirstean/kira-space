# P498 plan: one Agents tab for Context and Settings

Row: `SPEC.md` P498. Frontend only, IPC unchanged, so no backend test.

## Today (verified with `codegraph_explore` and grep)

- `shell/AdeShell.vue` (paths under `apps/kira-space/frontend/src/ade/v2/` unless full):
  right `TabsList` holds `context` (`ade-tab-context`, label `space.ade.context.tab.label`),
  `settings` (`ade-tab-settings`, label `common.module.settings.label`), `workflows`. `onView`
  calls `ctxUi.leave()` / `setUi.leave()` before leaving. Body: `<AdeContextPage v-else-if>` and
  `<AdeSettingsPage v-else-if>`.
- `state/adeBoardUi.ts:13`: `view` union holds `'context' | 'settings'`. Only `AdeShell.vue` reads
  those two values; nothing else assigns them.
- `context/AdeContextPage.vue` and `settings/AdeSettingsPage.vue` repeat lines 25-98 / 27-100
  byte for byte: `draft`, `dirty`, `baseHash`, `saveError`, `conflict`, `view`, `bytes`,
  `actionable`, `over`, the `view` watch, the `dirty` mirror into the store, `saveNow`, `resetTo`,
  `discard`, `reload`, `overwrite`, `onInput`, unmount reset. Each wraps itself in `paneFrame` +
  `PanelHeader`.
- `state/adeContextUi.ts` and `state/adeSettingsUi.ts`: identical stores (`dirty`, `leave()`), only
  the confirm body key differs (`space.ade.context.discardConfirm.body` /
  `space.ade.settings.discardConfirm.body`).
- `frontend/src/App.vue:128-133`: leaving ADE mode awaits workflows, context and settings `leave()`.
- Precedent: `state/adeWorkflowsUi.ts` keeps page sub-state (`pane`) and its guard in one store,
  with `openStatuses()` guarding the in-page switch. Follow it.
- `PanelHeader` has a `start` slot for view tabs. `SecondaryTabs` is the one sub-tab element.

## Label

Top tab label: **Agent config**. "Agents" is already the mode's own name (mobile heading
`mobile.shell.heading.title`, docs page `space/agents.mdx` title), so a tab named Agents inside
Agents reads as a loop. Internal names follow the row: view value `agents`, testid
`ade-tab-agents`, store `adeAgentsUi`. Sub-tab labels reuse `space.ade.context.tab.label`
("Context") and `common.module.settings.label` ("Settings").

## Design

### 1. Store: `state/adeAgentsUi.ts` (new), replaces both old stores

```ts
export type AgentsTab = 'context' | 'settings';
// Agent config page state and its leave guard, in memory only.
export const useAdeAgentsUiStore = defineStore('adeAgentsUi', () => {
  const tab = ref<AgentsTab>('context');
  /** The open editor (context or settings) holds edits that are not saved. */
  const dirty = ref(false);
  async function leave(): Promise<boolean>  // confirm body by tab.value:
      // 'context' -> space.ade.context.discardConfirm.body, else space.ade.settings.discardConfirm.body
      // title/confirmLabel/tone exactly as today; sets dirty=false on discard
  async function openTab(next: AgentsTab): Promise<void> // next===tab: return; !(await leave()): return; tab=next
  return { tab, dirty, leave, openTab };
});
```

One `dirty` suffices: only one editor is mounted at a time (`v-if`), and switching away always
passes `leave()`. Delete `state/adeContextUi.ts` and `state/adeSettingsUi.ts` (`git rm`).

### 2. Composable: `agents/useAgentFileEditor.ts` (new)

Moves the shared editor logic out of both pages, verbatim in behaviour.

```ts
interface FileView { text: string; hash: string; maxBytes: number }
interface SaveArgs { text: string; baseHash: string }
export function useAgentFileEditor<V extends FileView>(
  query: { data: Ref<V | undefined>; refetch: () => Promise<{ data: V | undefined }> },
  save: { mutateAsync: (args: SaveArgs) => Promise<V>; isPending: Ref<boolean> },
)
```

Body: today's `draft`/`dirty`/`baseHash`/`saveError`/`conflict` refs, `view`/`bytes`/`actionable`/
`over` computeds, the immediate `view` watch, `watch(dirty, v => agentsUi.dirty = v)`,
`onBeforeUnmount(() => agentsUi.dirty = false)`, `saveNow`, `resetTo` (private), `discard`,
`reload`, `overwrite`, `onInput`. Returns `{ view, draft, dirty, saveError, conflict, bytes,
actionable, over, saveNow, discard, reload, overwrite, onInput }`. Callers pass the TanStack
`useAgentContext()` / `useSaveAgentContext()` (and settings equivalents) objects as they are; the
structural types above accept them. If `vue-tsc` rejects the structural `refetch` type, type the
params as `UseQueryReturnType<V, Error>` / `UseMutationReturnType<V, Error, SaveArgs, unknown>`
from `@tanstack/vue-query` instead; no casts. No unit test (CLAUDE.md bar: thin logic moved, not new).

### 3. Panes (rename, strip frame)

- `git mv context/AdeContextPage.vue context/AdeContextPane.vue`,
  `git mv settings/AdeSettingsPage.vue settings/AdeSettingsPane.vue`. They are no longer pages.
- Script: drop the store import and every moved line; keep the query/mutation calls, pass them
  to `useAgentFileEditor`, keep `root` + `useShortcut(root, ['form.save'], …)` in the pane.
  Template keeps `save.isPending.value` for the Save button's `:pending`.
- Template: remove the outer `flex … gap-0.5` div, the `paneFrame` div and `PanelHeader`. Root
  becomes `<div class="min-h-0 flex-1 overflow-auto p-4" data-testid="ade-context">` (settings:
  `ade-settings`). Everything inside stays, every inner testid unchanged. Settings keeps its
  two-column grid, `AdeSettingsSecrets` and `AdeSettingsResult`.
- Drop now-unused imports (`paneFrame`, `cn`, `PanelHeader`, `onBeforeUnmount`, `watch`, `ref`,
  `computed` as applicable); biome flags leftovers.

### 4. Page: `agents/AdeAgentsPage.vue` (new)

```vue
<div class="flex min-h-0 min-w-0 flex-1 gap-0.5" data-testid="ade-agents">
  <div :class="cn(paneFrame, 'flex flex-1 flex-col')">
    <PanelHeader>
      <template #start>
        <SecondaryTabs :model-value="agentsUi.tab" :items="items"
          :aria-label="t('space.ade.agents.tab.label')"
          @update:model-value="(v) => void agentsUi.openTab(v as AgentsTab)" />
      </template>
    </PanelHeader>
    <AdeContextPane v-if="agentsUi.tab === 'context'" />
    <AdeSettingsPane v-else />
  </div>
</div>
```

`items`: `[{ value: 'context', label: t('space.ade.context.tab.label'), testid:
'ade-agents-tab-context' }, { value: 'settings', label: t('common.module.settings.label'),
testid: 'ade-agents-tab-settings' }]` (a plain `const`; labels do not change at runtime, but use
`computed` if `t` must track locale like other call sites do). `SecondaryTabs` is controlled
(`model-value`), so a refused guard leaves the old sub-tab selected, same as `AdeShell`'s `Tabs`.

### 5. Shell and app

- `state/adeBoardUi.ts:13`: union `'plan' | 'later' | 'needs' | 'agents' | 'workflows'`.
- `shell/AdeShell.vue`: replace the two triggers with one `TabsTrigger value="agents"`
  (`ade-tab-agents`, label `space.ade.agents.tab.label`), before `workflows`. Replace the two
  store imports/consts with `useAdeAgentsUiStore`; `onView` line becomes
  `if (ui.view === 'agents' && !(await agentsUi.leave())) return;`. Body: one
  `<AdeAgentsPage v-else-if="ui.view === 'agents'" />`; drop both page imports.
- `App.vue:30-32,128-133`: replace the two store imports and calls with one
  `useAdeAgentsUiStore().leave()`.
- Programmatic jumps (`openTask`, `openSession`, `openSessions` in `adeBoardUi.ts`) set `view`
  without any guard today, Workflows included. Unchanged here: same behaviour as today for all
  three editors, outside this row.

### 6. i18n (`packages/theme/src/i18n/en.json`)

P498 owns `space.ade.agents.*`. Add `space.ade.agents.tab.label` = `"Agent config"`. No key
becomes unused: `space.ade.context.tab.label`, `common.module.settings.label` and both
`discardConfirm.body` keys keep callers. `check-catalog --unused` (in `bun run lint`) confirms.
Touch no other subtree.

### 7. Docs

- `site/src/content/docs/docs/space/agents.mdx:230,240,292`: "The **Context** tab in Agents" ->
  "The **Context** sub-tab of **Agent config**"; same for **Settings** (two lines).
- `docs/ARCHITECTURE.md:3326`: "Context tab in Agents" -> "Context sub-tab of the Agent config tab".

## Tests (split at IPC: frontend half only; IPC unchanged, so no backend flow)

Update to the new tab (`ade-tab-agents`, then the sub-tab):

- `tests/ui/ade-agent-context.spec.ts`: `openContext` (line 34) clicks `ade-tab-agents`, then
  `ade-agents-tab-context`; the reload test (line 67) the same.
- `tests/ui/ade-agent-settings.spec.ts`: `openSettings` (line 26) and the reload test (line 89)
  click `ade-tab-agents`, then `ade-agents-tab-settings`.
- `tests/ui/ade-agent-port.spec.ts:43`: same as settings.
- `tests/ui/ade-agent-missing.spec.ts` and `tests/ui/agent-provider.spec.ts`: grep shows neither
  touches `ade-tab-context`/`ade-tab-settings` or the two pages (launch dialog and the app
  Settings dialog only). No edit; run both to confirm they still pass.
- Any other hit of `ade-tab-context|ade-tab-settings` in `apps/kira-space/tests` (grep at
  implementation time; none besides the three above today).

New: `tests/ui/ade-agents-tab.spec.ts`, one test, "unsaved edits ask before switching sub-tab or
leaving Agent config". Mocks `IPC.agentContextGet` (inline view as in `ade-agent-context.spec.ts`)
and `IPC.agentSettingsGet` (`contract('agent-settings', 'view')`). Steps:
1. Click `ade-tab-agents`: `ade-context` visible (default sub-tab).
2. Clean editor: click `ade-agents-tab-settings` -> `ade-settings` visible, `confirm-dialog`
   count 0. Click `ade-agents-tab-context` back.
3. Fill `ade-context-text`; click `ade-agents-tab-settings` -> `confirm-dialog-message` contains
   the context discard body; `confirm-dialog-cancel` -> `ade-context-text` keeps the draft,
   `ade-settings` count 0.
4. Click `ade-agents-tab-settings` again, `confirm-dialog-confirm` -> `ade-settings` visible.
5. Fill `ade-settings-text`; click `ade-tab-plan` -> dialog with the settings body; cancel ->
   `ade-settings` still visible; click `ade-tab-plan`, confirm -> `ade-agents` count 0.

Pattern for confirm steps: `ade-v2-workflows.spec.ts:140-171`.

## Fast checks (per commit)

`bun run typecheck:space-web`, `bun run typecheck:space-tests`, `bun run lint` (biome, catalog
`--unused`, check-docs). Then only the changed/new specs:
`bun run build:test:space && bunx playwright test --config=apps/kira-space/playwright.config.ts
--project=ui ade-agent-context ade-agent-settings ade-agent-port ade-agent-missing
agent-provider ade-agents-tab`. Full suite waits for the chapter's single run.

## Commits

1. `refactor: one agent config store and editor composable` (store, composable, panes, page,
   shell, App.vue, adeBoardUi, en.json).
2. `test: agent config tab specs` (spec updates + new spec).
3. `docs: agent config tab` (agents.mdx, ARCHITECTURE.md).

## File ownership (for stream planning)

P498 owns, exclusively:
- `apps/kira-space/frontend/src/ade/v2/shell/AdeShell.vue`
- `apps/kira-space/frontend/src/ade/v2/state/adeBoardUi.ts` (line 13 only)
- `apps/kira-space/frontend/src/ade/v2/state/adeAgentsUi.ts` (new)
- `apps/kira-space/frontend/src/ade/v2/state/adeContextUi.ts` (deleted)
- `apps/kira-space/frontend/src/ade/v2/state/adeSettingsUi.ts` (deleted)
- `apps/kira-space/frontend/src/ade/v2/agents/AdeAgentsPage.vue` (new)
- `apps/kira-space/frontend/src/ade/v2/agents/useAgentFileEditor.ts` (new)
- `apps/kira-space/frontend/src/ade/v2/context/AdeContextPage.vue` -> `AdeContextPane.vue`
- `apps/kira-space/frontend/src/ade/v2/settings/AdeSettingsPage.vue` -> `AdeSettingsPane.vue`
- `apps/kira-space/frontend/src/App.vue` (store import + mode-leave guard lines only)
- `apps/kira-space/tests/ui/ade-agent-context.spec.ts`, `ade-agent-settings.spec.ts`,
  `ade-agent-port.spec.ts`, `ade-agents-tab.spec.ts` (new)
- `site/src/content/docs/docs/space/agents.mdx` (lines 230, 240, 292)
- `docs/ARCHITECTURE.md` (line 3326)

Shared:
- `packages/theme/src/i18n/en.json`: P498 adds only `space.ade.agents.tab.label`; edits no
  existing key.

Overlap check against other rows' named files:
- P495 (`board/actions.ts`, `board/menu.ts`, `plan/useTaskMenu.ts`, `panel/AdeTaskPanel.vue`,
  `mobile/state/useTaskActions.ts`, task-menu/archive/later/task-flow specs): none shared.
- P496 (`panel/AdeTaskTab.vue`, `ade-v2-handover` spec): none.
- P497 (`sessions/AdeTaskLaunchMenu.vue`, `packages/workbench/src/automations/*`, launch specs):
  none.
- P499 (Go review store, `packages/git-ipc`, `packages/git-ui`, `review/`): none, unless P499's
  plan edits `App.vue` (it mounts `AdeReviewWindow`) or `docs/ARCHITECTURE.md`; P498 touches only
  the lines named above, so a rebase conflict there would be textual, not semantic. P499's
  stream plan should state whether it edits either file.
- P500-P502 (card, panel, branch row): none.
- No ordering dependency: P498 reads nothing another row changes.
