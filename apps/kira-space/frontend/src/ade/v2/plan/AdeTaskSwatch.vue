<script setup lang="ts">
import { TASK_SWATCH } from './taskGrid';

// Task colour: a filled square, or an outline when the task is parked (shape cue, not hue alone).
defineProps<{ color: string; parked?: boolean }>();
</script>

<template>
  <span
    :class="[TASK_SWATCH, parked ? 'border-2' : '']"
    :style="parked ? { borderColor: color } : { background: color }"
    data-testid="ade-task-swatch"
    aria-hidden="true"
  />
</template>
