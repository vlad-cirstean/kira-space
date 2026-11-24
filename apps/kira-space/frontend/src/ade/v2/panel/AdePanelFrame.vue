<script setup lang="ts" generic="T extends string">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import ColorLine from '@workbench/components/ColorLine.vue';
import PanelBar from '@workbench/components/PanelBar.vue';
import { computed } from 'vue';

// Panel body shared by task and branch mode: header slot (holds a DetailHeader), the task colour
// line under it, then the tab row (tabs left, actions right), then the active tab.
const props = defineProps<{
  tabs: { value: T; label: string; count?: number }[];
  color?: string;
}>();
const items = computed(() => props.tabs.map((t) => ({ ...t, testid: `ade-panel-tab-${t.value}` })));
const tab = defineModel<T>({ required: true });
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="relative shrink-0" :class="color && 'pb-0.75'" data-testid="ade-panel-head">
      <slot name="header" />
      <ColorLine v-if="color" data-testid="ade-task-line" variant="under" :color="color" />
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
