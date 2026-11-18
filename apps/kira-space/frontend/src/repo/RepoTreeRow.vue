<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { rowIndent, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import TreeTwisty from '@workbench/components/TreeTwisty.vue';
import { computed, type HTMLAttributes } from 'vue';
import { fileIconStyle } from './fileIcon';
import type { RepoTreeRowVm } from './state/fileTree';

const props = withDefaults(
  defineProps<{
    row: RepoTreeRowVm;
    selected: boolean;
    sticky?: boolean;
    class?: HTMLAttributes['class'];
  }>(),
  { sticky: false },
);

const emit = defineEmits<{
  select: [row: RepoTreeRowVm];
  toggle: [row: RepoTreeRowVm];
  open: [row: RepoTreeRowVm, preview: boolean];
  contextmenu: [row: RepoTreeRowVm, event: MouseEvent];
}>();

// P67b §6.2: directories keep their folder glyph (OQ-3 — the diff tree has no directory-icon rule
// to port, so none is invented here). A file's own icon comes from fileIconStyle below instead.
const dirIcon = computed(() => (props.row.expanded ? 'folder-opened' : 'folder'));

// The four-value status glyph — 'M'/'A' amber-ish/green-ish, 'D' struck, '?' muted, matching
// the connection tree's own status-color convention. data-status stays on the row for tests/
// styling hooks; the label's own colour is now a direct computed class rather than an
// attribute-selector CSS rule keyed off it.
const statusAttr = computed(() => props.row.status);

const labelStatusClass = computed(() => {
  switch (statusAttr.value) {
    case 'M':
    case 'A':
      return 'text-warn';
    case 'D':
      return 'text-error line-through';
    case '?':
      return 'text-muted-foreground';
    default:
      return '';
  }
});

function onClick(): void {
  emit('select', props.row);
  if (props.row.isDir) {
    emit('toggle', props.row);
  } else {
    // §7.2: single click opens a preview tab.
    emit('open', props.row, true);
  }
}

function onDblClick(): void {
  if (props.row.isDir) return;
  // §7.2: double click opens a permanent tab.
  emit('open', props.row, false);
}

function onContextMenu(e: MouseEvent): void {
  emit('contextmenu', props.row, e);
}
</script>

<template>
  <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter, Space and the arrows come from useListNavigation on the list. -->
  <div
    :class="
      cn(
        rowVariants({ layout: 'tree', selected }),
        'h-row',
        sticky && !selected && 'bg-bg',
        props.class,
      )
    "
    :style="rowIndent(row.depth)"
    :data-testid="sticky ? 'repo-tree-sticky-row' : 'repo-tree-row'"
    :data-path="row.path"
    :data-status="statusAttr"
    role="treeitem"
    :aria-expanded="row.isDir ? row.expanded : undefined"
    :aria-selected="selected"
    :tabindex="sticky ? -1 : selected ? 0 : -1"
    @click="onClick"
    @dblclick="onDblClick"
    @contextmenu.prevent.stop="onContextMenu"
  >
    <TreeTwisty
      :expanded="row.expanded"
      :has-children="row.hasChildren"
      @toggle="emit('toggle', row)"
    />
    <CodiconIcon
      v-if="row.isDir"
      :name="dirIcon"
      :size="13"
      class="node-icon shrink-0 text-muted-foreground"
    />
    <span v-else class="size-4 shrink-0 flex items-center justify-center" aria-hidden="true">
      <span
        class="node-icon size-3.25 text-muted-foreground mask-contain mask-no-repeat mask-center"
        :style="fileIconStyle(row.path)"
      ></span>
    </span>
    <TextTooltip :text="row.name">
      <span class="overflow-hidden text-ellipsis min-w-0" :class="labelStatusClass">{{
        row.name
      }}</span>
    </TextTooltip>
  </div>
</template>
