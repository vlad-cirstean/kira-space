<script setup lang="ts">
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';

// One property of the step: the only label layout in the inspector. Wide panes put the label in a
// column beside the control; narrow ones, and `stack`, put it above.
const props = defineProps<{ label: string; for?: string; stack?: boolean }>();
</script>

<template>
  <div
    data-slot="inspector-row"
    :class="
      stack
        ? 'flex flex-col gap-1'
        : 'grid grid-cols-1 gap-1 @[24rem]:grid-cols-[7.5rem_minmax(0,1fr)] @[24rem]:gap-x-3'
    "
  >
    <Label
      :for="props.for"
      :class="
        stack
          ? 'text-kira-sm text-muted-foreground'
          : 'text-kira-sm text-muted-foreground @[24rem]:flex @[24rem]:min-h-control-lg @[24rem]:items-center'
      "
      >{{
        label
      }}</Label
    >
    <div class="flex min-w-0 flex-col gap-1">
      <slot />
      <FieldDescription v-if="$slots.hint"><slot name="hint" /></FieldDescription>
      <FieldError v-if="$slots.error"><slot name="error" /></FieldError>
    </div>
  </div>
</template>
