<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import { cva } from 'class-variance-authority';
import type { HTMLAttributes } from 'vue';

// A read-only key/value grid. The default slot holds `<dt>`/`<dd>` pairs. Compact is for sidebars
// and tooltips.
const props = withDefaults(
  defineProps<{ class?: HTMLAttributes['class']; density?: 'default' | 'compact' }>(),
  { density: 'default' },
);

const listVariants = cva(
  'm-0 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-3 [&>dd]:m-0 [&>dd]:min-w-0 [&>dt]:text-muted-foreground',
  {
    variants: {
      density: { default: 'gap-y-1 text-kira-md', compact: 'gap-y-0.5 text-kira-sm' },
    },
  },
);
</script>

<template>
  <dl
    data-slot="key-value-list"
    :data-density="density"
    :class="cn(listVariants({ density }), props.class)"
  >
    <slot />
  </dl>
</template>
