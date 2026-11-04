<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

const props = defineProps<{ class?: HTMLAttributes['class'] }>();
</script>

<template>
  <thead data-slot="table-header" :class="cn('sticky top-0 z-1 bg-elevated', props.class)">
    <slot />
  </thead>
</template>
