<script setup lang="ts">
import { computed } from 'vue';
import { useConnectionsStore } from '../../state/connections';
import type { ConsoleTabRecord } from '../../state/tabDomain';
import ConsoleView from './ConsoleView.vue';
import KibanaDiscoverView from './kibana/KibanaDiscoverView.vue';

const props = defineProps<{ tab: ConsoleTabRecord }>();
const connectionsStore = useConnectionsStore();

const isKibana = computed(
  () => connectionsStore.connectionRecord(props.tab.connectionId)?.kind === 'kibana',
);
</script>

<template>
  <KibanaDiscoverView v-if="isKibana" :tab="tab" />
  <ConsoleView v-else :tab="tab" />
</template>
