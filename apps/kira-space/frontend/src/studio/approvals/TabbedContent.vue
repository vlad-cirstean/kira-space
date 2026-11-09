<script setup lang="ts">
import SecondaryTabs, { type SecondaryTabItem } from '@theme/components/SecondaryTabs.vue';
import { useCodeLineHeight } from './codeLine';

// Content with text tabs in their own right column, so code never runs under them. The tab row is
// one code line tall, so the tab words centre on the content's first line.
defineProps<{ items: readonly SecondaryTabItem[]; label: string; tabs: boolean }>();
const view = defineModel<string>({ required: true });
const lineHeight = useCodeLineHeight();
</script>

<template>
  <div class="grid grid-cols-[minmax(0,1fr)_auto] gap-x-3">
    <div class="min-w-0"><slot /></div>
    <div v-if="tabs" class="flex items-center" :style="{ height: `${lineHeight}px` }">
      <SecondaryTabs v-model="view" variant="text" :items="items" :aria-label="label" />
    </div>
  </div>
</template>
