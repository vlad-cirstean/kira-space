<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { FieldDescription } from '@theme/components/ui/field';

// Top strip of a view or detail pane: leading icon, title, meta, right-aligned actions.
defineProps<{ title?: string; icon?: string; titleTestid?: string }>();
</script>

<template>
  <div data-slot="detail-header" class="flex shrink-0 flex-col gap-0.5 px-3 py-2">
    <div class="flex min-h-control-lg min-w-0 items-center gap-2">
      <CodiconIcon v-if="icon" :name="icon" :size="16" class="shrink-0 text-muted-foreground" />
      <slot name="title">
        <h2 class="m-0 min-w-0 truncate text-kira-lg font-medium" :data-testid="titleTestid">
          {{ title }}
        </h2>
      </slot>
      <div
        v-if="$slots.meta"
        class="flex min-w-0 shrink-0 items-center gap-1.5 text-kira-sm text-subtle"
      >
        <slot name="meta" />
      </div>
      <div v-if="$slots.actions" class="ml-auto flex shrink-0 items-center gap-1">
        <slot name="actions" />
      </div>
    </div>
    <FieldDescription v-if="$slots.description"><slot name="description" /></FieldDescription>
  </div>
</template>
