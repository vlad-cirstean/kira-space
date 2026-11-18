<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

// Form block inside a flush popover: the same inset a default (non-flush) PopoverContent draws.
const props = defineProps<{
  class?: HTMLAttributes['class'];
}>();
</script>

<template>
  <div data-slot="popover-body" :class="cn('flex flex-col gap-2.5 p-2.5', props.class)">
    <slot />
  </div>
</template>
