<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

// The trigger of a hand-built picker (a popover or dropdown that lists choices): the select look
// with a label and a chevron. Pass it `as-child` to a reka trigger. The `leading` slot holds a
// colour dot or an icon.
const props = withDefaults(
  defineProps<{ label?: string; size?: 'kira' | 'kira-lg'; class?: HTMLAttributes['class'] }>(),
  { label: undefined, size: 'kira-lg' },
);

const SIZE = { kira: 'h-control', 'kira-lg': 'h-control-lg' } as const;
</script>

<template>
  <button
    type="button"
    data-slot="picker-trigger"
    :class="
      cn(
        'inline-flex max-w-full min-w-0 shrink-0 cursor-default items-center gap-1.5 rounded-kira-sm border border-border bg-field px-2 text-kira-md text-fg aria-expanded:bg-field aria-invalid:border-error disabled:pointer-events-none disabled:opacity-50',
        SIZE[props.size],
        props.class,
      )
    "
  >
    <slot name="leading" />
    <span class="min-w-0 truncate"><slot>{{ label }}</slot></span>
    <CodiconIcon name="chevron-down" :size="12" class="shrink-0 text-muted-foreground" />
  </button>
</template>
