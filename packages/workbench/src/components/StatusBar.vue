<script setup lang="ts">
// Shared bar chrome for both apps. Per-app content comes through the two slots: Kira Space's
// update, blame and agent-usage items on the left, agents/ops/runs/app-metrics on the right.
</script>

<template>
  <div
    class="h-statusbar shrink-0 flex items-center gap-2 px-2 font-ui text-kira-sm text-muted-foreground"
  >
    <div v-if="$slots['left-extra']" class="flex items-center gap-1" data-testid="status-bar-left">
      <slot name="left-extra" />
    </div>
    <div class="ml-auto flex items-center gap-1" data-testid="status-bar-right">
      <slot name="right" />
    </div>
  </div>
</template>
