<script setup lang="ts">
import { computed } from 'vue';

// A colour as a 3px line: `edge` overlays the top of a relative header, `under` the bottom of a
// title bar (directly above the content), `mark` sits inline before a name. `inset` pulls an
// `under` line in from the sides, to match content narrower than its bar.
const props = defineProps<{
  color: string;
  variant: 'edge' | 'under' | 'mark';
  inset?: { left?: string; right?: string };
}>();
const CLASS = {
  edge: 'pointer-events-none absolute inset-x-0 top-0 h-0.75',
  under: 'pointer-events-none absolute inset-x-0 bottom-0 h-0.75',
  mark: 'inline-block h-0.75 w-4 shrink-0 rounded-full',
};
const style = computed(() => ({
  background: props.color,
  ...(props.variant === 'under' && props.inset?.left ? { left: props.inset.left } : {}),
  ...(props.variant === 'under' && props.inset?.right ? { right: props.inset.right } : {}),
}));
</script>

<template>
  <span :class="CLASS[variant]" :style="style" aria-hidden="true" />
</template>
