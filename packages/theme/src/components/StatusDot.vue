<script setup lang="ts">
import { computed } from 'vue';

// P263 §2.2: one 8px status dot. `color` is a data colour (e.g. an ADE task colour) and wins over
// `tone`; `pulse` marks a transitional state; `label` exposes it to assistive tech.
const props = withDefaults(
  defineProps<{
    tone?: 'ok' | 'warn' | 'error' | 'info' | 'muted' | 'state-on';
    color?: string;
    pulse?: boolean;
    label?: string;
  }>(),
  { tone: 'muted', color: undefined, pulse: false, label: undefined },
);

const TONE_CLASS = {
  ok: 'bg-ok',
  warn: 'bg-warn',
  error: 'bg-error',
  info: 'bg-info',
  muted: 'bg-subtle',
  'state-on': 'bg-state-on',
} as const;

const toneClass = computed(() => (props.color ? undefined : TONE_CLASS[props.tone]));
</script>

<template>
  <span
    data-slot="status-dot"
    class="inline-block size-2 shrink-0 rounded-full"
    :class="[toneClass, pulse && 'animate-pulse']"
    :style="color ? { backgroundColor: color } : undefined"
    :role="label ? 'img' : undefined"
    :aria-label="label"
  />
</template>
