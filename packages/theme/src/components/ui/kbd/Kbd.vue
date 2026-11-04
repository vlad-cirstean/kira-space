<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

const props = defineProps<{ class?: HTMLAttributes['class'] }>();
</script>

<template>
  <kbd
    data-slot="kbd"
    :class="
      cn(
        'pointer-events-none inline-flex h-4 min-w-4 select-none items-center justify-center rounded-kira-xs border border-border bg-field px-1 font-ui text-kira-sm text-muted-foreground',
        props.class,
      )
    "
  >
    <slot />
  </kbd>
</template>
