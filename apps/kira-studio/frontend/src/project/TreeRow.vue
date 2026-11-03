<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import HighlightedText from '@theme/components/HighlightedText.vue';
import Spinner from '@theme/components/Spinner.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { rowIndent, rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { splitMatches } from '@workbench/behaviour/match';
import { SORTABLE_GHOST_CLASS } from '@workbench/behaviour/sortableReorder';
import TreeTwisty from '@workbench/components/TreeTwisty.vue';
import { computed, type HTMLAttributes } from 'vue';
import { useConnectionConfig } from '../state/connectionConfig';
import { useConnectionsStore } from '../state/connections';
import EngineIcon from '../theme/EngineIcon.vue';
import { columnTypeIcon, nodeIcon } from '../theme/icons';
import ErrorPopover from './ErrorPopover.vue';
import { type TreeRowVm, useTreeStore } from './state/tree';

// `sticky` is the only difference between a normal row and a band-pinned one — a
// different testid (so it can never double-count a `tree-row` locator) and a forced -1 tabindex
// (so the tree keeps its single roving tab stop). Every emit, every child element, the colour
// rail and the twisty behave identically either way — a pinned row is a real row, not a decoration.
const props = withDefaults(
  defineProps<{
    row: TreeRowVm;
    selected: boolean;
    sticky?: boolean;
    /** This connection row is the one being dragged. */
    dragging?: boolean;
    /** A connection drag would drop on this row's top or bottom edge. */
    dropEdge?: 'top' | 'bottom' | null;
    class?: HTMLAttributes['class'];
  }>(),
  { sticky: false, dragging: false, dropEdge: null },
);

// A pinned band row needs an opaque background; selected still wins (rowVariants selected).
const stickyClass = computed(() => (props.sticky && !props.selected ? 'bg-bg' : undefined));
const emit = defineEmits<{
  select: [row: TreeRowVm];
  toggle: [row: TreeRowVm];
  open: [row: TreeRowVm];
  contextmenu: [row: TreeRowVm, event: MouseEvent];
}>();

const connectionsStore = useConnectionsStore();
const treeStore = useTreeStore();
const config = useConnectionConfig();

// The file behind a connection row has an error: the row keeps its last valid name and color.
const fileProblem = computed(() => {
  if (props.row.kind !== 'connection') return null;
  const file = config.data.value?.files.find((f) => f.id === props.row.connectionId);
  const problem = file?.problem;
  if (!problem) return null;
  return problem.line > 0
    ? t('common.config.lineError.error', { line: problem.line, text: problem.message })
    : problem.message;
});

const icon = computed(() => {
  // P19: a group folder's icon reflects its own expand state, unlike every other kind.
  if (props.row.kind === 'group') return props.row.expanded ? 'folder-opened' : 'folder';
  if (props.row.kind === 'column' && props.row.detail) return columnTypeIcon(props.row.detail);
  return nodeIcon(props.row.kind === 'connection' ? 'connection' : props.row.kind);
});

const STATUS_TONE: Record<string, 'ok' | 'warn' | 'error'> = {
  connected: 'ok',
  connecting: 'warn',
  error: 'error',
};

const statusTitle = computed(() => {
  if (props.row.kind !== 'connection') return undefined;
  return props.row.statusDetail ?? undefined;
});

// P16 design system LAW: the connection colour is a 4px rail running the length
// of the connection's whole group in the tree, not a badge on one row — so it
// is looked up per-row from the connection record (row.color is only ever set
// on the connection row itself) and drawn at every depth.
const railColor = computed(() => connectionsStore.connectionRecord(props.row.connectionId)?.color);

// The connection row's own kind ("postgres", "mongodb", ...), shown as a badge
// instead of a second icon — the state dot already occupies the icon slot.
const connectionKind = computed(() => {
  if (props.row.kind !== 'connection') return undefined;
  return connectionsStore.connectionRecord(props.row.connectionId)?.kind;
});

// Only the matched substring(s) of the label are marked, not the whole label.
const parts = computed(() =>
  props.row.matched ? splitMatches(props.row.name, treeStore.activeSearchQuery) : [],
);

// A focused row is the roving stop, so Shift+F10 and arrows act on it.
function onFocus(): void {
  if (!props.sticky && !props.selected) emit('select', props.row);
}

function onClick(): void {
  emit('select', props.row);
}

function onDblClick(): void {
  // Not gated on hasChildren: a redis 'key' leaf (P9) is childless by design (D14) but still
  // needs double-click to open its keyvalue tab. onOpen() itself gates the expand/collapse
  // fallback on hasChildren, so a childless non-openable row (column, index) still no-ops.
  emit('open', props.row);
}

function onTwistyClick(): void {
  emit('toggle', props.row);
}

function onContextMenu(e: MouseEvent): void {
  emit('contextmenu', props.row, e);
}

// P105 §5.2(c): the container's own onTreeKeydown (ProjectPanel.vue) already claims Enter for
// the row's primary action (the same one dblclick fires) — Space mirrors a single click instead,
// a genuine non-conflicting keyboard equivalent, not a no-op stand-in.
function onKeydown(e: KeyboardEvent): void {
  if (e.key === ' ') {
    e.preventDefault();
    onClick();
  }
}
</script>

<template>
  <div
    :class="
      cn(
        rowVariants({ layout: 'tree', selected }),
        'h-row',
        stickyClass,
        dragging && SORTABLE_GHOST_CLASS,
        props.class,
      )
    "
    :style="rowIndent(row.depth)"
    :data-testid="sticky ? 'tree-sticky-row' : 'tree-row'"
    :data-path="row.path"
    :data-kind="row.kind"
    :data-connection-id="row.kind === 'connection' ? row.connectionId : undefined"
    :data-status="row.kind === 'connection' ? row.status : undefined"
    :data-depth="sticky ? row.depth : undefined"
    role="treeitem"
    :aria-level="row.depth + 1"
    :aria-expanded="row.hasChildren ? row.expanded : undefined"
    :aria-selected="selected"
    :tabindex="sticky ? -1 : selected ? 0 : -1"
    @focus="onFocus"
    @click="onClick"
    @dblclick="onDblClick"
    @keydown="onKeydown"
    @contextmenu.prevent.stop="onContextMenu"
  >
    <div :class="colorMarkClass('rail', railColor)" data-testid="tree-rail" />
    <div
      v-if="dropEdge"
      aria-hidden="true"
      data-testid="tree-drop-indicator"
      :class="
        cn(
          'pointer-events-none absolute inset-x-0 h-0.5 bg-focus',
          dropEdge === 'top' ? 'top-0' : 'bottom-0',
        )
      "
    />

    <!-- The twisty is the one control in the app whose entire meaning is already
         drawn by the chevron direction, and it fires on the single most-hovered control in the
         panel — no tooltip. :aria-label stays: every icon-only control sets its own explicit
         aria-label by hand (no more directive mirroring it in automatically), so dropping it
         here would leave this button nameless. -->
    <TreeTwisty :expanded="row.expanded" :has-children="row.hasChildren" @toggle="onTwistyClick">
      <Spinner v-if="row.loading" label="" data-testid="tree-row-spinner" />
      <CodiconIcon v-else :name="row.expanded ? 'chevron-down' : 'chevron-right'" :size="13" />
    </TreeTwisty>

    <span v-if="row.kind === 'connection'" class="size-4 flex items-center justify-center shrink-0">
      <Tooltip :disabled="!statusTitle">
        <TooltipTrigger as-child>
          <StatusDot
            class="status-dot"
            :tone="STATUS_TONE[row.status ?? 'disconnected'] ?? 'muted'"
            :pulse="row.status === 'connecting'"
            :data-status="row.status"
          />
        </TooltipTrigger>
        <TooltipContent v-if="statusTitle">{{ statusTitle }}</TooltipContent>
      </Tooltip>
    </span>
    <span v-if="connectionKind" class="size-4 flex items-center justify-center shrink-0">
      <EngineIcon :kind="connectionKind" :size="13" />
    </span>
    <CodiconIcon
      v-else-if="row.kind !== 'connection'"
      :name="icon"
      :size="13"
      class="shrink-0 text-muted-foreground"
    />

    <span class="label min-w-0">
      <TruncatedText :text="row.name">
        <HighlightedText v-if="parts.length > 0" :parts="parts" />
        <template v-else>{{ row.name }}</template>
      </TruncatedText>
    </span>

    <Tooltip v-if="fileProblem">
      <TooltipTrigger as-child>
        <span class="flex shrink-0 text-error" data-testid="tree-connection-problem">
          <CodiconIcon name="warning" :size="13" />
          <span class="sr-only">{{ fileProblem }}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>{{ fileProblem }}</TooltipContent>
    </Tooltip>

    <span v-if="row.badges?.length" class="flex gap-0.5 shrink-0">
      <span v-for="badge in row.badges" :key="badge" :class="rowMeta">{{ badge }}</span>
    </span>

    <ErrorPopover v-if="row.error" :message="row.error" />
    <!-- A connect failure has no `row.error`/ErrorPopover (tree.ts sets `error` only for a
         post-connect children-fetch failure), so the reason shows here, truncated. -->
    <span
      v-else-if="row.kind === 'connection' && row.status === 'error' && row.statusDetail"
      class="ml-auto min-w-0 shrink text-error"
      data-testid="connection-error-detail"
      ><TruncatedText :text="row.statusDetail" /></span
    >
    <span v-else-if="row.detail" class="ml-auto min-w-0 shrink" :class="rowMeta"
      ><TruncatedText :text="row.detail" /></span
    >
  </div>
</template>
