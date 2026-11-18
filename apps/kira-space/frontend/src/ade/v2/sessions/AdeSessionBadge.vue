<script setup lang="ts">
import type { SessionView } from './sessionView';

defineProps<{ view: SessionView }>();
</script>

<template>
  <span
    class="shrink-0 rounded-kira-sm px-1 text-kira-sm"
    :class="
      view.headless
        ? 'border border-border text-muted-foreground'
        : 'bg-agent text-chrome border-transparent'
    "
    data-testid="ade-session-badge"
    >{{
      view.badge
    }}</span
  >
</template>
