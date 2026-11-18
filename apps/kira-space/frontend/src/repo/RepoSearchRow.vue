<script setup lang="ts">
import HighlightedText from '@theme/components/HighlightedText.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { t } from '@theme/i18n';
import { rowIndent, rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import TreeTwisty from '@workbench/components/TreeTwisty.vue';
import { computed, type HTMLAttributes } from 'vue';
import { fileIconStyle } from './fileIcon';
import type { RepoSearchRowVm } from './state/search';

// C7 §7.3: one flat row component for both shapes the store's own row fold produces — a file
// header (path, match count, collapse chevron) or a match (line:column, highlighted preview) —
// mirroring RepoTreeRow.vue's own markup/class conventions so the two panels read as one system.
const props = defineProps<{
  row: RepoSearchRowVm;
  selected: boolean;
  /** Roving tabindex from `nav.rowProps`. */
  tabindex?: number;
  class?: HTMLAttributes['class'];
}>();

// P110 I2-14: no sticky branch -- this row is never rendered inside a sticky layer, unlike
// TreeRow.vue/CollectionRow.vue/RepoTreeRow.vue, which all take a `sticky` prop.

const emit = defineEmits<{
  select: [row: RepoSearchRowVm];
  toggleCollapse: [row: RepoSearchRowVm];
  open: [row: RepoSearchRowVm, preview: boolean];
}>();

const fileName = computed(() => {
  const i = props.row.path.lastIndexOf('/');
  return i < 0 ? props.row.path : props.row.path.slice(i + 1);
});

// previewMatchStart/End are UTF-16 offsets into preview (packages/shared/domain/repo.ts) — exactly
// what a JS string index already is, so slicing needs no further conversion.
const previewParts = computed(() => {
  const preview = props.row.preview ?? '';
  const start = props.row.previewMatchStart ?? 0;
  const end = props.row.previewMatchEnd ?? 0;
  return [
    { text: preview.slice(0, start), match: false },
    { text: preview.slice(start, end), match: true },
    { text: preview.slice(end), match: false },
  ].filter((p) => p.text !== '');
});

function onClick(): void {
  emit('select', props.row);
  if (props.row.kind === 'file') {
    emit('toggleCollapse', props.row);
  } else {
    // §7.3: a single click opens a preview tab — the tree's own onSelect/onOpen split.
    emit('open', props.row, true);
  }
}

function onDblClick(): void {
  if (props.row.kind === 'file') return;
  emit('open', props.row, false);
}
</script>

<template>
  <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter, Space and the arrows come from useListNavigation on the list. -->
  <div
    v-if="row.kind === 'file'"
    :class="cn(rowVariants({ layout: 'tree', selected }), 'h-row', props.class)"
    :style="rowIndent(0)"
    data-testid="repo-search-file-row"
    :data-path="row.path"
    role="option"
    :aria-selected="selected"
    :tabindex="tabindex"
    @click="onClick"
  >
    <!-- The whole row is the click target (onClick), so the twisty's @toggle mirrors that row click. -->
    <TreeTwisty :expanded="!row.collapsed" :has-children="true" @toggle="onClick" />
    <span
      class="shrink-0 w-4 h-4 mask-contain mask-no-repeat mask-center"
      :style="fileIconStyle(row.path)"
      aria-hidden="true"
    ></span>
    <span class="min-w-0 flex-1"
      ><TruncatedText :text="row.path">{{ fileName }}</TruncatedText></span
    >
    <span :class="[rowMeta, 'shrink-0']" data-testid="repo-search-match-count">{{
      row.matchCount
    }}</span>
    <TextTooltip v-if="row.fileTruncated" :text="t('space.repo.search.truncated.tooltip')"
      ><span class="text-kira-sm text-subtle">+</span></TextTooltip
    >
  </div>
  <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter, Space and the arrows come from useListNavigation on the list. -->
  <div
    v-else
    :class="cn(rowVariants({ layout: 'tree', selected }), 'h-row font-data', props.class)"
    :style="rowIndent(1)"
    data-testid="repo-search-match-row"
    :data-path="row.path"
    :data-line="row.line"
    role="option"
    :aria-selected="selected"
    :tabindex="tabindex"
    @click="onClick"
    @dblclick="onDblClick"
  >
    <span :class="[rowMeta, 'shrink-0 min-w-10']">{{ row.line }}:{{ row.column }}</span>
    <span class="min-w-0 truncate"><HighlightedText :parts="previewParts" /></span>
  </div>
  <!-- Row state and indent come from `rowVariants`/`rowIndent` (match rows sit one level in). -->
</template>
