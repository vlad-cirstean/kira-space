<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { agentApiResultSchema, apiResponsePayloadSchema } from '@shared/domain/dbmcp';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import ApprovalCode from '@workbench/approvals/ApprovalCode.vue';
import QueryState from '@workbench/components/QueryState.vue';
import { computed } from 'vue';
import ApiResponseView from './ApiResponseView.vue';
import FlushEditor from './FlushEditor.vue';
import JsScriptBlock from './JsScriptBlock.vue';
import { usePendingResponse } from './pendingResponse';
import { prettyJson } from './prettyJson';
import ResultReview from './ResultReview.vue';

// The held API response: what the agent gets on Send, and with a script, the raw response it read and
// the script. Every byte shown comes from the pending-response query, never from the item payload.
const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => apiResponsePayloadSchema.parse(props.item.payload));
// The item id is "api.response:<requestId>"; the held response is fetched by request id.
const requestId = computed(() => props.item.id.slice(props.item.kind.length + 1));
const response = usePendingResponse(() => requestId.value);

const text = computed(() => response.data.value?.text ?? '');
const raw = computed(() => response.data.value?.raw ?? '');
const script = computed(() => response.data.value?.script ?? null);
const failed = computed(() => payload.value.isError);
const resultJson = computed(() => (failed.value ? null : prettyJson(text.value)));
// Never hide bytes: an unparseable response falls back to its verbatim text.
const parsedRaw = computed(() => {
  const source = raw.value || text.value;
  try {
    const r = agentApiResultSchema.safeParse(JSON.parse(source));
    return r.success && r.data.status > 0 ? r.data : null;
  } catch {
    return null;
  }
});
</script>

<template>
  <div data-testid="api-review-body">
    <QueryState :query="response" variant="inline" :is-empty="() => false">
      <ResultReview
        testid="api-review"
        :scripted="payload.scripted"
        :raw-label="t('studio.dbMcp.approval.api.responseRaw.label')"
        :result-label="t('studio.dbMcp.approval.api.responseResult.label')"
        :views-label="t('studio.dbMcp.approval.api.responseViews.aria')"
      >
        <template #raw>
          <ApiResponseView
            v-if="parsedRaw"
            :response="parsedRaw"
            :plan-url="payload.url"
            testid="api-review-raw"
          />
          <ApprovalCode v-else data-testid="api-review-raw">{{ raw || text }}</ApprovalCode>
        </template>
        <template #result>
          <FieldError v-if="failed" class="whitespace-pre-wrap" data-testid="api-review-result">{{
            text
          }}</FieldError>
          <FlushEditor
            v-else-if="resultJson !== null"
            :doc="resultJson"
            language="json"
            data-testid="api-review-result"
          />
          <ApprovalCode v-else data-testid="api-review-result">{{ text }}</ApprovalCode>
        </template>
        <template #script>
          <JsScriptBlock
            v-if="script"
            :label="t('studio.dbMcp.approval.api.scriptTransform.label')"
            :source="script.source"
            testid="api-review-script"
          />
        </template>
      </ResultReview>
    </QueryState>
  </div>
</template>
