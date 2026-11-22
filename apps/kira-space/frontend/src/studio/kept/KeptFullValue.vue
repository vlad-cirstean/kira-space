<script setup lang="ts">
import QueryState from '@workbench/components/QueryState.vue';
import { computed } from 'vue';
import FlushEditor from '../approvals/FlushEditor.vue';
import { indentJson } from '../beautify';
import { useKeptValue } from './keptQueries';

// The whole stored value of a temporary variable whose preview was cut, read on demand.
const props = defineProps<{ id: string }>();

const full = useKeptValue(
  () => props.id,
  () => true,
);
const doc = computed(() => {
  const json = full.data.value?.json;
  if (json === undefined) return '';
  return indentJson(json) ?? json;
});
</script>

<template>
  <QueryState :query="full" variant="inline">
    <FlushEditor :doc="doc" language="json" data-testid="kept-value-full" />
  </QueryState>
</template>
