<script setup lang="ts">
import TextTooltip from '@theme/components/TextTooltip.vue';
import { formatDateTime } from '@theme/format';
import { formatRelative } from '@workbench/behaviour/format';

// "5m ago" with the absolute date-time in a tooltip.
defineProps<{ at: number }>();
</script>

<template>
  <TextTooltip :text="formatDateTime(at)">
    <span>{{ formatRelative(at) }}</span>
  </TextTooltip>
</template>
