<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

const props = defineProps<{ class?: HTMLAttributes['class'] }>();
</script>

<template>
  <div
    data-slot="card-footer"
    :class="
      cn(
        'flex items-center gap-1.5 group-data-[variant=section]/card:px-3 group-data-[variant=section]/card:py-2',
        props.class,
      )
    "
  >
    <slot />
  </div>
</template>
