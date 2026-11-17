<script setup lang="ts">
import { rowMeta } from '../panel/adeRow';
import type { BranchRowModel } from './usePlanModel';

// A branch row's merged and deployed targets, split by a divider.
defineProps<{ chips: BranchRowModel['chips'] }>();
</script>

<template>
  <span
    v-for="c in chips.merged"
    :key="c.label"
    class="shrink-0 text-kira-sm"
    :class="c.tone === 'stale' ? 'text-tone-amber' : rowMeta"
    data-testid="ade-branch-merged"
    >{{
      c.label
    }}</span
  >
  <span v-if="chips.divider" class="h-2.5 w-px shrink-0 bg-border" />
  <span
    v-for="c in chips.deployed"
    :key="c.label"
    class="shrink-0 text-kira-sm"
    :class="c.tone === 'stale' ? 'text-tone-amber' : rowMeta"
    data-testid="ade-branch-deployed"
    >{{
      c.label
    }}</span
  >
</template>
