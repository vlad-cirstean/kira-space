<script setup lang="ts">
import type { SecondaryTabItem } from '@theme/components/SecondaryTabs.vue';
import ApprovalBox from '@workbench/approvals/ApprovalBox.vue';
import { computed, ref, watch } from 'vue';
import ReviewRequest from './ReviewRequest.vue';
import TabbedContent from './TabbedContent.vue';

// One review of a held result, whatever the MCP: the views are tabs in one right-hand column, one slot
// `view-<value>` each; a single view shows without tabs. The views and the masking note share one box;
// the script that produced the result, what the approval saves and the request, collapsed, follow in
// boxes of their own.
const props = defineProps<{
  testidPrefix: string;
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
    <ApprovalBox :testid="`${testidPrefix}-box-result`">
      <TabbedContent
        v-model="view"
        :tabs="views.length > 1"
        :first-line="firstLines?.[active || (views[0]?.value ?? '')]"
        :items="views"
        :label="label"
      >
        <slot :name="`view-${active || views[0]?.value}`" />
      </TabbedContent>
      <slot name="mask" />
    </ApprovalBox>
    <slot name="script" />
    <slot name="after" />
    <ReviewRequest v-if="$slots.request" :testid="`${testidPrefix}-box-request`">
      <slot name="request" />
    </ReviewRequest>
  </div>
</template>
