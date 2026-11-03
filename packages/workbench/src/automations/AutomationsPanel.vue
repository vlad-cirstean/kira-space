<script setup lang="ts">
import type { CustomScript, ScriptFileProblem, ScriptKind } from '@shared/domain/scripts';
import EmptyState from '@theme/components/EmptyState.vue';
import InlineState from '@theme/components/InlineState.vue';
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { useEventListener } from '@vueuse/core';
import { useAgentInfo } from '@workbench/agent/agentInfo';
import { useCopy } from '@workbench/behaviour/clipboard';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { matchesQuery } from '@workbench/behaviour/match';
import { usePanelHeaderSearch } from '@workbench/behaviour/panelSearch';
import { usePersistedState } from '@workbench/behaviour/persisted';
import { moveId } from '@workbench/behaviour/sortableReorder';
import { useTreeShortcuts } from '@workbench/behaviour/treeShortcuts';
import PanelBar from '@workbench/components/PanelBar.vue';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import SortableList from '@workbench/components/SortableList.vue';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref, useTemplateRef } from 'vue';
import { errorText } from '../bridge/codedError';
import { useAutomationsModule } from './module';
import { groupMenu, newScriptItems, type ScriptMenuActions, scriptMenu } from './panelMenus';
import RunFileDialog from './RunFileDialog.vue';
import RunsRestoreBanner from './restore/RunsRestoreBanner.vue';
import { useRunScript } from './runScript';
import RunsSection from './runs/RunsSection.vue';
import { useLiveRunByScript } from './runs/runsQueries';
import ScriptBrokenRow from './ScriptBrokenRow.vue';
import ScriptGroupHeader from './ScriptGroupHeader.vue';
import ScriptRow from './ScriptRow.vue';
import { useScheduleConfirmStore } from './schedule/confirmDialog';
import ScheduleTargetDialog from './schedule/ScheduleTargetDialog.vue';
import { runFileName, useRemoveScript } from './scriptActions';
import { useScriptTabs } from './scriptTabs';
import { defaultSmart } from './smart/smartSettings';

const contextMenuStore = useContextMenuStore();
const ctx = useAutomationsModule();
const removeScript = useRemoveScript();
const { info: agentInfo } = useAgentInfo();

// The Automations module's own left panel: a view over the run files through the `ctx.scripts`
// seam, both apps. A run is a file; the header `+` writes a new one and a row opens it as text.
const scripts = ctx.scripts;

// The run file open in the dialog, and the run whose schedule target is being set.
const fileDialog = ref<string | null>(null);
const targetFor = ref<CustomScript | null>(null);

const search = ref('');
const actionError = ref<string | null>(null);

interface ScriptGroup {
  name: string;
  rows: readonly CustomScript[];
}

function matches(script: CustomScript, q: string): boolean {
  return matchesQuery(script.name, q) || matchesQuery(script.command, q);
}

// Panel search filters rows by name and command; a collection shows when its name matches (all
// its rows) or any of its rows does.
const view = computed(() => {
  const q = search.value.trim().toLowerCase();
  const known = new Set(scripts.collections().map((c) => c.name));
  const records = scripts.records();
  const ungrouped = records.filter(
    (s) => (s.collection === '' || !known.has(s.collection)) && (q === '' || matches(s, q)),
  );
  const groups: ScriptGroup[] = [];
  for (const collection of scripts.collections()) {
    const own = records.filter((s) => s.collection === collection.name);
    const nameHit = q !== '' && matchesQuery(collection.name, q);
    const rows = q === '' || nameHit ? own : own.filter((s) => matches(s, q));
    if (q === '' || nameHit || rows.length > 0) groups.push({ name: collection.name, rows });
  }
  const ids = new Set(records.map((s) => s.id));
  const broken = scripts
    .problems()
    .filter((p) => !ids.has(p.id) && (q === '' || matchesQuery(p.file, q)));
  return { ungrouped, groups, broken };
});

const problemById = computed(
  () => new Map(scripts.problems().map((p): [string, ScriptFileProblem] => [p.id, p])),
);

const collapsed = usePersistedState<string[]>('kira.automations.collapsedCollections', [], {
  migrateFrom: 'kira.quickCommands.collapsedCollections',
});

// A search shows every matching row, collapsed or not.
function isOpen(name: string): boolean {
  return search.value.trim() !== '' || !collapsed.value.includes(name);
}

function toggleGroup(name: string): void {
  collapsed.value = collapsed.value.includes(name)
    ? collapsed.value.filter((n) => n !== name)
    : [...collapsed.value, name];
}

function openGroup(name: string): void {
  collapsed.value = collapsed.value.filter((n) => n !== name);
}

// A search shows a filtered subset, so it pauses drag, Alt+arrow and the Move up/down items.
const reorderOff = computed(() => search.value.trim() !== '');
const ungroupedIds = computed(() => view.value.ungrouped.map((s) => s.id));
const groupNames = computed(() => view.value.groups.map((g) => g.name));

function reorderSet(collection: string, ids: readonly string[], from: string, to: string): void {
  void guarded(() => scripts.reorder(collection, moveId(ids, from, to)));
}

function reorderGroups(from: string, to: string): void {
  void guarded(() => scripts.reorderCollections(moveId(groupNames.value, from, to)));
}

const empty = computed(() => scripts.records().length === 0 && scripts.problems().length === 0);

async function guarded(fn: () => Promise<unknown>): Promise<void> {
  actionError.value = null;
  try {
    await fn();
  } catch (err) {
    actionError.value = errorText(err);
  }
}

const rootEl = useTemplateRef<HTMLElement>('rootEl');
const { showSearch, toggleSearch } = usePanelHeaderSearch(rootEl, {
  searchable: () => true,
  getSearch: () => search.value,
  setSearch: (v) => {
    search.value = v;
  },
});

// A new run starts from a small valid example and opens as text to edit.
async function createScript(kind: ScriptKind, collection = ''): Promise<void> {
  const smart = kind === 'smart';
  await guarded(async () => {
    const created = await scripts.create({
      name: t(smart ? 'automations.script.newSmart.label' : 'automations.script.new.label'),
      command: t(
        smart
          ? 'automations.script.promptExample.example'
          : 'automations.script.commandExample.example',
      ),
      kind,
      params: [],
      smart: smart ? defaultSmart(agentInfo.value.defaultModel) : null,
      schedule: null,
      dirMode: 'kira',
      workingDir: '',
      color: 'none',
      collection,
      useAdeDir: false,
    });
    if (created.collection !== '') openGroup(created.collection);
    fileDialog.value = runFileName(created.id);
  });
}

// The header "+" opens a dropdown under the button, the tab strip "+" pattern.
function onAddClick(e: MouseEvent): void {
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
  contextMenuStore.openContextMenuAt(rect.left, rect.bottom + 2, newScriptItems(createScript));
}

interface NavItem {
  key: string;
  kind: 'script' | 'group' | 'broken';
  id: string;
  depth: 0 | 1;
}

const navItems = computed<NavItem[]>(() => [
  ...view.value.ungrouped.map(
    (s): NavItem => ({ key: `script:${s.id}`, kind: 'script', id: s.id, depth: 0 }),
  ),
  ...view.value.broken.map(
    (p): NavItem => ({ key: `broken:${p.file}`, kind: 'broken', id: p.file, depth: 0 }),
  ),
  ...view.value.groups.flatMap(({ name, rows }): NavItem[] => [
    { key: `group:${name}`, kind: 'group', id: name, depth: 0 },
    ...(isOpen(name)
      ? rows.map((s): NavItem => ({ key: `script:${s.id}`, kind: 'script', id: s.id, depth: 1 }))
      : []),
  ]),
]);

const treeEl = useTemplateRef<HTMLElement>('treeEl');
const activeKey = ref<string | null>(null);

function scriptById(id: string): CustomScript | undefined {
  return scripts.records().find((s) => s.id === id);
}

function editFile(script: CustomScript): void {
  fileDialog.value = runFileName(script.id);
}

function openItem(item: NavItem): void {
  if (item.kind === 'group') toggleGroup(item.id);
  else if (item.kind === 'broken') fileDialog.value = item.id;
  else {
    const script = scriptById(item.id);
    if (script) editFile(script);
  }
}

function groupSize(name: string): number {
  return view.value.groups.find((g) => g.name === name)?.rows.length ?? 0;
}

const nav = useListNavigation<NavItem>({
  container: treeEl,
  items: () => navItems.value,
  key: (i) => i.key,
  activeKey,
  onOpen: openItem,
  tree: {
    depth: (i) => i.depth,
    isExpanded: (i) => i.kind === 'group' && isOpen(i.id),
    hasChildren: (i) => i.kind === 'group' && groupSize(i.id) > 0,
    expand: (i) => openGroup(i.id),
    collapse: (i) => {
      if (!collapsed.value.includes(i.id)) collapsed.value = [...collapsed.value, i.id];
    },
  },
  rendered: () => navItems.value,
});

const { copy } = useCopy();
useTreeShortcuts(
  { container: treeEl, activeKey },
  {
    'tree.copyName': (key) => {
      const item = navItems.value.find((i) => i.key === key);
      const name = item?.kind === 'script' ? scriptById(item.id)?.name : item?.id;
      if (name !== undefined) void copy(name);
    },
    'tree.delete': (key) => {
      const item = navItems.value.find((i) => i.key === key);
      const script = item?.kind === 'script' ? scriptById(item.id) : undefined;
      if (script) void onRemove(script);
    },
  },
);

const confirmStore = useScheduleConfirmStore();
const startScript = useRunScript();
const scriptTabs = useScriptTabs();
async function runScript(script: CustomScript): Promise<void> {
  actionError.value = await startScript(script);
}

const liveRunByScript = useLiveRunByScript();

// Both surfaces named: a script removed here also stops launching from the tab strip.
async function onRemove(script: CustomScript): Promise<void> {
  await guarded(async () => {
    await removeScript(scripts, script);
  });
}

function openMenu(item: NavItem, e: MouseEvent, items: MenuItem[]): void {
  void nav.focusKey(item.key);
  nav.contextMenu(item, e, items);
}

function navItemOf(id: string): NavItem {
  return (
    navItems.value.find((i) => i.kind === 'script' && i.id === id) ?? {
      key: `script:${id}`,
      kind: 'script',
      id,
      depth: 0,
    }
  );
}

// Alt+arrow stays on the lists' keyboard handling; these items run the same swap by menu.
function setIdsOf(script: CustomScript): readonly string[] {
  const group = view.value.groups.find((g) => g.name === script.collection);
  return group ? group.rows.map((r) => r.id) : ungroupedIds.value;
}

function stepIds(ids: readonly string[], id: string, step: -1 | 1): string[] | null {
  const neighbour = ids[ids.indexOf(id) + step];
  return ids.includes(id) && neighbour ? moveId(ids, id, neighbour) : null;
}

const scriptActions: ScriptMenuActions = {
  run: runScript,
  runNow: (script) => {
    const { taskId, branchId } = script.schedule ?? { taskId: '', branchId: '' };
    if (!scriptTabs.focusLive({ scriptId: script.id, taskId, branchId })) {
      confirmStore.openNow(script.id);
    }
  },
  editFile,
  openFile: (script) => guarded(() => ctx.files.open(runFileName(script.id))),
  revealFile: (script) => guarded(() => ctx.files.reveal(runFileName(script.id))),
  copyPath: (script) =>
    guarded(async () => copy((await scripts.readFile(runFileName(script.id))).path)),
  editTarget: scripts.setScheduleTarget
    ? (script) => {
        targetFor.value = script;
      }
    : undefined,
  remove: onRemove,
  canMoveStep: (script, step) =>
    !reorderOff.value && stepIds(setIdsOf(script), script.id, step) !== null,
  moveStep: (script, step) => {
    const order = stepIds(setIdsOf(script), script.id, step);
    if (order) void guarded(() => scripts.reorder(script.collection, order));
  },
};

function onGroupMenu(e: MouseEvent, name: string): void {
  const step = (by: -1 | 1) => {
    const order = stepIds(groupNames.value, name, by);
    if (order) void guarded(() => scripts.reorderCollections(order));
  };
  openMenu(
    { key: `group:${name}`, kind: 'group', id: name, depth: 0 },
    e,
    groupMenu((kind) => createScript(kind, name), {
      step,
      can: (by) => !reorderOff.value && stepIds(groupNames.value, name, by) !== null,
    }),
  );
}

function onContextMenu(e: MouseEvent, script: CustomScript): void {
  openMenu(navItemOf(script.id), e, scriptMenu(script, scriptActions));
}

// Right-click on empty space (the empty state included), the API tree's own pattern.
const bodyEl = useTemplateRef<HTMLElement>('bodyEl');
useEventListener(bodyEl, 'contextmenu', (e: MouseEvent) => {
  e.preventDefault();
  contextMenuStore.openContextMenu(e, newScriptItems(createScript));
});
</script>

<template>
  <div class="h-full" data-testid="automations-panel">
    <div ref="rootEl" class="flex h-full flex-col">
      <PanelHeader>
        {{ t('common.module.automations.label') }}
        <template #actions>
          <TooltipIconButton
            icon="search"
            :label="
              showSearch ? t('common.action.hideSearch.label') : t('common.action.search.label')
            "
            :pressed="showSearch"
            data-testid="toggle-search"
            @click="toggleSearch"
          />
          <TooltipIconButton
            icon="info"
            :label="t('common.config.about.label')"
            data-testid="automations-about"
            @click="guarded(() => ctx.files.open('AGENTS.md'))"
          />
          <TooltipIconButton
            icon="add"
            :label="t('automations.panel.add.tooltip')"
            :aria-label="t('automations.script.new.label')"
            data-testid="automations-add"
            @click="onAddClick"
          />
        </template>
      </PanelHeader>
      <RunsRestoreBanner />
      <PanelBar v-if="!empty && showSearch">
        <SearchField v-model="search" data-testid="tree-search" />
      </PanelBar>
      <div ref="bodyEl" class="flex min-h-0 flex-1 flex-col" data-testid="automations-body">
        <div v-if="!empty" class="flex h-full flex-col overflow-y-auto">
          <InlineState v-if="actionError" kind="error" data-testid="script-error">{{
            actionError
          }}</InlineState>
          <div ref="treeEl" class="flex flex-col" data-testid="script-list">
            <SortableList
              :ids="ungroupedIds"
              draggable='[data-nav-key^="script:"]'
              :disabled="reorderOff"
              class="flex flex-col"
              @move="(from, to) => reorderSet('', ungroupedIds, from, to)"
            >
              <ScriptRow
                v-for="script in view.ungrouped"
                :key="script.id"
                :script="script"
                :depth="0"
                :live-run="liveRunByScript.get(script.id)"
                :problem="problemById.get(script.id)"
                v-bind="nav.rowProps(`script:${script.id}`)"
                :selected="activeKey === `script:${script.id}`"
                :live="scriptTabs.isLive({ scriptId: script.id, taskId: '', branchId: '' })"
                @run="runScript(script)"
                @open="editFile(script)"
                @menu="onContextMenu($event, script)"
              />
            </SortableList>
            <ScriptBrokenRow
              v-for="problem in view.broken"
              :key="problem.file"
              :problem="problem"
              v-bind="nav.rowProps(`broken:${problem.file}`)"
              :selected="activeKey === `broken:${problem.file}`"
              @open="fileDialog = problem.file"
            />
            <SortableList
              :ids="groupNames"
              draggable="[data-group-entry]"
              handle='[data-testid="script-group"]'
              keyboard-handle='[data-testid="script-group"]'
              :disabled="reorderOff"
              class="flex flex-col"
              @move="reorderGroups"
            >
              <div
                v-for="group in view.groups"
                :key="group.name"
                data-group-entry
                class="flex flex-col"
              >
                <ScriptGroupHeader
                  :name="group.name"
                  :count="group.rows.length"
                  :open="isOpen(group.name)"
                  v-bind="nav.rowProps(`group:${group.name}`)"
                  @toggle="toggleGroup(group.name)"
                  @menu="onGroupMenu($event, group.name)"
                />
                <SortableList
                  v-if="isOpen(group.name)"
                  :ids="group.rows.map((s) => s.id)"
                  draggable='[data-nav-key^="script:"]'
                  :disabled="reorderOff"
                  class="flex flex-col"
                  @move="
                    (from, to) =>
                      reorderSet(
                        group.name,
                        group.rows.map((s) => s.id),
                        from,
                        to,
                      )
                  "
                >
                  <ScriptRow
                    v-for="script in group.rows"
                    :key="script.id"
                    :script="script"
                    :depth="1"
                    :live-run="liveRunByScript.get(script.id)"
                    :problem="problemById.get(script.id)"
                    v-bind="nav.rowProps(`script:${script.id}`)"
                    :selected="activeKey === `script:${script.id}`"
                    :live="scriptTabs.isLive({ scriptId: script.id, taskId: '', branchId: '' })"
                    @run="runScript(script)"
                    @open="editFile(script)"
                    @menu="onContextMenu($event, script)"
                  />
                </SortableList>
              </div>
            </SortableList>
          </div>
        </div>
        <EmptyState
          v-else
          fill
          icon="terminal-bash"
          :title="t('automations.panel.none.empty')"
          :description="t('automations.panel.none.emptyHint')"
        >
          <FieldError v-if="actionError" data-testid="script-error">{{ actionError }}</FieldError>
        </EmptyState>
      </div>
      <RunsSection />
      <RunFileDialog v-if="fileDialog" :file="fileDialog" @close="fileDialog = null" />
      <ScheduleTargetDialog v-if="targetFor" :script="targetFor" @close="targetFor = null" />
    </div>
  </div>
</template>
