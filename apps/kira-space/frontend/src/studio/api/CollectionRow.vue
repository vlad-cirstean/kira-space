<script setup lang="ts">
import { grpcMethodClass } from '@shared/domain/grpc';
import { httpMethodToken } from '@shared/domain/http';
import CodiconIcon from '@theme/CodiconIcon.vue';
import HighlightedText from '@theme/components/HighlightedText.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { rowIndent, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { methodTextClass } from '@theme/methodColor';
import { splitMatches } from '@workbench/behaviour/match';
import { SORTABLE_GHOST_CLASS } from '@workbench/behaviour/sortableReorder';
import InlineEditInput from '@workbench/components/InlineEditInput.vue';
import TreeTwisty from '@workbench/components/TreeTwisty.vue';
import { computed, type HTMLAttributes } from 'vue';
import type { DropZone } from './collectionDrop';
import { type CollectionRowVm, useCollectionsStore } from './state/collections';

// The same rowIndent indent, roving tabindex and twisty as project/TreeRow.vue,
// with three differences that are the whole reason this is a separate file rather than a widened
// shared row: no connection colour rail, no status dot and no EngineIcon (none of which mean
// anything here); a leading **method chip** for a request row, which is what Postman itself shows
// and what makes a long request list scannable; and an inline rename input.
//
// It is a separate file for a hard reason as well as a soft one: `http/**` may not import
// `project/**` (biome.json), so reuse was never on the table.
const props = withDefaults(
  defineProps<{
    row: CollectionRowVm;
    selected: boolean;
    /** The roving stop: the one row with tabindex 0, whether or not more rows are selected. */
    focused?: boolean;
    /** The tree's one tab stop when no row is focused. */
    tabbable?: boolean;
    sticky?: boolean;
    /** Part of the set being dragged. */
    dragging?: boolean;
    /** Where the drag would land relative to this row. */
    dropZone?: DropZone | null;
    class?: HTMLAttributes['class'];
  }>(),
  { sticky: false, tabbable: false, focused: false, dragging: false, dropZone: null },
);

// A pinned band row needs an opaque background; selected still wins (rowVariants selected).
const stickyClass = computed(() => (props.sticky && !props.selected ? 'bg-bg' : undefined));

const emit = defineEmits<{
  select: [row: CollectionRowVm, event?: MouseEvent];
  toggle: [row: CollectionRowVm];
  open: [row: CollectionRowVm];
  contextmenu: [row: CollectionRowVm, event: MouseEvent];
  rename: [row: CollectionRowVm, name: string];
  cancelRename: [];
}>();

const collectionsStore = useCollectionsStore();

// A collection is a library; a folder flips with its own expand state, the same way Studio's
// group row does. A request gets no icon at all — the method chip is its identity.
const icon = computed(() => {
  if (props.row.kind === 'collection') return 'folder-library';
  return props.row.expanded ? 'folder-opened' : 'folder';
});

const renaming = computed(() => collectionsStore.renamingKey === props.row.key);

// A URL-only match leaves the name unsplit: the URL is searched, not shown.
const parts = computed(() =>
  props.row.matched ? splitMatches(props.row.name, collectionsStore.activeSearchQuery) : [],
);

function onTwistyClick(): void {
  emit('toggle', props.row);
}

// A focused row is the roving stop, so Shift+F10 and arrows act on it. Focus from a mouse press is
// the click's to handle: selecting here would undo a Ctrl or Shift click before it lands.
let pressed = false;
function onMouseDown(): void {
  pressed = true;
}
function onMouseUp(): void {
  pressed = false;
}
function onClick(event: MouseEvent): void {
  pressed = false;
  emit('select', props.row, event);
}
function onFocus(): void {
  const byMouse = pressed;
  pressed = false;
  if (!byMouse && !props.sticky && !props.selected) emit('select', props.row);
}

// The container's useListNavigation claims Enter and the arrows; Space mirrors a single click.
function onKeydown(e: KeyboardEvent): void {
  if (e.key === ' ') {
    e.preventDefault();
    emit('select', props.row);
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
        dropZone === 'into' && 'ring-1 ring-inset ring-focus bg-focus/10',
        props.class,
      )
    "
    :style="rowIndent(row.depth)"
    :data-testid="sticky ? 'collection-sticky-row' : 'collection-row'"
    :data-kind="row.kind"
    :data-id="row.id"
    :data-depth="row.depth"
    role="treeitem"
    :aria-level="row.depth + 1"
    :aria-expanded="row.hasChildren ? row.expanded : undefined"
    :aria-selected="selected"
    :data-key="row.key"
    :data-drop-zone="dropZone ?? undefined"
    :tabindex="sticky ? -1 : focused || tabbable ? 0 : -1"
    @mousedown="onMouseDown"
    @mouseup="onMouseUp"
    @click="onClick"
    @dblclick="emit('open', row)"
    @focus="onFocus"
    @keydown="onKeydown"
    @contextmenu.prevent.stop="emit('contextmenu', row, $event)"
  >
    <div
      v-if="dropZone === 'before' || dropZone === 'after'"
      aria-hidden="true"
      data-testid="tree-drop-indicator"
      :data-zone="dropZone"
      :class="
        cn(
          'pointer-events-none absolute inset-x-0 h-0.5 bg-focus',
          dropZone === 'before' ? 'top-0' : 'bottom-0',
        )
      "
    />

    <!-- Same reasoning as project/TreeRow.vue's twisty: its entire meaning is drawn by the
         chevron direction, so no tooltip, but :aria-label stays so it isn't nameless. -->
    <TreeTwisty :expanded="row.expanded" :has-children="row.hasChildren" @toggle="onTwistyClick" />

    <!-- A fixed width so every row's name starts at the same x -- an unaligned ragged edge is
         exactly what makes a long request list hard to scan, which is the reason the chip exists
         at all. -->
    <Badge
      v-if="row.kind === 'request' && row.protocol === 'grpc'"
      class="w-14 shrink-0 overflow-hidden text-center text-ellipsis text-kira-sm"
      :variant="grpcMethodClass(row.method)"
      data-testid="grpc-collection-chip"
    >
      {{ t('studio.common.grpc.label') }}
    </Badge>
    <Badge
      v-else-if="row.kind === 'request'"
      variant="chip"
      data
      class="method w-14 shrink-0 overflow-hidden text-center text-ellipsis text-kira-sm"
      :class="methodTextClass(httpMethodToken(row.method))"
      >{{
        row.method
      }}</Badge
    >
    <CodiconIcon v-else :name="icon" :size="13" class="shrink-0 text-muted-foreground" />

    <!-- D13: inline rename doubles as the naming step for all three creation paths, so there is
         one naming interaction instead of a prompt dialog this app does not have. It is also VS
         Code's explorer behaviour, which is the tree this panel is modelled on. -->
    <InlineEditInput
      v-if="renaming"
      :name="row.name"
      data-testid="collection-rename-input"
      @commit="(name) => emit('rename', row, name)"
      @cancel="emit('cancelRename')"
    />
    <span v-else class="min-w-0">
      <TruncatedText :text="row.url || row.name">
        <HighlightedText v-if="parts.length > 0" :parts="parts" />
        <template v-else>{{ row.name }}</template>
      </TruncatedText>
    </span>
  </div>
</template>
