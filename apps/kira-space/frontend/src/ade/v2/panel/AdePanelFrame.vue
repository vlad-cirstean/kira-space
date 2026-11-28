<script setup lang="ts" generic="T extends string">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import PanelBar from '@workbench/components/PanelBar.vue';
import { computed } from 'vue';
import AdeTaskBand from '../AdeTaskBand.vue';

// Panel body shared by task and branch mode: header slot (holds a DetailHeader) on the task colour
// band, then the tab row (tabs left, actions right), then the active tab.
const props = defineProps<{
  tabs: { value: T; label: string; count?: number }[];
  colorSlot?: number;
}>();
const items = computed(() => props.tabs.map((t) => ({ ...t, testid: `ade-panel-tab-${t.value}` })));
const tab = defineModel<T>({ required: true });
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="shrink-0" data-testid="ade-panel-head">
      <AdeTaskBand :color-slot="colorSlot">
        <slot name="header" />
      </AdeTaskBand>
    </div>
    <PanelBar class="flex-row flex-wrap items-center">
      <SecondaryTabs v-model="tab" :items="items" />
      <div
        v-if="$slots.actions"
        class="ml-auto flex shrink-0 items-center gap-1"
        data-testid="ade-panel-actions"
      >
        <slot name="actions" />
      </div>
    </PanelBar>
    <slot />
  </div>
</template>
