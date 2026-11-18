<script setup lang="ts">
import VarText from '@theme/components/VarText.vue';
import type { TextPart } from '@theme/varText';

// Label column of a menu row: one truncated line, rich when `parts` is set, plus an optional muted
// second line.
defineProps<{ label: string; parts?: readonly TextPart[]; detail?: string }>();
</script>

<template>
  <span class="flex-1 min-w-0">
    <span class="block truncate">
      <VarText v-if="parts" :parts="parts" />
      <template v-else>{{ label }}</template>
    </span>
    <span v-if="detail" class="block truncate text-kira-sm text-muted-foreground">{{
      detail
    }}</span>
  </span>
</template>
