<script setup lang="ts">
import { computed } from 'vue';
import AdeTip from './AdeTip.vue';
import { type ActivityKind, activityLabel } from './activity';

// Activity glyph of a session: `!` needs input, a green dot working, `z` waiting on a monitor, an
// open dot idle, a square stopped (mockup `act`). `bare` drops the tooltip when a parent has one.
const props = defineProps<{ kind: ActivityKind; size?: 12 | 14; bare?: boolean }>();

const big = computed(() => props.size === 14);
const badge = computed(() => (big.value ? 'size-3.5' : 'size-3'));
const dot = computed(() => (big.value ? 'size-2.5' : 'size-2'));
</script>

<template>
  <AdeTip :text="bare ? '' : activityLabel(kind)">
    <span :data-activity="kind" class="inline-flex shrink-0 items-center justify-center">
      <span
        v-if="kind === 'input'"
        :class="badge"
        class="flex items-center justify-center rounded-full bg-tone-amber-solid text-kira-sm font-medium leading-none text-tone-ink"
        >!</span
      >
      <span
        v-else-if="kind === 'working'"
        :class="dot"
        class="m-0.5 inline-block rounded-full bg-tone-green-solid shadow-[0_0_0_2px_color-mix(in_srgb,var(--color-tone-green-solid)_28%,transparent)]"
      />
      <span
        v-else-if="kind === 'waiting'"
        :class="badge"
        class="flex items-center justify-center rounded-full border-2 border-tone-blue-solid text-kira-sm font-medium leading-none text-tone-blue"
        >z</span
      >
      <span
        v-else-if="kind === 'idle'"
        :class="dot"
        class="m-0.5 inline-block rounded-full border-2 border-subtle"
      />
      <span v-else class="m-0.75 inline-block size-1.5 rounded-kira-xs bg-subtle" />
    </span>
  </AdeTip>
</template>
