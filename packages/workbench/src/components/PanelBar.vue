<script setup lang="ts">
import { cn } from '@theme/lib/utils';

// P262 §2.7: sub-bar under a PanelHeader (search, view tabs, filter row).
defineProps<{ class?: string }>();
</script>

<template>
  <div data-slot="panel-bar" :class="cn('flex shrink-0 flex-col gap-1 px-1.5 py-1', $props.class)">
    <slot />
  </div>
</template>
