<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import {
  kubeDescribeResultSchema,
  kubeListResultSchema,
  kubeLogsResultSchema,
  kubeResponsePayloadSchema,
} from '@shared/domain/dbmcp';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import QueryState from '@workbench/components/QueryState.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { computed } from 'vue';
import { indentJson } from '../beautify';
import FlushEditor from './FlushEditor.vue';
import JsScriptBlock from './JsScriptBlock.vue';
import { usePendingResponse } from './pendingResponse';
import ResultReview from './ResultReview.vue';
import ReviewLog from './ReviewLog.vue';
import ReviewTable from './ReviewTable.vue';
import StagedKeepList from './StagedKeepList.vue';

// The held kube_list_resources, kube_describe or kube_pod_logs result: the operation's own view first
// (table, manifest, log lines), then the JSON the agent gets on Send. With a script, "Agent gets" comes first
// and the operation's views show the result the script read, then the script and the values it keeps. Every
// byte shown comes from the pending-response query, never from the item payload.
const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => kubeResponsePayloadSchema.parse(props.item.payload));
// The item id is "kube.response:<requestId>"; the held result is fetched by request id.
const requestId = computed(() => props.item.id.slice(props.item.kind.length + 1));
const response = usePendingResponse(() => requestId.value);

const { single: rowHeight } = useRowHeight();
const firstLines = computed(() => ({ table: rowHeight.value, log: rowHeight.value }));

const scripted = computed(() => payload.value.scripted);
// What the agent gets on Send; with a script, the result the script read is a view of its own.
const text = computed(() => response.data.value?.text ?? '');
const script = computed(() => response.data.value?.script ?? null);
const keep = computed(() => response.data.value?.keep ?? []);
const read = computed(() => (scripted.value ? (response.data.value?.raw ?? '') : text.value));
// A failed script still leaves the tool's own result to show.
const failed = computed(() => payload.value.isError && !scripted.value);
const json = computed(() => (failed.value ? null : indentJson(read.value)));
const agentFailed = computed(() => payload.value.isError);
const agentJson = computed(() => (agentFailed.value ? null : indentJson(text.value)));

function parse<T>(schema: {
  safeParse: (v: unknown) => { success: true; data: T } | { success: false };
}): T | null {
  if (failed.value || !read.value) return null;
  try {
    const r = schema.safeParse(JSON.parse(read.value));
    return r.success ? r.data : null;
  } catch {
    return null;
  }
}
const list = computed(() =>
  payload.value.operation === 'list' ? parse(kubeListResultSchema) : null,
);
const described = computed(() =>
  payload.value.operation === 'describe' ? parse(kubeDescribeResultSchema) : null,
);
const logs = computed(() =>
  payload.value.operation === 'logs' ? parse(kubeLogsResultSchema) : null,
);

// Never hide bytes: a result that does not parse falls back to its verbatim JSON view.
const first = computed(() => {
  if (list.value) return { value: 'table', label: t('studio.definition.table.label') };
  if (described.value)
    return { value: 'manifest', label: t('kubernetes.describe.section.manifest.label') };
  if (logs.value) return { value: 'log', label: t('common.tab.logs.label') };
  return null;
});
const views = computed(() => [
  ...(scripted.value
    ? [{ value: 'result', label: t('common.agentGets.label'), testid: 'kube-review-view-result' }]
    : []),
  ...(first.value ? [{ ...first.value, testid: `kube-review-view-${first.value.value}` }] : []),
  { value: 'json', label: t('common.copyFormat.json.label'), testid: 'kube-review-view-json' },
]);

const footnote = computed(() => {
  if (list.value?.truncated) return t('studio.dbMcp.preview.truncated.status');
  if (logs.value?.truncated) return t('studio.dbMcp.approval.kube.logsTruncated.status');
  if (described.value?.manifestTruncated)
    return t('studio.dbMcp.approval.kube.manifestTruncated.status');
  return '';
});
// The service redacts a Secret's values before the manifest leaves it; the user is told why they are absent.
const secretNote = computed(() => described.value?.kind === 'Secret');
</script>

<template>
  <div data-testid="kube-review-body">
    <QueryState :query="response" variant="inline" :is-empty="() => false">
      <FieldError v-if="failed" class="whitespace-pre-wrap" data-testid="kube-review-error">{{
        text
      }}</FieldError>
      <ResultReview
        v-else
        :views="views"
        :first-lines="firstLines"
        :label="t('studio.dbMcp.approval.query.responseViews.aria')"
      >
        <template #view-result>
          <FieldError
            v-if="agentFailed"
            class="whitespace-pre-wrap"
            data-testid="kube-review-result"
            >{{
              text
            }}</FieldError
          >
          <FlushEditor
            v-else
            :doc="agentJson ?? text"
            language="json"
            data-testid="kube-review-result"
          />
        </template>
        <template #view-table>
          <ReviewTable
            v-if="list"
            :columns="list.columns.map((label) => ({ label }))"
            :rows="list.rows.map((r) => r.cells)"
            testid="kube-review"
          />
        </template>
        <template #view-manifest>
          <FlushEditor
            v-if="described"
            :doc="described.manifest"
            language="yaml"
            data-testid="kube-review-manifest"
          />
        </template>
        <template #view-log>
          <ReviewLog v-if="logs" :lines="logs.lines" testid="kube-review-log" />
        </template>
        <template #view-json>
          <FlushEditor :doc="json ?? read" language="json" data-testid="kube-review-json" />
        </template>
        <template #script>
          <JsScriptBlock
            v-if="script"
            :label="t('studio.dbMcp.approval.api.scriptTransform.label')"
            :source="script.source"
            testid="kube-review-script"
          />
        </template>
        <template #after>
          <StagedKeepList
            v-if="keep.length"
            :keep="keep"
            :request-id="requestId"
            testid="kube-review-keep"
          />
        </template>
        <template #mask>
          <span v-if="secretNote" class="text-kira-sm text-subtle" data-testid="kube-review-mask">{{
            t('studio.dbMcp.approval.kube.secretRedacted.hint')
          }}</span>
          <span
            v-if="footnote"
            class="text-kira-sm text-subtle"
            data-testid="kube-review-summary"
            >{{
              footnote
            }}</span
          >
        </template>
      </ResultReview>
    </QueryState>
  </div>
</template>
