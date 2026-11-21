<script setup lang="ts">
import { computed } from 'vue';
import { TASK_LINE_EDGE, TASK_LINE_MARK } from './taskGrid';

// Task colour as a line: `edge` overlays the top of a relative header, `mark` sits inline before a
// name. Parked dashes it (shape cue, not hue alone).
const props = defineProps<{ color: string; parked?: boolean; variant: 'edge' | 'mark' }>();
const paint = computed(() => ({
  background: props.parked
    ? `repeating-linear-gradient(90deg, ${props.color} 0 6px, transparent 6px 10px)`
    : props.color,
}));
</script>

<template>
  <span
    :class="variant === 'edge' ? TASK_LINE_EDGE : TASK_LINE_MARK"
    :style="paint"
    :data-parked="parked || undefined"
    data-testid="ade-task-line"
    aria-hidden="true"
  />
</template>
