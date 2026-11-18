<script setup lang="ts">
import KeyValueList from '@theme/components/KeyValueList.vue';
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
    <KeyValueList v-if="rows?.length" density="compact">
      <div
        v-for="row in rows"
        :key="row.label"
        class="contents"
        data-testid="tooltip-details-row"
        :data-label="row.label"
      >
        <dt>{{ row.label }}</dt>
        <dd class="tabular-nums wrap-anywhere" :class="row.class">{{ row.value }}</dd>
      </div>
    </KeyValueList>
    <slot />
    <FieldDescription v-if="hint" data-testid="tooltip-details-hint">{{ hint }}</FieldDescription>
  </div>
</template>
