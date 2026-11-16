<script setup lang="ts">
import { computed } from 'vue';
import type { BranchRowModel } from './usePlanModel';

// A branch row's progress: segments plus its label ("Implement · stuck").
const props = defineProps<{ prog: NonNullable<BranchRowModel['prog']> }>();

const SEG: Record<string, string> = {
  done: 'bg-muted-foreground',
  running: 'bg-fg',
  bad: 'bg-tone-red-solid',
};
const segClass = (state: string): string => SEG[state] ?? 'bg-border';
const labelClass = computed(() =>
  props.prog.tone === 'red' ? 'text-tone-red' : 'text-muted-foreground',
);
</script>

<template>
  <span class="inline-flex shrink-0 items-center gap-1" data-testid="ade-branch-prog">
    <span class="inline-flex gap-0.5">
      <span
        v-for="(seg, i) in prog.segs"
        :key="i"
        class="h-1.25 w-1.75 rounded-kira-sm"
        :class="segClass(seg)"
      />
    </span>
    <span class="max-w-28 truncate text-kira-sm" :class="labelClass">{{ prog.label }}</span>
  </span>
</template>
