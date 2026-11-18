<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

const props = defineProps<{
  class?: HTMLAttributes['class'];
}>();
</script>

<template>
  <div
    data-slot="popover-footer"
    :class="
      cn('flex items-center justify-end gap-1 border-t border-border px-1.5 py-1', props.class)
    "
  >
    <slot />
  </div>
</template>
