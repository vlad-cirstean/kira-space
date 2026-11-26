<script setup lang="ts">
import type { ConsoleTabRecord } from '../../../state/tabDomain';
import ConsoleView from './ConsoleView.vue';

defineProps<{ tab: ConsoleTabRecord }>();
</script>

<template>
  <ConsoleView :tab="tab" />
</template>
