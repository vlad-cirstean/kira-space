<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbResponsePayloadSchema } from '@shared/domain/dbmcp';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import ApprovalBox from '@workbench/approvals/ApprovalBox.vue';
import QueryState from '@workbench/components/QueryState.vue';
import { computed } from 'vue';
import { indentJson } from '../beautify';
import DbRequestView from './DbRequestView.vue';
import FlushEditor from './FlushEditor.vue';
import JsScriptBlock from './JsScriptBlock.vue';
import PendingResponse from './PendingResponse.vue';
import { usePendingResponse } from './pendingResponse';
import ResultReview from './ResultReview.vue';
import ReviewRequest from './ReviewRequest.vue';
import StagedKeepList from './StagedKeepList.vue';

// The held run_query result. Without a script: the rows and the exact bytes the agent gets on Send. With one
// (P680): the result the agent gets and the rows the script read are two views, then the script and the values
// it keeps. Every byte shown comes from the pending-response query, never from the item payload.
const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => dbResponsePayloadSchema.parse(props.item.payload));
// The item id is "db.response:<requestId>"; the held result is fetched by request id.
const requestId = computed(() => props.item.id.slice(props.item.kind.length + 1));
const response = usePendingResponse(() => requestId.value);

const text = computed(() => response.data.value?.text ?? '');
const script = computed(() => response.data.value?.script ?? null);
const keep = computed(() => response.data.value?.keep ?? []);
const failed = computed(() => payload.value.isError);
const views = computed(() => [
  {
    value: 'result',
    label: t('studio.http.diff.after.label'),
    testid: 'db-review-view-result',
  },
  {
    value: 'raw',
    label: t('studio.http.diff.before.label'),
    testid: 'db-review-view-raw',
  },
]);
const resultJson = computed(() => (failed.value ? null : indentJson(text.value)));
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="db-mcp-preview-body">
    <ReviewRequest v-if="!payload.scripted" testid="db-review-box-request">
      <DbRequestView
        :kind="item.target.kind"
        :statement="payload.statement"
        :kept="payload.kept"
        :params="payload.params"
        :placeholders="payload.placeholders"
      />
    </ReviewRequest>
    <ApprovalBox v-if="!payload.scripted" testid="db-review-box-result">
      <PendingResponse :request-id="requestId" :connection-id="payload.connectionId" />
    </ApprovalBox>
    <ApprovalBox v-else-if="!response.isSuccess.value" testid="db-review-box-state">
      <QueryState :query="response" variant="inline" :is-empty="() => false" />
    </ApprovalBox>
    <QueryState v-else :query="response" variant="inline" :is-empty="() => false">
      <ResultReview
        testid-prefix="db-review"
        :views="views"
        :label="t('studio.dbMcp.approval.query.responseViews.aria')"
      >
        <template #request>
          <DbRequestView
            :kind="item.target.kind"
            :statement="payload.statement"
            :kept="payload.kept"
            :params="payload.params"
            :placeholders="payload.placeholders"
          />
        </template>
        <template #view-raw>
          <PendingResponse
            :request-id="requestId"
            :connection-id="payload.connectionId"
            field="raw"
          />
        </template>
        <template #view-result>
          <FieldError v-if="failed" class="whitespace-pre-wrap" data-testid="db-review-result">{{
            text
          }}</FieldError>
          <FlushEditor
            v-else
            :doc="resultJson ?? text"
            language="json"
            data-testid="db-review-result"
          />
        </template>
        <template #after>
          <ApprovalBox v-if="keep.length" testid="db-review-box-keep">
            <StagedKeepList :keep="keep" :request-id="requestId" testid="db-review-keep" />
          </ApprovalBox>
        </template>
        <template #script>
          <ApprovalBox v-if="script" testid="db-review-box-script">
            <JsScriptBlock
              :label="t('studio.dbMcp.approval.query.scriptResult.label')"
              :source="script.source"
              testid="db-review-script"
            />
          </ApprovalBox>
        </template>
      </ResultReview>
    </QueryState>
  </div>
</template>
