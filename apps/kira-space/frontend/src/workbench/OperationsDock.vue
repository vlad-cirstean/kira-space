<script setup lang="ts">
import { computed } from 'vue';
import { useModeStore } from '../state/mode';
import StudioOperationsPanel from '../studio/workbench/panels/OperationsPanel.vue';
import { STUDIO_MODES } from './modes';
import OperationsPanel from './OperationsPanel.vue';

// Two op feeds: the database and API modules log to Studio's, every other module to Space's.

const modeStore = useModeStore();
const studioFeed = computed(() => STUDIO_MODES.has(modeStore.active));
</script>

<template>
  <StudioOperationsPanel v-if="studioFeed" />
  <OperationsPanel v-else />
</template>
