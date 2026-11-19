<script setup lang="ts">
import HighlightedText from '@theme/components/HighlightedText.vue';
import { KEPT_MARK_CLASS } from './keptHighlight';

// A kept variable's name in the same mark the request text carries, then its type.
defineProps<{ name: string; type: string }>();
</script>

<template>
  <span class="font-data"
    ><HighlightedText :parts="[{ text: name, match: true }]" :mark-class="KEPT_MARK_CLASS" />
    <span class="text-muted-foreground"> · {{ type }}</span></span
  >
</template>
