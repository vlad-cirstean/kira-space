<script setup lang="ts">
import { TASK_LINE_EDGE, TASK_LINE_MARK, TASK_LINE_UNDER } from './taskGrid';

// Task colour as a line: `edge` overlays the top of a relative header, `under` the bottom of a
// title bar (directly above the content), `mark` sits inline before a name.
const props = defineProps<{ color: string; variant: 'edge' | 'under' | 'mark' }>();
const CLASS = { edge: TASK_LINE_EDGE, under: TASK_LINE_UNDER, mark: TASK_LINE_MARK };
</script>

<template>
  <span
    :class="CLASS[props.variant]"
    :style="{ background: color }"
    data-testid="ade-task-line"
    aria-hidden="true"
  />
</template>
