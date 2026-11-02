<script setup lang="ts">
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { formatDateTime } from '@theme/format';
import { formatRelative } from '@workbench/behaviour/format';

// "5m ago" with the absolute date-time in a tooltip.
defineProps<{ at: number }>();
</script>

<template>
  <Tooltip>
    <TooltipTrigger as-child>
      <span>{{ formatRelative(at) }}</span>
    </TooltipTrigger>
    <TooltipContent>{{ formatDateTime(at) }}</TooltipContent>
  </Tooltip>
</template>
