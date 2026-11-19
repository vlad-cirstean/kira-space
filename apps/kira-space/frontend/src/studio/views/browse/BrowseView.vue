<script setup lang="ts">
import type { ShortcutId } from '@shared/domain/shortcuts';
import { decodePath, encodePath, pathTail, type TreeNode } from '@shared/domain/tree';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { ResizeHandle } from '@theme/components/ui/resizable';
import { useLiveSize } from '@theme/components/ui/resizable/useLiveSize';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import RunState from '@theme/RunState.vue';
import { useDebounceFn } from '@vueuse/core';
import { matchesQuery } from '@workbench/behaviour/match';
import { TIMING } from '@workbench/behaviour/timing';
import { useTreeShortcuts } from '@workbench/behaviour/treeShortcuts';
import ViewHead from '@workbench/components/ViewHead.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import VirtualList from '@workbench/components/VirtualList.vue';
import { type MenuItem, runMenuShortcut, useContextMenuStore } from '@workbench/state/contextMenu';
import { tabViewKey } from '@workbench/state/viewState';
import {
  type ComponentPublicInstance,
  computed,
  onBeforeUnmount,
  onMounted,
  onUnmounted,
  useTemplateRef,
} from 'vue';
import type { BrowseTabRecord } from '../../../state/tabDomain';
import { useTabsStore } from '../../../state/tabs';
import { useConnectionsStore } from '../../state/connections';
import { useObjectStoreStore } from '../../state/objectStore';
import { useRunState } from '../../state/runState';
import EngineIcon from '../../theme/EngineIcon.vue';
import { nodeIcon, redisTypeIcon, redisTypeLabel } from '../../theme/icons';
import {
  type KeyValueHost,
  registerKeyValueHost,
  unregisterKeyValueHost,
} from '../shared/keyvalue/host';
import KeyValuePane from '../shared/keyvalue/KeyValuePane.vue';
import StaleDataStrip from '../shared/StaleDataStrip.vue';
import { refreshOrReconnect, useConnectionGate } from '../shared/useConnectionGate';
import { writeDisabledReason } from '../shared/writeGate';
import { menuForNode } from './menu';
import { useBrowseViewStore } from './state';

const contextMenuStore = useContextMenuStore();

// MainView.vue keys this component by tab.id — same discipline as every other view.
const props = defineProps<{ tab: BrowseTabRecord }>();

const objectStoreStore = useObjectStoreStore();
const connectionsStore = useConnectionsStore();
const tabsStore = useTabsStore();
const browseViewStore = useBrowseViewStore();

const { needsReconnect, reconnecting, onReconnectAndLoad } = useConnectionGate(
  () => props.tab,
  () => browseViewStore.load(props.tab.id),
);

const rt = computed(() => browseViewStore.runtime[props.tab.id]);
const loading = computed(() => rt.value?.status === 'loading');

// Shown level outlived its connection: kept, read-only, behind StaleDataStrip.
const stale = computed(() => browseViewStore.isStale(props.tab.id));
const hasNodes = computed(() => (rt.value?.nodes.length ?? 0) > 0);
const showGate = computed(() => needsReconnect.value && !hasNodes.value);

// P41 D14: '' in session state means "the tab's own container path".
const currentLevelPath = computed(() =>
  props.tab.state.levelPath === '' ? props.tab.path : props.tab.state.levelPath,
);
const atRoot = computed(() => currentLevelPath.value === props.tab.path);

const targetTail = computed(() => pathTail(props.tab.path));
const targetName = computed(() => targetTail.value?.name ?? props.tab.path);
const headerIcon = computed(() => nodeIcon(targetTail.value?.kind ?? 'database'));
const connRecord = computed(() => connectionsStore.connectionRecord(props.tab.connectionId));
const pathPrefix = computed(() => (connRecord.value?.name ? `${connRecord.value.name} / ` : ''));

// P104 §3: ViewChrome/ViewHeader/RunState inlined -- railColor mirrors ViewChrome.vue's own
// `envColor ?? (connection ? connection.color ?? null : undefined)`; this view has no envColor.
const railColor = computed(() => (connRecord.value ? (connRecord.value.color ?? null) : undefined));
const runState = useRunState(() => props.tab.id);

// The breadcrumb: one crumb per path segment from the current level, each a jump target for
// goToLevel (D12) — not just the immediate parent Up already covers.
const crumbs = computed(() => {
  if (!props.tab.connectionId) return [];
  const decoded = decodePath(props.tab.connectionId, currentLevelPath.value);
  return decoded.segments.map((seg, i) => ({
    name: seg.name,
    path: encodePath(decoded.segments.slice(0, i + 1)),
  }));
});

function onCrumbClick(path: string): void {
  if (path === currentLevelPath.value) return;
  void browseViewStore.goToLevel(props.tab.id, path);
}

function onUp(): void {
  void browseViewStore.ascend(props.tab.id);
}

function onReload(): void {
  refreshOrReconnect(needsReconnect.value, onReconnectAndLoad, () =>
    browseViewStore.reload(props.tab.id),
  );
}

// D18: a plain substring filter over the loaded level, never a second server call.
const filterText = computed({
  get: () => rt.value?.filter ?? '',
  set: (v: string) => browseViewStore.setFilter(props.tab.id, v),
});

// Shared search idiom (HttpRequestView.vue's toggleFieldFilter), not an always-visible field.
// Runtime store, not tab state: a lens, not a setting. Kept across tab switches (P375).
const filterOpen = computed(() => rt.value?.filterOpen ?? false);
function toggleFilter(): void {
  const open = !filterOpen.value;
  browseViewStore.setFilterOpen(props.tab.id, open);
  // D13's own rule (restated here): closing must restore every hidden row.
  if (!open) filterText.value = '';
}

const filteredNodes = computed<TreeNode[]>(() => {
  const nodes = rt.value?.nodes ?? [];
  return nodes.filter((n) => matchesQuery(n.name, filterText.value));
});

const countText = computed(() => {
  const total = rt.value?.nodes.length ?? 0;
  if (filterText.value.trim()) {
    return t('studio.browse.filtered.status', { shown: filteredNodes.value.length, total });
  }
  return t('studio.browse.count.status', { count: total });
});

// P63 §2.1/§2.2: the vertical split's own state. `listWidth` mirrors HttpRequestView.vue's own
// requestPaneHeight persistence idiom — `0` means "the default", so a tab saved before this field
// existed restores unchanged.
const DEFAULT_LIST_WIDTH = 320;
const listPane = useLiveSize(
  () => props.tab.state.listWidth || DEFAULT_LIST_WIDTH,
  (size) => tabsStore.patchBrowseTabState(props.tab.id, { listWidth: size }),
);

// The detail pane previews the selected row when it's a leaf (hasChildren: false) — a container
// row, or nothing selected, has no value of its own to show (§2.3).
const selectedNode = computed<TreeNode | null>(() => {
  const selected = rt.value?.selected;
  if (!selected) return null;
  return rt.value?.nodes.find((n) => n.path === selected) ?? null;
});
const previewable = computed(() => !!selectedNode.value && !selectedNode.value.hasChildren);
const emptyPreviewLabel = computed(() =>
  targetTail.value?.kind === 'bucket'
    ? t('studio.browse.detail.selectObject.empty')
    : t('studio.browse.detail.selectKey.empty'),
);
const emptyPreviewIcon = computed(() => (targetTail.value?.kind === 'bucket' ? 'file' : 'tag'));

// A non-tab viewKey, registered once for this tab's whole lifetime — its own resolver reads
// `rt` fresh on every call, so it never needs re-registering as selection changes. `path` reads
// `rt.selected` directly rather than `selectedNode`/`previewable`: KeyValuePane is only ever
// mounted while `previewable` holds (below), so a container's own path here is never read.
const previewKey = `${props.tab.id}::preview`;
registerKeyValueHost(previewKey, (): KeyValueHost | null => {
  const r = rt.value;
  if (!r?.selected) return null;
  return {
    gateTabId: props.tab.id,
    connectionId: props.tab.connectionId,
    path: r.selected,
    pageIndex: r.previewPageIndex,
    pageSize: r.previewPageSize,
    patch: (p) => {
      const target = rt.value;
      if (!target) return;
      if (p.pageIndex !== undefined) target.previewPageIndex = p.pageIndex;
      if (p.pageSize !== undefined) target.previewPageSize = p.pageSize;
    },
  };
});
onUnmounted(() => unregisterKeyValueHost(previewKey));

// D12 (§2.3 extends it, not replaces it): a single click still just selects the row — the detail
// pane above reacts to `rt.selected` on its own (KeyValuePane's own watch on its resolved host
// path), so no separate "load into the right pane" call belongs here.
function onRowClick(node: TreeNode): void {
  browseViewStore.selectRow(props.tab.id, node.path);
}

// P63 §4.3: gates the whole per-type fetch — an S3 browse tab (caps.keyTypes false) never issues
// treeKeyTypes at all, not just gets an empty answer from it.
const supportsKeyTypes = computed(
  () => !!connectionsStore.states[props.tab.connectionId ?? '']?.caps?.keyTypes,
);

// Reads through `keyTypesVersion` so this re-renders when ensureKeyTypes below fills in a path —
// `rt.keyTypes` itself is markRaw'd (browse/state.ts's own doc comment), so the Map's own mutation
// is invisible to Vue without this.
function keyType(path: string): string | undefined {
  void rt.value?.keyTypesVersion;
  return rt.value?.keyTypes.get(path);
}

// VirtualList's `visible` emit fires up to once per animation frame
// during a scroll fling — each distinct window used to issue its own IPC round trip even though
// path-level dedup (ensureKeyTypes' own pending/keyTypes checks) only prevents REPEATING work, not
// the round trip itself. Debounced by the same 150ms already used elsewhere, so a fling settles on
// one ensureKeyTypes call for its final window instead of one per frame.
const ensureKeyTypesDebounced = useDebounceFn((range: { start: number; end: number }) => {
  const nodes = filteredNodes.value;
  const paths: string[] = [];
  for (let i = range.start; i < range.end && i < nodes.length; i++) {
    const node = nodes[i];
    if (node && node.kind === 'key') paths.push(node.path);
  }
  if (paths.length > 0) browseViewStore.ensureKeyTypes(props.tab.id, paths);
}, TIMING.search);
onBeforeUnmount(() => ensureKeyTypesDebounced.cancel());

// D12: a container descends; a leaf previews in the split. The context menu opens a standalone tab.
function onRowOpen(node: TreeNode): void {
  if (node.hasChildren) {
    void browseViewStore.descend(props.tab.id, node.path);
    return;
  }
  onRowClick(node);
}

// P33 D3: the same caps.fileTransfer + canInsert + not-read-only gate uploadMenuItem applies,
// surfaced as a toolbar button too (Console.html-style primary action) rather than only reachable
// through a container row's own context menu.
const canUpload = computed(() => {
  const caps = connectionsStore.states[props.tab.connectionId ?? '']?.caps;
  const record = connectionsStore.connectionRecord(props.tab.connectionId);
  return !!caps?.fileTransfer && !!caps.canInsert && !record?.readOnly;
});
const uploadTitle = computed(() =>
  stale.value ? writeDisabledReason(true, true) : t('studio.browse.upload.label'),
);
function onUploadClick(): void {
  if (!props.tab.connectionId || stale.value) return;
  objectStoreStore.openUploadDialog(props.tab.connectionId, currentLevelPath.value, props.tab.id);
}

const listRows = computed(() => filteredNodes.value.map((node) => ({ key: node.path, node })));
const listRef = useTemplateRef<
  ComponentPublicInstance & { focusKey: (key: string) => Promise<void> }
>('listRef');
const listEl = computed(() => listRef.value?.$el as HTMLElement | undefined);
const activeKey = computed({
  get: () => rt.value?.selected ?? null,
  set: (path) => {
    if (path !== null) browseViewStore.selectRow(props.tab.id, path);
  },
});

function menuFor(node: TreeNode): MenuItem[] {
  const connectionId = props.tab.connectionId;
  return connectionId ? menuForNode(props.tab.id, connectionId, node, stale.value) : [];
}

function onRowMenu(node: TreeNode, rect: DOMRect): void {
  contextMenuStore.openContextMenuAt(rect.left, rect.bottom, menuFor(node), {
    onRestoreFocus: () => void listRef.value?.focusKey(node.path),
  });
}

function viaMenu(id: ShortcutId): (path: string) => void {
  return (path) => {
    const connectionId = props.tab.connectionId;
    const node = rt.value?.nodes.find((n) => n.path === path);
    if (connectionId && node)
      runMenuShortcut(menuForNode(props.tab.id, connectionId, node, stale.value), id);
  };
}
useTreeShortcuts(
  { container: listEl, activeKey },
  {
    'tree.copyName': viaMenu('tree.copyName'),
    'tree.delete': viaMenu('tree.delete'),
  },
);

function onVisibleRange(range: { start: number; end: number }): void {
  if (supportsKeyTypes.value) void ensureKeyTypesDebounced(range);
}

onMounted(() => {
  if (!needsReconnect.value && !browseViewStore.runtime[props.tab.id]) {
    void browseViewStore.load(props.tab.id);
  }
});
</script>

<template>
  <div
    class="h-full flex flex-col min-h-0"
    data-testid="browse-view"
    :data-path="tab.path"
    :data-level="currentLevelPath"
  >
    <ViewHead
      :color="railColor"
      :icon="headerIcon"
      :prefix="pathPrefix || undefined"
      :title="targetName"
      title-testid="browse-target"
    >
      <template v-if="connRecord?.kind" #lead
        ><EngineIcon :kind="connRecord.kind" :size="13" /></template
      >
    </ViewHead>
    <ViewToolbar>
      <div class="flex items-center gap-1.5 min-w-0">
        <TooltipIconButton
          :icon="ICON.refresh"
          :label="t('common.action.refresh.label')"
          data-testid="browse-refresh"
          @click="onReload"
        />
      </div>
      <!-- P63 §3.2: navigator-scoped controls (back + breadcrumb + count) moved into the list
             pane's own .list-head band, alongside the VirtualList they act on — this toolbar keeps
             only what is view-scoped: filter, upload, and the refresh/stop group above. -->
      <TooltipIconButton
        :icon="ICON.search"
        :label="t('common.action.filter.label')"
        :pressed="filterOpen"
        data-testid="browse-filter-toggle"
        @click="toggleFilter"
      />
      <TooltipIconButton
        v-if="canUpload"
        icon="cloud-upload"
        :label="uploadTitle"
        :aria-label="t('studio.browse.upload.label')"
        disabled-trigger
        :disabled="stale"
        data-testid="browse-upload"
        @click="onUploadClick"
      />
      <span class="ml-auto" />
      <RunState :state="runState" />
      <div class="flex items-center gap-1.5 min-w-0"></div>
    </ViewToolbar>

    <div v-if="filterOpen" class="shrink-0 px-1.5 py-1">
      <div class="flex w-full font-ui"
        ><SearchField
          :model-value="filterText"
          :placeholder="t('studio.browse.filter.placeholder')"
          data-testid="browse-filter"
          @update:model-value="(v: string | number) => (filterText = String(v))"
        /></div
      >
    </div>
    <StaleDataStrip
      v-if="stale && !showGate"
      testid="browse-stale"
      :reconnecting="reconnecting"
      @reconnect="onReconnectAndLoad"
    />
    <Alert
      strip
      v-if="rt?.status === 'error' && rt.error"
      variant="error"
      data-testid="browse-error"
    >
      <AlertDescription>{{ rt.error.message }}</AlertDescription>
    </Alert>
    <!-- P43 F6/D7: a failed delete from this level, distinct from a failed load above. -->
    <Alert strip v-if="rt?.actionError" variant="error" data-testid="browse-action-error">
      <AlertDescription>{{ rt.actionError }}</AlertDescription>
    </Alert>
    <!-- P43 iter2 F16/D23: the adapter's own round budget cut this level's listing short —
             nothing failed, the listing is real, it's just incomplete. -->
    <Alert strip v-if="rt?.truncated" variant="warn" data-testid="browse-truncated">
      <AlertDescription>
        {{ t('studio.browse.truncated.body') }}
      </AlertDescription>
    </Alert>

    <!-- The reconnect gate used to replace this whole chrome (header, toolbar and
           all) — every other view but the grid's DataView.vue did the same, the one inconsistency
           this fixes. The chrome above (and so its toolbar rows) now always renders; only
           the body — the part that needs a live connection — swaps for the gate.
           "the body" is now the whole split (list pane + splitter + detail pane), never
           just the list — a disconnected connection means neither side has anything live to show.
           -->
    <EmptyState
      v-if="showGate"
      data-testid="browse-reconnect"
      icon="plug"
      :title="t('studio.common.disconnected.empty')"
    >
      <Button
        variant="dialog-primary"
        size="kira-lg"
        data-testid="browse-reconnect-load"
        :pending="reconnecting"
        @click="onReconnectAndLoad"
      >
        {{ t('studio.common.reconnectAndLoad.label') }}
      </Button>
    </EmptyState>
    <div v-else class="flex flex-row flex-1 min-h-0">
      <div class="min-w-0 flex flex-col min-h-0" :style="{ flex: `0 1 ${listPane.size.value}px` }">
        <!-- Back + breadcrumb + count — navigator-scoped controls, moved out of
               ViewChrome's own toolbar into the pane whose list they act on. Reuses the same
               toolbar utility band (the same 26px in-view band every other toolbar already is)
               rather than inventing a header. -->
        <ViewToolbar data-testid="browse-list-head">
          <TooltipIconButton
            :icon="ICON.back"
            :label="t('common.action.back.label')"
            disabled-trigger
            :disabled="atRoot"
            data-testid="browse-up"
            @click="onUp"
          />
          <span class="flex items-center min-w-0 overflow-hidden gap-0.5">
            <template v-for="(crumb, i) in crumbs" :key="crumb.path">
              <span v-if="i > 0" class="text-subtle">/</span>
              <Button
                variant="toolbar"
                size="inline"
                class="min-w-0 truncate px-0.5 text-kira-md"
                :class="i === crumbs.length - 1 && 'text-fg'"
                data-testid="browse-crumb"
                @click="onCrumbClick(crumb.path)"
              >
                {{ crumb.name }}
              </Button>
            </template>
          </span>
          <span class="ml-auto" :class="rowMeta" data-testid="browse-count">{{ countText }}</span>
        </ViewToolbar>
        <div class="bg-bg overflow-hidden flex flex-col min-h-0 flex-1 rounded-none border-0">
          <EmptyState
            v-if="!rt || (loading && rt.nodes.length === 0)"
            fill
            loading
            :title="t('common.state.loading.loading')"
          />
          <EmptyState
            v-else-if="rt.nodes.length === 0"
            fill
            data-testid="browse-empty"
            icon="folder"
            :title="t('studio.browse.items.empty')"
          />
          <EmptyState
            v-else-if="filteredNodes.length === 0"
            fill
            data-testid="browse-empty"
            :icon="ICON.search"
            :title="t('common.state.noMatches.empty')"
          />
          <VirtualList
            v-else
            ref="listRef"
            v-model:active-key="activeKey"
            :rows="listRows"
            :keep-scroll-key="tabViewKey(tab.id, `browse:${currentLevelPath}`)"
            testid="virtual-list"
            :aria-label="t('studio.browse.list.aria')"
            :select="(r) => onRowClick(r.node)"
            :open="(r) => onRowOpen(r.node)"
            :menu="(r, rect) => onRowMenu(r.node, rect)"
            :menu-for="(r) => menuFor(r.node)"
            @visible="onVisibleRange"
          >
            <template #row="{ row: item, nav }">
              <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter and Space come from useListNavigation on the container. -->
              <div
                data-testid="browse-row"
                :data-path="item.node.path"
                :data-kind="item.node.kind"
                :class="
                  cn(
                    rowVariants({ layout: 'tree', selected: rt?.selected === item.node.path }),
                    'h-full px-2',
                  )
                "
                role="option"
                :aria-selected="rt?.selected === item.node.path"
                v-bind="nav"
                :tabindex="nav.tabindex"
                @click="onRowClick(item.node)"
                @dblclick="onRowOpen(item.node)"
              >
                <!-- A redis key's icon becomes its per-type glyph once its TYPE has arrived
                     (redisTypeIcon falls back to the generic key glyph otherwise). -->
                <span class="size-4 flex items-center justify-center shrink-0 text-muted-foreground"
                  ><CodiconIcon
                    :name="
                      item.node.kind === 'key'
                        ? redisTypeIcon(keyType(item.node.path))
                        : nodeIcon(item.node.kind)
                    "
                    :size="13"
                  /></span
                >
                <span class="min-w-0 flex-1 truncate">{{ item.node.name }}</span>
                <Badge
                  data
                  v-if="item.node.kind === 'key' && redisTypeLabel(keyType(item.node.path))"
                  data-testid="browse-key-type"
                  >{{
                    redisTypeLabel(keyType(item.node.path))
                  }}</Badge
                >
                <span v-if="item.node.detail" class="shrink-0" :class="rowMeta">{{
                  item.node.detail
                }}</span>
              </div>
            </template>
          </VirtualList>
        </div>
      </div>

      <ResizeHandle
        :label="t('studio.common.resizeList.aria')"
        line
        :min="220"
        :max="900"
        v-bind="listPane.bind"
      />

      <div class="min-w-0 flex flex-1 flex-col min-h-0" data-testid="browse-detail-pane">
        <KeyValuePane v-if="previewable" :view-key="previewKey" />
        <EmptyState
          v-else
          fill
          data-testid="browse-preview-empty"
          :icon="emptyPreviewIcon"
          :title="emptyPreviewLabel"
        />
      </div>
    </div>
  </div>
</template>
