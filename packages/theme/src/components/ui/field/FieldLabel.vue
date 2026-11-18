<script setup lang="ts">
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

// `required` appends the one required-field marker after the label text.
const props = defineProps<{
  class?: HTMLAttributes['class'];
  required?: boolean;
}>();
</script>

<template>
  <Label data-slot="field-label" :class="cn('text-muted-foreground', props.class)">
    <slot />
    <span v-if="required" class="text-error"
      >*<span class="sr-only"> {{ t('common.required.label') }}</span></span
    >
  </Label>
</template>
