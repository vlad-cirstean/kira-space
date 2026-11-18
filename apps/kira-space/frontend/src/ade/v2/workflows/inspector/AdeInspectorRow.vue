<script setup lang="ts">
import { Field, FieldDescription, FieldError } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';

// One property of the step: the only label layout in the inspector. Wide panes put the label in a
// column beside the control; narrow ones, and `stack`, put it above.
const props = defineProps<{ label: string; for?: string; stack?: boolean }>();
</script>

<template>
  <Field :orientation="stack ? 'vertical' : 'responsive'">
    <Label :for="props.for" class="text-kira-sm text-muted-foreground">{{ label }}</Label>
    <div class="flex min-w-0 flex-col gap-1">
      <slot />
      <FieldDescription v-if="$slots.hint"><slot name="hint" /></FieldDescription>
      <FieldError v-if="$slots.error"><slot name="error" /></FieldError>
    </div>
  </Field>
</template>
