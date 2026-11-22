<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import { cva } from 'class-variance-authority';
import type { HTMLAttributes } from 'vue';

// A read-only key/value grid. The default slot holds `<dt>`/`<dd>` pairs (optionally wrapped in
// `div.contents`). Compact is for sidebars and tooltips; `fixed` gives a 72px key column. `approval`
// is the approval dialog's detail grid: a 6rem key column and content-size values.
const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class'];
    density?: 'default' | 'compact' | 'approval';
    columns?: 'auto' | 'fixed' | 'approval';
  }>(),
  { density: 'default', columns: 'auto' },
);

const listVariants = cva(
  'm-0 grid items-baseline gap-x-3 [&_dd]:m-0 [&_dd]:min-w-0 [&_dt]:text-muted-foreground',
  {
    variants: {
      columns: {
        auto: 'grid-cols-[max-content_minmax(0,1fr)]',
        fixed: 'grid-cols-[4.5rem_minmax(0,1fr)]',
        approval: 'grid-cols-[6rem_minmax(0,1fr)]',
      },
      density: {
        default: 'gap-y-1 text-kira-md',
        compact: 'gap-y-0.5 text-kira-sm',
        approval:
          'gap-y-1.5 text-kira-md [&_dd]:text-fg [&_dt]:text-kira-sm [&_dt]:text-muted-foreground',
      },
    },
  },
);
</script>

<template>
  <dl
    data-slot="key-value-list"
    :data-density="density"
    :class="cn(listVariants({ density, columns }), props.class)"
  >
    <slot />
  </dl>
</template>
