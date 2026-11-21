<script setup lang="ts">
import { useVirtualRows } from '@workbench/behaviour/virtualRows';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { computed, ref } from 'vue';

// A held log as lines, verbatim: a timestamp column when any line has one, long lines scroll sideways.
// Rows are virtualized, so a long tail stays cheap.
const props = defineProps<{
  lines: readonly { ts?: string; text: string }[];
  testid: string;
}>();

const { single: rowHeight } = useRowHeight();
const scrollRef = ref<HTMLElement | null>(null);
const {
  virtualItems: virtualRows,
  totalSize,
  onScroll,
} = useVirtualRows({
  count: () => props.lines.length,
  rowHeight: () => rowHeight.value,
  scrollElement: scrollRef,
  overscan: 12,
});
const hasTs = computed(() => props.lines.some((l) => l.ts));
</script>

<template>
  <div
    ref="scrollRef"
    class="max-h-72 overflow-auto font-data text-kira-md"
    :data-testid="testid"
    @scroll.passive="onScroll"
  >
    <div class="relative min-w-max" :style="{ height: `${totalSize}px` }">
      <div
        v-for="vr in virtualRows"
        :key="vr.index"
        class="absolute inset-x-0 flex items-center gap-3 whitespace-pre"
        :style="{ top: `${vr.start}px`, height: `${rowHeight}px` }"
        :data-testid="`${testid}-line`"
      >
        <span v-if="hasTs" class="shrink-0 text-subtle">{{ lines[vr.index]?.ts ?? '' }}</span>
        <span>{{ lines[vr.index]?.text }}</span>
      </div>
    </div>
  </div>
</template>
