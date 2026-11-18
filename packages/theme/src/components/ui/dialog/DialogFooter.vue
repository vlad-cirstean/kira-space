<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';
import { dialogFooterClass } from '.';

const props = defineProps<{
  class?: HTMLAttributes['class'];
}>();
</script>

<template>
  <div data-slot="dialog-footer" :class="cn(dialogFooterClass, props.class)">
    <div
      v-if="$slots.start"
      class="mr-auto flex min-w-0 items-center gap-1.5 text-kira-sm text-subtle"
    >
      <slot name="start" />
    </div>
    <slot />
  </div>
</template>
