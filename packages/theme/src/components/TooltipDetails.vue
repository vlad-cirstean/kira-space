<script setup lang="ts">
import { FieldDescription } from '@theme/components/ui/field';

// Body for any tooltip with more than one fact: a title, label/value rows, a closing hint. Never
// join facts into one line with a dash or a dot.
defineProps<{
  title?: string;
  rows?: readonly { label: string; value: string; class?: string }[];
  hint?: string;
}>();
</script>

<template>
  <div class="flex flex-col gap-1 py-0.5 text-left whitespace-normal" data-testid="tooltip-details">
    <div v-if="title" class="font-medium text-fg wrap-anywhere" data-testid="tooltip-details-title">
      {{ title }}
    </div>
    <dl v-if="rows?.length" class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
      <div
        v-for="row in rows"
        :key="row.label"
        class="contents"
        data-testid="tooltip-details-row"
        :data-label="row.label"
      >
        <dt class="text-muted-foreground">{{ row.label }}</dt>
        <dd class="tabular-nums wrap-anywhere" :class="row.class">{{ row.value }}</dd>
      </div>
    </dl>
    <slot />
    <FieldDescription v-if="hint" data-testid="tooltip-details-hint">{{ hint }}</FieldDescription>
  </div>
</template>
