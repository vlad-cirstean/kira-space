<script setup lang="ts">
import { ResizeHandle } from '@theme/components/ui/resizable';
import { useLiveSize } from '@theme/components/ui/resizable/useLiveSize';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useLayoutStore } from '../../../../state/layout';
import { useCellSelectionStore } from '../../../state/cellSelection';
import CellEditorView from './CellEditorView.vue';

const cellSelectionStore = useCellSelectionStore();
const layoutStore = useLayoutStore();

// Mounted by the view that owns the tab, so one dock <-> one tab, torn down with it.
// `readOnly`: true when the mounting view has no write path for its cells at all (the
// query console) — a viewer, not an editor that happens to be refusing this particular cell.
// Distinct from a cell being individually uneditable (a read-only connection, a truncated value),
// which stays governed by CellEditorView's own readOnlyReasonFor regardless of this flag.
const props = withDefaults(defineProps<{ tabId: string; readonly?: boolean }>(), {
  readonly: false,
});

const emit = defineEmits<{ ready: [] }>();
const cell = computed(() => cellSelectionStore.selectedCellFor(props.tabId));
const pane = useLiveSize(
  () => layoutStore.panel.cellEditor.height,
  (height) => layoutStore.setCellEditorHeight(height),
);
</script>

<template>
  <!-- Handle and panel are siblings in the mounting view's flex column: the view puts this below its
       `flex-1` body. The height lives in the layout store, one place for every mounting view. -->
  <template v-if="cell">
    <ResizeHandle
      class="cell-splitter"
      :label="t('studio.cellEditor.resize.aria')"
      axis="y"
      direction="reverse"
      line
      :min="120"
      :max="480"
      v-bind="pane.bind"
    />
    <div
      class="min-h-0 overflow-hidden bg-bg"
      data-testid="cell-editor"
      :data-tab-id="tabId"
      :style="{ flex: `0 1 ${pane.size.value}px` }"
    >
      <CellEditorView :cell="cell" :readonly="readonly" @ready="emit('ready')" />
    </div>
  </template>
</template>
