<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

const props = defineProps<{
  class?: HTMLAttributes['class'];
}>();
</script>

<template>
  <div data-slot="popover-bar" :class="cn('flex flex-none items-center gap-1 p-1', props.class)">
    <slot />
  </div>
</template>
