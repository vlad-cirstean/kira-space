<script setup lang="ts">
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import type { HTMLAttributes } from 'vue';

// A popover anchored to a viewport point (a click inside a canvas or grid, where the trigger element
// is gone by the next frame). reka's Popover owns placement, flip, outside click, Escape and focus;
// the zero-size fixed span is the same virtual-point trick ContextMenu uses.
defineProps<{
  /** Viewport coordinates of the anchor. */
  x: number;
  y: number;
  flush?: boolean;
  class?: HTMLAttributes['class'];
}>();
const open = defineModel<boolean>('open', { default: true });
</script>

<template>
  <Popover v-model:open="open">
    <PopoverAnchor as-child>
      <span class="fixed size-0" aria-hidden="true" :style="{ left: `${x}px`, top: `${y}px` }" />
    </PopoverAnchor>
    <PopoverContent
      align="start"
      :side-offset="6"
      avoid-collisions
      :flush="flush"
      :class="$props.class"
    >
      <slot />
    </PopoverContent>
  </Popover>
</template>
