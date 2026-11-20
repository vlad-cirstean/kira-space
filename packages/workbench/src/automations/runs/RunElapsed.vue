<script setup lang="ts">
import { useIntervalFn } from '@vueuse/core';
import { formatElapsed } from '@workbench/behaviour/format';
import { TIMING } from '@workbench/behaviour/timing';
import { computed, ref, watchEffect } from 'vue';
import { type RunTimes, runElapsedMs } from './runText';

// A run's elapsed time; ticks each second only while the run is live.
const props = defineProps<{ run: RunTimes }>();
const now = ref(Date.now());
const { pause } = useIntervalFn(() => {
  now.value = Date.now();
}, TIMING.clockTick);
watchEffect(() => {
  if (props.run.state !== 'running') pause();
});
const label = computed(() => {
  const ms = runElapsedMs(props.run, now.value);
  return ms === null ? '' : formatElapsed(ms);
});
</script>

<template>
  <span class="font-data" data-testid="run-elapsed">{{ label }}</span>
</template>
