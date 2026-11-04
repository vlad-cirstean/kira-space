<script setup lang="ts">
import { Label } from '@theme/components/ui/label';
import { cn } from '@theme/lib/utils';
import {
  ADE_ROW,
  ADE_ROW_ACTIONS,
  ADE_ROW_BODY,
  ADE_ROW_LABEL,
  ADE_ROW_LEAD,
  rowIndent,
} from './adeRow';

// The one task-panel row: lead | label? | body | actions. `term` renders dt/dd for a `dl`.
withDefaults(
  defineProps<{
    depth?: number;
    label?: string;
    labelFor?: string;
    as?: 'div' | 'li';
    term?: boolean;
  }>(),
  { depth: 0, as: 'div' },
);
</script>

<template>
  <component
    :is="as"
    data-slot="ade-row"
    :data-depth="depth"
    :class="ADE_ROW"
    :style="rowIndent(depth)"
  >
    <span data-slot="ade-row-lead" :class="ADE_ROW_LEAD"><slot name="lead" /></span>
    <template v-if="label !== undefined || $slots.label">
      <Label v-if="labelFor" :for="labelFor" data-slot="ade-row-label" :class="cn(ADE_ROW_LABEL)"
        ><slot name="label"><span class="truncate">{{ label }}</span></slot></Label
      >
      <dt v-else-if="term" data-slot="ade-row-label" :class="ADE_ROW_LABEL"
        ><slot name="label"><span class="truncate">{{ label }}</span></slot></dt
      >
      <span v-else data-slot="ade-row-label" :class="ADE_ROW_LABEL"
        ><slot name="label"><span class="truncate">{{ label }}</span></slot></span
      >
    </template>
    <dd v-if="term" data-slot="ade-row-body" class="m-0" :class="ADE_ROW_BODY"><slot /></dd>
    <div v-else data-slot="ade-row-body" :class="ADE_ROW_BODY"><slot /></div>
    <div v-if="$slots.actions" data-slot="ade-row-actions" :class="ADE_ROW_ACTIONS"
      ><slot name="actions" /></div
    >
  </component>
</template>
