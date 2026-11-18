<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

const props = defineProps<{
  class?: HTMLAttributes['class'];
}>();
</script>

<template>
  <div data-slot="popover-list" :class="cn('flex flex-col gap-0.5 p-1', props.class)">
    <slot />
  </div>
</template>
