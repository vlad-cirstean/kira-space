<script setup lang="ts">
import TextTooltip from '@theme/components/TextTooltip.vue';
import { formatDateTime } from '@theme/format';
import { useRelativeTime } from '@workbench/behaviour/format';
import { computed } from 'vue';

// A standalone instant: "5m ago" / "in 2d", kept current, with the absolute date-time in a tooltip.
// A composable cannot be called per v-for row, so every row renders this instead.
const props = defineProps<{ at: number | string | Date }>();
const text = useRelativeTime(() => props.at);
const millis = computed(() => new Date(props.at).getTime());
const iso = computed(() =>
  Number.isNaN(millis.value) ? undefined : new Date(millis.value).toISOString(),
);
</script>

<template>
  <TextTooltip :text="Number.isNaN(millis) ? null : formatDateTime(millis)">
    <time :datetime="iso">{{ text }}</time>
  </TextTooltip>
</template>
