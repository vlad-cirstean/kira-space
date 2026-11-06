<script setup lang="ts">
// P110 B40: TimestampPane.vue's own `.ts-reading` repeated 4 times (local/UTC/relative/
// unparseable readings) -- extracted per plan 5.14's "4+ times in one file" rule rather than
// repeating the same utility string at each call site.
defineProps<{ subtle?: boolean }>();
</script>

<template>
  <span class="whitespace-nowrap" :class="{ 'text-subtle': subtle }"><slot /></span>
</template>
