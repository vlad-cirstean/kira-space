<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

// The h-bar shrink-0 flex items-center gap-1.5 px-2 recipe, carried once across the views.
// `class` reaches the root via cn(), so a per-site extra like bg-elevated keeps working.
const props = defineProps<{ class?: HTMLAttributes['class'] }>();
</script>

<template>
  <div :class="cn('h-bar shrink-0 flex items-center gap-1.5 px-2', props.class)">
    <slot />
  </div>
</template>
