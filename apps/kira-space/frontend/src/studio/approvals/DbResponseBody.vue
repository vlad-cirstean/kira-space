<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbResponsePayloadSchema } from '@shared/domain/dbmcp';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import QueryState from '@workbench/components/QueryState.vue';
import { computed } from 'vue';
import { indentJson } from '../beautify';
import FlushEditor from './FlushEditor.vue';
import JsScriptBlock from './JsScriptBlock.vue';
import PendingResponse from './PendingResponse.vue';
import { usePendingResponse } from './pendingResponse';
import ResultReview from './ResultReview.vue';
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
const resultJson = computed(() => (failed.value ? null : indentJson(text.value)));
</script>

<template>
  <div data-testid="db-mcp-preview-body">
    <PendingResponse
      v-if="!payload.scripted"
      :request-id="requestId"
      :connection-id="payload.connectionId"
    />
    <QueryState v-else :query="response" variant="inline" :is-empty="() => false">
      <ResultReview
        testid="db-review"
        :scripted="true"
        :raw-label="t('studio.dbMcp.approval.query.responseRaw.label')"
        :result-label="t('common.agentGets.label')"
        :views-label="t('studio.dbMcp.approval.query.responseViews.aria')"
      >
        <template #raw>
          <PendingResponse
            :request-id="requestId"
            :connection-id="payload.connectionId"
            field="raw"
          />
        </template>
        <template #result>
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
          <StagedKeepList v-if="keep.length" :keep="keep" testid="db-review-keep" />
        </template>
        <template #script>
          <JsScriptBlock
            v-if="script"
            :label="t('studio.dbMcp.approval.query.scriptResult.label')"
            :source="script.source"
            testid="db-review-script"
          />
        </template>
      </ResultReview>
    </QueryState>
  </div>
</template>
