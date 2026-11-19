<script setup lang="ts">
import AttributeTooltip from '@theme/components/AttributeTooltip.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import { t } from '@theme/i18n';
import { copyText } from '@workbench/behaviour/clipboard';
import { compareNatural, matchesQuery } from '@workbench/behaviour/match';
import QueryState from '@workbench/components/QueryState.vue';
import VirtualList from '@workbench/components/VirtualList.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref } from 'vue';
import { isActiveState } from '../lib/format';
import { type ContainerAction, useContainerAction, useContainers } from '../queries';
import { useDockerUiStore } from '../state/dockerUi';
import type { DockerContainer } from '../wire';
import ContainerGroupRow from './ContainerGroupRow.vue';
import ContainerRow from './ContainerRow.vue';

const { single: groupRowHeight, double: containerRowHeight } = useRowHeight();

interface GroupRow {
  kind: 'group';
  key: string;
  name: string;
  members: DockerContainer[];
}
interface ContainerItem {
  kind: 'container';
  key: string;
  container: DockerContainer;
  grouped: boolean;
}
type Row = GroupRow | ContainerItem;

const ui = useDockerUiStore();
const containers = useContainers();
const actions = useContainerAction();
const contextMenu = useContextMenuStore();

const searching = computed(() => ui.search.trim() !== '');

const visible = computed(() => {
  return (containers.data.value ?? []).filter((c) => {
    if (!ui.showStopped && !isActiveState(c.state)) return false;
    return [c.name, c.image, c.composeProject, c.composeService].some((v) =>
      matchesQuery(v, ui.search),
    );
  });
});

const rows = computed<Row[]>(() => {
  const byProject = new Map<string, DockerContainer[]>();
  for (const c of visible.value) {
    const list = byProject.get(c.composeProject) ?? [];
    list.push(c);
    byProject.set(c.composeProject, list);
  }
  const byName = (a: DockerContainer, b: DockerContainer) => compareNatural(a.name, b.name);
  const out: Row[] = [];
  const projects = [...byProject.keys()].filter((p) => p !== '').sort(compareNatural);
  for (const project of projects) {
    const members = (byProject.get(project) ?? []).sort(byName);
    out.push({ kind: 'group', key: `g:${project}`, name: project, members });
    if (searching.value || !ui.collapsedGroups.includes(project)) {
      for (const container of members) {
        out.push({ kind: 'container', key: container.id, container, grouped: true });
      }
    }
  }
  for (const container of (byProject.get('') ?? []).sort(byName)) {
    out.push({ kind: 'container', key: container.id, container, grouped: false });
  }
  return out;
});

const rowHeights = computed(() =>
  rows.value.map((r) => (r.kind === 'group' ? groupRowHeight.value : containerRowHeight.value)),
);

const emptyCopy = computed(() => {
  if (searching.value)
    return {
      title: t('common.state.noMatches.empty'),
      hint: t('docker.container.noMatches.emptyHint'),
    };
  if ((containers.data.value ?? []).length > 0 && !ui.showStopped) {
    return {
      title: t('docker.container.noRunning.empty'),
      hint: t('docker.container.noRunning.emptyHint'),
    };
  }
  return { title: t('docker.container.none.empty'), hint: t('docker.container.none.emptyHint') };
});

function isOpen(name: string): boolean {
  return searching.value || !ui.collapsedGroups.includes(name);
}

function isSelected(c: DockerContainer): boolean {
  return ui.selection?.kind === 'container' && ui.selection.id === c.id;
}

function act(c: DockerContainer, action: ContainerAction): void {
  actions.run(c.id, action).catch(() => {
    // Surfaced through `actions.error`.
  });
}

function groupAction(project: string, action: 'start' | 'stop'): void {
  const group = rows.value.find((r): r is GroupRow => r.kind === 'group' && r.name === project);
  const targets = (group?.members ?? []).filter((c) =>
    action === 'start' ? c.state !== 'running' : isActiveState(c.state),
  );
  for (const c of targets) act(c, action);
}

function menuItems(c: DockerContainer): MenuItem[] {
  const running = c.state === 'running';
  const items: MenuItem[] = [
    {
      type: 'item',
      id: 'start',
      label: t('common.action.start.label'),
      icon: 'play',
      disabled: running,
      run: () => act(c, 'start'),
    },
    {
      type: 'item',
      id: 'stop',
      label: t('common.action.stop.label'),
      icon: 'debug-stop',
      danger: true,
      disabled: !isActiveState(c.state),
      run: () => act(c, 'stop'),
    },
    {
      type: 'item',
      id: 'restart',
      label: t('common.action.restart.label'),
      icon: 'debug-restart',
      disabled: !running,
      run: () => act(c, 'restart'),
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'logs',
      label: t('docker.tab.logs.label'),
      icon: 'output',
      run: () => ui.select({ kind: 'container', id: c.id }, 'logs'),
    },
    {
      type: 'item',
      id: 'terminal',
      label: t('common.action.openTerminal.label'),
      icon: 'terminal',
      disabled: !running,
      run: () => ui.select({ kind: 'container', id: c.id }, 'terminal'),
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'copy-id',
      label: t('common.action.copyId.label'),
      icon: 'copy',
      run: () => copyText(c.id),
    },
    {
      type: 'item',
      id: 'copy-name',
      label: t('common.action.copyName.label'),
      icon: 'copy',
      run: () => copyText(c.name),
    },
  ];
  return items;
}

function menuFor(row: Row): MenuItem[] {
  return row.kind === 'container' ? menuItems(row.container) : [];
}

function selectContainer(c: DockerContainer): void {
  ui.select({ kind: 'container', id: c.id });
}

function openContainer(c: DockerContainer): void {
  ui.select({ kind: 'container', id: c.id }, 'logs');
}

function selectRow(row: Row): void {
  if (row.kind === 'container') ui.select({ kind: 'container', id: row.container.id });
}

function onKeyMenu(row: Row, rect: DOMRect): void {
  if (row.kind !== 'container') return;
  contextMenu.openContextMenuAt(rect.left, rect.bottom, menuItems(row.container));
}

function openRow(row: Row): void {
  if (row.kind === 'group') ui.toggleGroup(row.name);
  else ui.select({ kind: 'container', id: row.container.id }, 'logs');
}

const listEl = ref<HTMLElement | null>(null);

const groupTree = {
  depth: (row: Row) => (row.kind === 'group' || !row.grouped ? 0 : 1),
  isExpanded: (row: Row) => row.kind === 'group' && isOpen(row.name),
  hasChildren: (row: Row) => row.kind === 'group',
  expand: (row: Row) => {
    if (row.kind === 'group' && !isOpen(row.name)) ui.toggleGroup(row.name);
  },
  collapse: (row: Row) => {
    if (row.kind === 'group' && isOpen(row.name)) ui.toggleGroup(row.name);
  },
};
</script>

<template>
  <QueryState :query="containers" :is-empty="() => rows.length === 0">
    <template #empty>
      <EmptyState
        fill
        data-testid="docker-list-empty"
        icon="server"
        :title="emptyCopy.title"
        :description="emptyCopy.hint"
      />
    </template>
    <div ref="listEl" class="flex min-h-0 flex-1 flex-col">
      <VirtualList
        :rows="rows"
        :row-heights="rowHeights"
        keep-scroll-key="docker:containers"
        testid="docker-container-list"
        :select="selectRow"
        :menu-for="menuFor"
        :open="openRow"
        :menu="onKeyMenu"
        :tree="groupTree"
      >
        <template #row="{ row, nav }">
          <ContainerGroupRow
            v-if="row.kind === 'group'"
            v-bind="nav"
            :name="row.name"
            :running="row.members.filter((c) => c.state === 'running').length"
            :total="row.members.length"
            :open="isOpen(row.name)"
            @toggle="ui.toggleGroup"
            @group-action="groupAction"
          />
          <ContainerRow
            v-else
            v-bind="nav"
            :container="row.container"
            :grouped="row.grouped"
            :selected="isSelected(row.container)"
            :busy="actions.isBusy(row.container.id)"
            @select="selectContainer"
            @open="openContainer"
            @act="act"
          />
        </template>
      </VirtualList>
    </div>
    <AttributeTooltip :container="listEl" />
  </QueryState>
</template>
