<script setup lang="ts">
import { ACTION_CLASS } from '../tones';
import type { SessionView } from './sessionView';

defineProps<{ view: SessionView }>();
</script>

<template>
  <span
    class="shrink-0 rounded-kira-sm px-1 text-kira-sm"
    :class="view.headless ? 'border border-border text-muted-foreground' : ACTION_CLASS.agent"
    data-testid="ade-session-badge"
    >{{
      view.badge
    }}</span
  >
</template>
