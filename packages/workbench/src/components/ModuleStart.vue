<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';

// A module's front door (nothing open yet): icon, title, one line of copy, one primary action
// (`dialog-primary` Button in the default slot), optional `dialog` secondary actions, and an
// optional `#below` block (Studio's recents). Side-panel empties use EmptyState instead.
defineProps<{ icon: string; title: string; description?: string }>();
</script>

<template>
  <div
    data-slot="module-start"
    class="flex flex-1 min-h-0 items-center justify-center overflow-auto p-3"
  >
    <div class="flex w-105 max-w-full flex-col items-center gap-4 text-center">
      <span class="text-subtle"><CodiconIcon :name="icon" :size="24" /></span>
      <div class="text-kira-xl text-fg">{{ title }}</div>
      <div v-if="description" class="text-kira-md leading-normal text-muted-foreground">{{
        description
      }}</div>
      <div v-if="$slots.default" class="flex flex-wrap items-center justify-center gap-1.5">
        <slot />
      </div>
      <div v-if="$slots.secondary" class="flex flex-wrap items-center justify-center gap-1.5">
        <slot name="secondary" />
      </div>
      <div v-if="$slots.below" class="w-full text-left">
        <slot name="below" />
      </div>
    </div>
  </div>
</template>
