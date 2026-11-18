<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { cn } from '@theme/lib/utils';
import { computed } from 'vue';

// View or detail pane title strip (RH.2). `wrap` lets a long title wrap instead of truncating;
// `#control` replaces the heading with an inline-edit input; default slot holds blocks below.
const props = defineProps<{
  title?: string;
  icon?: string;
  titleTestid?: string;
  as?: 'h1' | 'h2';
  wrap?: boolean;
  data?: boolean;
  flush?: boolean;
}>();
const titleClass = computed(() =>
  cn(
    'm-0 min-w-0 flex-1 text-fg',
    props.wrap
      ? 'flex min-h-control-lg items-center whitespace-normal break-words leading-snug'
      : 'truncate',
    props.data && 'font-data break-all',
  ),
);
</script>

<template>
  <div
    data-slot="detail-header"
    :class="cn('flex shrink-0 flex-col gap-0.5', !flush && 'px-3 py-2')"
  >
    <div v-if="$slots.eyebrow" class="flex min-w-0 items-center"><slot name="eyebrow" /></div>
    <div :class="cn('flex min-h-control-lg min-w-0 gap-2', wrap ? 'items-start' : 'items-center')">
      <div v-if="$slots.leading" class="flex h-control-lg shrink-0 items-center gap-1.5">
        <slot name="leading" />
      </div>
      <div v-else-if="icon" class="flex h-control-lg shrink-0 items-center">
        <CodiconIcon :name="icon" :size="16" class="text-muted-foreground" />
      </div>
      <slot v-if="$slots.control" name="control" />
      <h1
        v-else-if="as === 'h1'"
        class="text-kira-lg font-medium"
        :class="titleClass"
        :data-testid="titleTestid"
      >
        <slot name="title">{{ title }}</slot>
      </h1>
      <h2 v-else class="text-kira-lg font-medium" :class="titleClass" :data-testid="titleTestid">
        <slot name="title">{{ title }}</slot>
      </h2>
      <div
        v-if="$slots.meta"
        class="flex h-control-lg min-w-0 shrink-0 items-center gap-1.5 text-kira-sm text-subtle"
      >
        <slot name="meta" />
      </div>
      <div v-if="$slots.actions" class="ml-auto flex h-control-lg shrink-0 items-center gap-1">
        <slot name="actions" />
      </div>
    </div>
    <div
      v-if="$slots.description"
      class="flex min-w-0 flex-col gap-0.5 text-kira-sm leading-normal text-subtle"
    >
      <slot name="description" />
    </div>
    <div v-if="$slots.default" class="mt-1 flex flex-col gap-1.5"><slot /></div>
  </div>
</template>
