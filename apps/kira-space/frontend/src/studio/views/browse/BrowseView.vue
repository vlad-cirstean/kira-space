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
import { Tooltip, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import RunState from '@theme/RunState.vue';
import { useDebounceFn } from '@vueuse/core';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { matchesQuery } from '@workbench/behaviour/match';
import { TIMING } from '@workbench/behaviour/timing';
import { useTreeShortcuts } from '@workbench/behaviour/treeShortcuts';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { runMenuShortcut, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, onBeforeUnmount, onMounted, onUnmounted, ref, watch } from 'vue';
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

// The app's own shared search idiom (HttpRequestView.vue's toggleFieldFilter) replaces
// this file's own always-visible `.filter-field` — `rt.filter`/`filteredNodes`/`countText` below
// are untouched, only the affordance moves. Runtime store, not tab state: a lens, not a setting.
// Kept across tab switches (P375) so an active filter never hides rows behind a closed bar.
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

// VirtualList's own `visible-range` emit fires up to once per animation frame
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

// The list navigation claims Enter and the arrows; Space mirrors a single click.
function onRowKeydown(e: KeyboardEvent, node: TreeNode): void {
  if (e.key === ' ') {
    e.preventDefault();
    onRowClick(node);
  }
}

// D10: the moved keyMenu/objectMenu/namespaceMenu/prefixMenu bodies, now addressed by node
// instead of by tree row.
function onRowContextMenu(e: MouseEvent, node: TreeNode): void {
  if (!props.tab.connectionId) return;
  nav.contextMenu(node, e, menuForNode(props.tab.id, props.tab.connectionId, node, stale.value));
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

const { single: rowHeight } = useRowHeight();

// P104 §3.4: VirtualList's own recipe, rebuilt on @tanstack/vue-virtual via the shared
// useVirtualRows composable.
const scrollEl = ref<HTMLElement | null>(null);
const { virtualItems, totalSize, onScroll, scrollToIndex } = useVirtualRows({
  count: () => filteredNodes.value.length,
  rowHeight: () => rowHeight.value,
  scrollElement: scrollEl,
});

const activeKey = computed({
  get: () => rt.value?.selected ?? null,
  set: (path) => {
    if (path !== null) browseViewStore.selectRow(props.tab.id, path);
  },
});
const nav = useListNavigation<TreeNode>({
  container: scrollEl,
  items: () => filteredNodes.value,
  key: (node) => node.path,
  activeKey,
  onOpen: onRowOpen,
  onSelect: onRowClick,
  onMenu: (node, rect) => {
    const connectionId = props.tab.connectionId;
    if (!connectionId) return;
    contextMenuStore.openContextMenuAt(
      rect.left,
      rect.bottom,
      menuForNode(props.tab.id, connectionId, node, stale.value),
      { onRestoreFocus: () => void nav.focusKey(node.path) },
    );
  },
  scrollToIndex: (index) => scrollToIndex(index),
  rendered: () => virtualItems.value,
});

function viaMenu(id: ShortcutId): (path: string) => void {
  return (path) => {
    const connectionId = props.tab.connectionId;
    const node = rt.value?.nodes.find((n) => n.path === path);
    if (connectionId && node)
      runMenuShortcut(menuForNode(props.tab.id, connectionId, node, stale.value), id);
  };
}
useTreeShortcuts(nav, {
  'tree.copyName': viaMenu('tree.copyName'),
  'tree.delete': viaMenu('tree.delete'),
});

// VirtualList's own visible-range emit (already used by KeyValuePane's identical need) — windowed
// per §4.3, never the whole (up to 200 000-key) level.
watch(virtualItems, (items) => {
  if (!supportsKeyTypes.value || items.length === 0) return;
  void ensureKeyTypesDebounced({ start: items[0].index, end: items[items.length - 1].index + 1 });
});

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
    <!-- P104 §3: ViewChrome/ViewHeader/RunState inlined -- no component wraps this chrome anymore. -->
    <ViewToolbar>
      <span v-if="railColor !== undefined" :class="colorMarkClass('dot', railColor)" />
      <span v-if="connRecord?.kind" class="size-4 flex items-center justify-center shrink-0">
        <EngineIcon :kind="connRecord.kind" :size="13" />
      </span>
      <span class="size-4 flex items-center justify-center shrink-0">
        <CodiconIcon :name="headerIcon" :size="13" />
      </span>
      <span class="text-kira-md text-fg truncate" data-testid="browse-target"
        ><span v-if="pathPrefix" class="text-subtle">{{ pathPrefix }}</span>{{ targetName }}</span
      >
      <span class="ml-auto flex items-center gap-1"></span>
    </ViewToolbar>

    <div :class="colorMarkClass('band', railColor)" />
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
      <Tooltip :disabled="true">
        <TooltipTrigger as-child>
          <RunState :state="runState" />
        </TooltipTrigger>
      </Tooltip>
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
            :title="t('studio.browse.items.empty')"
          />
          <EmptyState
            v-else-if="filteredNodes.length === 0"
            fill
            data-testid="browse-empty"
            :title="t('common.state.noMatches.empty')"
          />
          <div
            v-else
            ref="scrollEl"
            class="h-full overflow-auto"
            data-testid="virtual-list"
            role="listbox"
            :aria-label="t('studio.browse.list.aria')"
            @scroll="onScroll"
          >
            <div class="relative" :style="{ height: `${totalSize}px` }">
              <!-- P110 I2-15/I2-17: `.virtual-row` moved to VIRTUAL_ROW_CLASS (packages/workbench/
                     src/util/virtualRows.ts) -- see ProjectTree.vue's identical note. -->
              <div
                v-for="vi in virtualItems"
                :key="String(vi.key)"
                data-testid="browse-row"
                :data-path="filteredNodes[vi.index]?.path"
                :data-kind="filteredNodes[vi.index]?.kind"
                :class="
                  cn(
                    rowVariants({
                      layout: 'tree',
                      selected: rt?.selected === filteredNodes[vi.index]?.path,
                    }),
                    'px-2',
                    VIRTUAL_ROW_CLASS,
                  )
                "
                :style="{ height: `${vi.size}px`, transform: `translateY(${vi.start}px)` }"
                role="option"
                v-bind="nav.rowProps(filteredNodes[vi.index]!.path)"
                :tabindex="nav.tabindexFor(filteredNodes[vi.index]!.path)"
                :aria-selected="rt?.selected === filteredNodes[vi.index]?.path"
                @click="onRowClick(filteredNodes[vi.index]!)"
                @dblclick="onRowOpen(filteredNodes[vi.index]!)"
                @keydown="onRowKeydown($event, filteredNodes[vi.index]!)"
                @contextmenu.prevent="onRowContextMenu($event, filteredNodes[vi.index]!)"
              >
                <!-- P63 §4.2/§4.3: a redis key's icon becomes its per-type glyph once its TYPE
                       has arrived (redisTypeIcon falls back to the generic key glyph otherwise —
                       never a wrong type). S3 objects and every container kind are unaffected. -->
                <span class="size-4 flex items-center justify-center shrink-0 text-muted-foreground"
                  ><CodiconIcon
                    :name="
                      filteredNodes[vi.index]?.kind === 'key'
                        ? redisTypeIcon(keyType(filteredNodes[vi.index]!.path))
                        : nodeIcon(filteredNodes[vi.index]!.kind)
                    "
                    :size="13"
                  /></span
                >
                <span class="min-w-0 flex-1 truncate">{{ filteredNodes[vi.index]?.name }}</span>
                <Badge
                  data
                  v-if="
                    filteredNodes[vi.index]?.kind === 'key' &&
                    redisTypeLabel(keyType(filteredNodes[vi.index]!.path))
                  "
                  data-testid="browse-key-type"
                  >{{
                    redisTypeLabel(keyType(filteredNodes[vi.index]!.path))
                  }}</Badge
                >
                <span v-if="filteredNodes[vi.index]?.detail" class="shrink-0" :class="rowMeta">{{
                  filteredNodes[vi.index]?.detail
                }}</span>
              </div>
            </div>
          </div>
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
