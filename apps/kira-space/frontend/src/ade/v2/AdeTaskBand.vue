<script setup lang="ts">
import { computed } from 'vue';
import { taskBand } from './palette';

// Faint task-colour ground shared by the task details head, review title bar and files title bar. It
// sets `--task-band` / `--task-band-hover`, the same variables card heads and span rows read. Without a
// slot (archived task) it is a plain wrapper.
const props = defineProps<{ colorSlot?: number }>();
const band = computed(() => (props.colorSlot === undefined ? null : taskBand(props.colorSlot)));
</script>

<template>
  <div
    class="shrink-0"
    :class="band && 'bg-(--task-band)'"
    :style="band ? { '--task-band': band.rest, '--task-band-hover': band.hover } : undefined"
    :data-testid="band ? 'ade-task-band' : undefined"
  >
    <slot />
  </div>
</template>
