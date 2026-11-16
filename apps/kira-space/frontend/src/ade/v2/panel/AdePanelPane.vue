<script setup lang="ts">
import { ResizeHandle } from '@theme/components/ui/resizable';
import { useLiveSize } from '@theme/components/ui/resizable/useLiveSize';
import { t } from '@theme/i18n';
import { paneFrame } from '@theme/lib/frames';
import { cn } from '@theme/lib/utils';
import { useElementSize, useParentElement } from '@vueuse/core';
import { computed } from 'vue';
import { useAdeLayoutStore } from '../../../state/adeLayout';

// Resizable right-hand pane beside a page. Width is `panel.ade.width` in the layout store, 0 = list at 400px
// (panel takes the rest of the row).
const LIST_DEFAULT = 400;
const MIN_WIDTH = 340;
const MIN_PAGE = 360;

const layout = useAdeLayoutStore();

const parent = useParentElement();
const { width: rowWidth } = useElementSize(parent);

const maxWidth = computed(() => Math.max(MIN_WIDTH, rowWidth.value - MIN_PAGE));
const pane = useLiveSize(
  () => {
    const stored = layout.width;
    const base = stored > 0 ? stored : rowWidth.value - LIST_DEFAULT;
    return Math.max(MIN_WIDTH, Math.min(maxWidth.value, base));
  },
  (w) => layout.setWidth(Math.round(w)),
);
const width = pane.size;
</script>

<template>
  <ResizeHandle
    :label="t('space.ade.panel.resize.label')"
    data-testid="ade-panel-resize-handle"
    direction="reverse"
    :min="MIN_WIDTH"
    :max="maxWidth"
    v-bind="pane.bind"
  />
  <aside
    :class="cn(paneFrame, 'flex shrink-0 flex-col')"
    :style="{ width: `${width}px` }"
    data-testid="ade-panel"
  >
    <slot />
  </aside>
</template>
