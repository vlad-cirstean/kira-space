<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { agentApiResultSchema, apiResponsePayloadSchema } from '@shared/domain/dbmcp';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import ApprovalBox from '@workbench/approvals/ApprovalBox.vue';
import ApprovalCode from '@workbench/approvals/ApprovalCode.vue';
import QueryState from '@workbench/components/QueryState.vue';
import { computed } from 'vue';
import { indentJson } from '../beautify';
import ApiRequestView from './ApiRequestView.vue';
import ApiResponseView from './ApiResponseView.vue';
import FlushEditor from './FlushEditor.vue';
import JsScriptBlock from './JsScriptBlock.vue';
import { usePendingResponse } from './pendingResponse';
import ResultReview from './ResultReview.vue';
import StagedKeepList from './StagedKeepList.vue';

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
// Values the script keeps once this result is sent (P678): not stored yet, previews only.
const keep = computed(() => response.data.value?.keep ?? []);
const failed = computed(() => payload.value.isError);
// Without a script the raw response is what the agent gets: one view.
const views = computed(() => {
  const raw = {
    value: 'raw',
    label: t('studio.http.diff.before.label'),
    testid: 'api-review-view-raw',
  };
  return payload.value.scripted
    ? [
        {
          value: 'result',
          label: t('studio.http.diff.after.label'),
          testid: 'api-review-view-result',
        },
        raw,
      ]
    : [raw];
});
const resultJson = computed(() => (failed.value ? null : indentJson(text.value)));
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
  <div class="flex flex-col gap-2" data-testid="api-review-body">
    <ApprovalBox v-if="!response.isSuccess.value" testid="api-review-box-state">
      <QueryState :query="response" variant="inline" :is-empty="() => false" />
    </ApprovalBox>
    <QueryState v-else :query="response" variant="inline" :is-empty="() => false">
      <ResultReview
        testid-prefix="api-review"
        :views="views"
        :label="t('studio.dbMcp.approval.api.responseViews.aria')"
      >
        <template #request>
          <ApiRequestView
            :method="payload.method"
            :url="payload.url"
            :headers="payload.request.headers"
            :body="payload.request.body"
            :variables="payload.request.variables"
            :kept="payload.request.kept"
          />
        </template>
        <template #view-raw>
          <ApiResponseView
            v-if="parsedRaw"
            :response="parsedRaw"
            :plan-url="payload.url"
            testid="api-review-raw"
          />
          <ApprovalCode v-else data-testid="api-review-raw">{{ raw || text }}</ApprovalCode>
        </template>
        <template #view-result>
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
        <template #mask>
          <span
            v-if="payload.masked.length"
            class="text-kira-sm text-subtle"
            data-testid="api-review-mask"
            >{{
              t('studio.dbMcp.approval.api.masked.status', { names: payload.masked.join(', ') })
            }}</span
          >
        </template>
        <template #after>
          <ApprovalBox v-if="keep.length" testid="api-review-box-keep">
            <StagedKeepList :keep="keep" :request-id="requestId" testid="api-review-keep" />
          </ApprovalBox>
        </template>
        <template #script>
          <ApprovalBox v-if="script" testid="api-review-box-script">
            <JsScriptBlock
              :label="t('studio.dbMcp.approval.api.scriptTransform.label')"
              :source="script.source"
              testid="api-review-script"
            />
          </ApprovalBox>
        </template>
      </ResultReview>
    </QueryState>
  </div>
</template>
