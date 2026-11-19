<script setup lang="ts">
import { computed, ref } from 'vue';
import TabbedContent from './TabbedContent.vue';

// One review of a held result, whatever the MCP: with a script, the result the agent gets and the raw
// response the script read are two views, then the script itself. Without one, the raw view alone.
const props = defineProps<{
  scripted: boolean;
  /** Test id prefix: `${testid}-view-result`, `${testid}-view-raw`. */
  testid: string;
  rawLabel: string;
  resultLabel: string;
  viewsLabel: string;
}>();

const view = ref<'result' | 'raw'>('result');
const items = computed(() => [
  { value: 'result', label: props.resultLabel, testid: `${props.testid}-view-result` },
  { value: 'raw', label: props.rawLabel, testid: `${props.testid}-view-raw` },
]);
</script>

<template>
  <div class="flex flex-col gap-2">
    <TabbedContent
      v-if="scripted"
      :model-value="view"
      :tabs="true"
      :items="items"
      :label="viewsLabel"
      @update:model-value="(v) => (view = v === 'raw' ? 'raw' : 'result')"
    >
      <slot v-if="view === 'result'" name="result" />
      <slot v-else name="raw" />
    </TabbedContent>
    <slot v-else name="raw" />
    <slot v-if="scripted" name="script" />
    <slot name="after" />
  </div>
</template>
