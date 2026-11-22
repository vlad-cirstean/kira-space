<script setup lang="ts">
import HighlightedText from '@theme/components/HighlightedText.vue';
import { KEPT_MARK_CLASS } from './keptHighlight';

// A temporary variable's name in the same mark the request text carries, then its type when given.
defineProps<{ name: string; type?: string }>();
</script>

<template>
  <span class="inline-flex min-w-0 flex-wrap items-center gap-x-1 font-data"
    ><HighlightedText :parts="[{ text: name, match: true }]" :mark-class="KEPT_MARK_CLASS" />
    <span v-if="type" class="text-muted-foreground"> · {{ type }}</span></span
  >
</template>
