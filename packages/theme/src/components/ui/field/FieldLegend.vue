<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';
import { useSlots } from 'vue';

const props = defineProps<{
  class?: HTMLAttributes['class'];
}>();
const slots = useSlots();
</script>

<template>
  <legend
    data-slot="field-legend"
    :class="
      cn(
        'uppercase text-kira-sm text-subtle tracking-wider pt-1 px-0',
        slots.actions && 'flex w-full items-center justify-between gap-1',
        props.class,
      )
    "
  >
    <template v-if="slots.actions">
      <span><slot /></span>
      <span class="flex items-center gap-1 normal-case tracking-normal"
        ><slot name="actions" /></span
      >
    </template>
    <slot v-else />
  </legend>
</template>
