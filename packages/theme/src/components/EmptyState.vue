<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import Spinner from '@theme/components/Spinner.vue';
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@theme/components/ui/empty';
import { useSpinnerDelay } from '@theme/lib/useSpinnerDelay';

// The one empty / unavailable composition. `media` only for a content illustration; loading is the
// `loading` prop. Default slot = actions (Retry, a primary button). `fill` takes the full height of
// the parent. `loading` swaps the icon for a spinner; the whole state stays invisible for
// `SPINNER_DELAY` so a fast load never flashes. Side panels pass no actions; a module front door
// uses ModuleStart instead.
const props = defineProps<{
  icon?: string;
  title: string;
  description?: string;
  fill?: boolean;
  loading?: boolean;
}>();

const shown = useSpinnerDelay(() => props.loading === true);
</script>

<template>
  <Empty
    data-slot="empty-state"
    :class="['p-6', fill && 'h-full', loading && !shown && 'invisible']"
  >
    <EmptyHeader>
      <EmptyMedia v-if="loading || icon || $slots.media"
        ><Spinner v-if="loading" :size="24" label="" />
        <slot v-else name="media"
          ><CodiconIcon v-if="icon" :name="icon" :size="24" /></slot
        ></EmptyMedia
      >
      <EmptyTitle>{{ title }}</EmptyTitle>
      <EmptyDescription v-if="description">{{ description }}</EmptyDescription>
    </EmptyHeader>
    <EmptyContent v-if="$slots.default"><slot /></EmptyContent>
  </Empty>
</template>
