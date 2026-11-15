<script setup lang="ts">
import { Toaster } from '@theme/components/ui/sonner';
import { useElementSize } from '@vueuse/core';
import { computed, useTemplateRef } from 'vue';

const GAP_PX = 8;
const EDGE_PX = 16;

// Chips stay their own surfaces: Sonner's deck collapses and drops toasts, but chips must stay visible and actionable.
const el = useTemplateRef<HTMLElement>('stack');
const { height } = useElementSize(el, undefined, { box: 'border-box' });

const top = computed(() => {
  const base = `calc(var(--kira-titlebar-h) + ${GAP_PX}px`;
  return height.value > 0 ? `${base} + ${height.value}px + ${GAP_PX}px)` : `${base})`;
});
const offset = computed(() => ({ top: top.value, right: `${EDGE_PX}px` }));
const mobileOffset = computed(() => ({
  top: top.value,
  left: `${EDGE_PX}px`,
  right: `${EDGE_PX}px`,
}));
</script>

<template>
  <div
    ref="stack"
    class="pointer-events-none fixed top-[calc(var(--kira-titlebar-h)+8px)] right-4 z-(--kira-z-dialog) flex flex-col items-end gap-2 *:pointer-events-auto"
    data-testid="corner-stack"
  >
    <slot />
  </div>
  <Toaster position="top-right" :offset="offset" :mobile-offset="mobileOffset" />
</template>
