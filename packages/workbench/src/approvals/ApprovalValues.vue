<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';

// A short list of values, one chip each: replaces comma and dot joins in a Details row.
defineProps<{ values: readonly string[]; data?: boolean }>();
</script>

<template>
  <span class="flex flex-wrap gap-1">
    <Badge v-for="v in values" :key="v" :data="data">{{ v }}</Badge>
  </span>
</template>
