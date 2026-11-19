<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { computed, ref } from 'vue';

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
    <template v-if="scripted">
      <SecondaryTabs
        :model-value="view"
        :items="items"
        :aria-label="viewsLabel"
        @update:model-value="(v) => (view = v === 'raw' ? 'raw' : 'result')"
      />
      <slot v-if="view === 'result'" name="result" />
      <slot v-else name="raw" />
      <slot name="script" />
    </template>
    <slot v-else name="raw" />
    <slot name="after" />
  </div>
</template>
