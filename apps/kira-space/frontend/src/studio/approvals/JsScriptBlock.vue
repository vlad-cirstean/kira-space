<script setup lang="ts">
import SectionHeading from '@theme/components/SectionHeading.vue';
import type { RangeHighlight } from '@workbench/editor/ranges';
import FlushEditor from './FlushEditor.vue';

// A script the agent attached, read-only and highlighted. Shared by the API card and later the DB card.
// `rangeHighlights` marks the kept-variable calls.
defineProps<{
  label: string;
  source: string;
  testid: string;
  rangeHighlights?: (doc: string) => readonly RangeHighlight[];
}>();
</script>

<template>
  <section class="flex flex-col gap-1" :data-testid="testid">
    <SectionHeading flush :label="label" />
    <FlushEditor :doc="source" language="javascript" :range-highlights="rangeHighlights" />
  </section>
</template>
