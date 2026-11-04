<script setup lang="ts">
import { rowIndent } from './adeRow';

// Block under an AdeRow, aligned to its body rail.
withDefaults(defineProps<{ depth?: number; labelled?: boolean }>(), { depth: 0 });
</script>

<template>
  <div data-slot="ade-row-detail" class="flex min-w-0 gap-1 pr-1.5 pb-1" :style="rowIndent(depth)">
    <span class="w-5 shrink-0" aria-hidden="true" />
    <span v-if="labelled" class="w-24 shrink-0" aria-hidden="true" />
    <div class="min-w-0 flex-1"><slot /></div>
  </div>
</template>
