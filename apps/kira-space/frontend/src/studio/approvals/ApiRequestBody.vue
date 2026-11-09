<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { apiRequestPayloadSchema } from '@shared/domain/dbmcp';
import ApprovalCode from '@workbench/approvals/ApprovalCode.vue';
import ApprovalPairs from '@workbench/approvals/ApprovalPairs.vue';
import { computed } from 'vue';
import FlushEditor from './FlushEditor.vue';

// What the agent would send: path and query, then the body. Host, headers and variables sit on
// line 1 and in the details. A credential's value never reaches this view.
const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => apiRequestPayloadSchema.parse(props.item.payload));

const pathAndQuery = computed(() => {
  try {
    const u = new URL(payload.value.url);
    return u.pathname + u.search;
  } catch {
    return payload.value.url;
  }
});

const json = computed(() => {
  const b = payload.value.body;
  if (!b || b.form || !/json/i.test(b.contentType) || b.truncated) return null;
  try {
    return JSON.stringify(JSON.parse(b.text), null, 2);
  } catch {
    return null;
  }
});
</script>

<template>
  <div class="flex flex-col gap-1" data-testid="api-approval-body">
    <ApprovalCode data-testid="api-approval-url">{{ pathAndQuery }}</ApprovalCode>
    <template v-if="payload.body">
      <ApprovalPairs
        v-if="payload.body.form"
        :rows="payload.body.form"
        testid="api-approval-form"
      />
      <FlushEditor
        v-else-if="json !== null"
        :doc="json"
        language="json"
        data-testid="api-approval-body-text"
      />
      <ApprovalCode v-else data-testid="api-approval-body-text">{{
        payload.body.text
      }}</ApprovalCode>
    </template>
  </div>
</template>
