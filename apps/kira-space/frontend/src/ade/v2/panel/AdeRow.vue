<script setup lang="ts">
import {
  ADE_ROW,
  ADE_ROW_ACTIONS,
  ADE_ROW_BODY,
  ADE_ROW_LEAD,
  ADE_ROW_TIME,
  rowIndent,
} from './adeRow';

// The one task-panel row: lead | time? | body | actions. `label` is the time column of a log row,
// never a form label (forms use Field).
withDefaults(defineProps<{ depth?: number; label?: string; as?: 'div' | 'li' }>(), {
  depth: 0,
  as: 'div',
});
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
    <span v-if="label !== undefined" data-slot="ade-row-time" :class="ADE_ROW_TIME"
      ><span class="truncate">{{ label }}</span></span
    >
    <div data-slot="ade-row-body" :class="ADE_ROW_BODY"><slot /></div>
    <div v-if="$slots.actions" data-slot="ade-row-actions" :class="ADE_ROW_ACTIONS"
      ><slot name="actions" /></div
    >
  </component>
</template>
