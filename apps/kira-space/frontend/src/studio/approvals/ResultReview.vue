<script setup lang="ts">
import type { SecondaryTabItem } from '@theme/components/SecondaryTabs.vue';
import { computed, ref, watch } from 'vue';
import TabbedContent from './TabbedContent.vue';

// One review of a held result, whatever the MCP: the views are tabs in one right-hand column, one slot
// `view-<value>` each; a single view shows without tabs. Under them: the script that produced the result,
// the masking note, then what the approval keeps.
const props = defineProps<{
  views: readonly SecondaryTabItem[];
  label: string;
  /** Height of a view's first line when it is not a code line, by view value (a table's row height). */
  firstLines?: Readonly<Record<string, number>>;
}>();

const view = ref(props.views[0]?.value ?? '');
// A view that disappears (the query reloads, the item changes) falls back to the first.
watch(
  () => props.views,
  (v) => {
    if (!v.some((x) => x.value === view.value)) view.value = v[0]?.value ?? '';
  },
);
const active = computed(() => (props.views.some((v) => v.value === view.value) ? view.value : ''));
</script>

<template>
  <div class="flex flex-col gap-2">
    <TabbedContent
      v-model="view"
      :tabs="views.length > 1"
      :first-line="firstLines?.[active || (views[0]?.value ?? '')]"
      :items="views"
      :label="label"
    >
      <slot :name="`view-${active || views[0]?.value}`" />
    </TabbedContent>
    <slot name="script" />
    <slot name="mask" />
    <slot name="after" />
  </div>
</template>
