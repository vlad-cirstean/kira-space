<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { apiRequestPayloadSchema } from '@shared/domain/dbmcp';
import HighlightedText from '@theme/components/HighlightedText.vue';
import { t } from '@theme/i18n';
import ApprovalBox from '@workbench/approvals/ApprovalBox.vue';
import ApprovalCode from '@workbench/approvals/ApprovalCode.vue';
import ApprovalDetailItem from '@workbench/approvals/ApprovalDetailItem.vue';
import ApprovalDetails from '@workbench/approvals/ApprovalDetails.vue';
import ApprovalPairs from '@workbench/approvals/ApprovalPairs.vue';
import { computed } from 'vue';
import { keptHover } from '../kept/keptFormat';
import {
  KEPT_MARK_CLASS,
  keptEditorRanges,
  keptParts,
  keptRanges,
  keptScriptRanges,
  prettyKeptJson,
} from '../kept/keptHighlight';
import FlushEditor from './FlushEditor.vue';
import JsScriptBlock from './JsScriptBlock.vue';
import KeptVariablesPanel from './KeptVariablesPanel.vue';
import PreparedList from './PreparedList.vue';
import { apiPrepared, PREPARED_BOX, PREPARED_MARK } from './preparedValues';
import RequestLine from './RequestLine.vue';
import TransformScriptBlock from './TransformScriptBlock.vue';
import UserInputsPanel from './UserInputsPanel.vue';

// What the agent would send, one box per section: kept variables, the request (line, body, headers
// and variables), values to type, prepare script, transform script. A credential's value never
// reaches this view. Every place the request reads a kept variable is marked.
const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => apiRequestPayloadSchema.parse(props.item.payload));
// Every use marks its places in the request; a pending one (a value still to type) has no row to show.
const kept = computed(() => payload.value.kept);
const keptRows = computed(() => kept.value.filter((u) => !u.pending));

const prepared = computed(() => apiPrepared(payload.value));
const changed = computed(() => payload.value.script?.prepare?.changed ?? []);
const isPrepared = (key: string) => changed.value.includes(key);
// Content-Type shows as a header row whenever the body names one the headers do not: it is sent either way.
const headerRows = computed(() => {
  const rows = payload.value.headers.map((h) => ({ name: h.name, value: h.value }));
  const ct = payload.value.body?.contentType;
  if (ct && !rows.some((r) => r.name.toLowerCase() === 'content-type')) {
    rows.push({ name: 'Content-Type', value: ct });
  }
  return rows;
});
const headerPrepared = (row: number) => {
  if (row < payload.value.headers.length) return isPrepared('headers') ? 'headers' : undefined;
  return isPrepared('contentType') ? 'contentType' : undefined;
};

const urlKept = computed(() => keptRanges(kept.value, 'url'));

const keptNames = computed(() => new Set(kept.value.map((u) => u.name)));
const json = computed(() => {
  const b = payload.value.body;
  if (!b || b.form || !/json/i.test(b.contentType) || b.truncated) return null;
  return prettyKeptJson(b.text);
});
const bodyParts = computed(() =>
  keptParts(payload.value.body?.text ?? '', keptRanges(kept.value, 'body')),
);
const formParts = (row: number, part: 'name' | 'value') => {
  const text = payload.value.body?.form?.[row]?.[part] ?? '';
  const ranges = keptRanges(kept.value, 'form', row, part);
  return ranges.length ? keptParts(text, ranges) : null;
};
// The prepare script's own kept calls (reads); the transform block marks its own.
const keptSites = computed(() => kept.value.flatMap((u) => u.sites));
const headerParts = (row: number, part: 'name' | 'value') => {
  const text = payload.value.headers[row]?.[part] ?? '';
  const ranges = keptRanges(kept.value, 'header', row, part);
  return ranges.length ? keptParts(text, ranges) : null;
};
const pairName = (field: 'header' | 'query' | 'form', index: number): string => {
  if (field === 'header') return payload.value.headers[index]?.name ?? '';
  if (field === 'query') return payload.value.query[index]?.name ?? '';
  return payload.value.body?.form?.[index]?.name ?? '';
};
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="api-approval-body">
    <ApprovalBox v-if="keptRows.length" testid="api-approval-box-kept">
      <KeptVariablesPanel :uses="keptRows" :pair-name="pairName" />
    </ApprovalBox>
    <ApprovalBox testid="api-approval-box-request">
      <div class="flex flex-col gap-1">
        <PreparedList v-if="prepared.length" :rows="prepared" testid="api-approval-prepared" />
        <RequestLine
          :method="payload.method"
          :url="payload.url"
          :kept="urlKept"
          :prepared="changed"
        />
        <template v-if="payload.body">
          <ApprovalPairs
            v-if="payload.body.form"
            :rows="payload.body.form"
            :highlight="formParts"
            :mark-class="KEPT_MARK_CLASS"
            :prepared-of="() => (isPrepared('form') ? 'form' : undefined)"
            :prepared-class="PREPARED_MARK"
            density="approval"
            testid="api-approval-form"
          />
          <FlushEditor
            v-else-if="json !== null"
            :doc="json"
            language="json"
            :range-highlights="kept.length ? keptEditorRanges(keptNames) : undefined"
            :hover-source="kept.length ? keptHover(kept) : undefined"
            data-testid="api-approval-body-text"
            :class="isPrepared('body') && PREPARED_BOX"
            :data-prepared="isPrepared('body') ? 'body' : undefined"
          />
          <ApprovalCode
            v-else
            :class="isPrepared('body') && PREPARED_BOX"
            :data-prepared="isPrepared('body') ? 'body' : undefined"
            data-testid="api-approval-body-text"
            ><HighlightedText v-if="kept.length" :parts="bodyParts" :mark-class="KEPT_MARK_CLASS" />
            <template v-else>{{ payload.body.text }}</template></ApprovalCode
          >
        </template>
        <ApprovalDetails v-if="headerRows.length || payload.variables.length" class="py-1">
          <ApprovalDetailItem v-if="headerRows.length" :label="t('studio.http.pane.headers.label')">
            <ApprovalPairs
              :rows="headerRows"
              :highlight="headerParts"
              :mark-class="KEPT_MARK_CLASS"
              :prepared-of="headerPrepared"
              :prepared-class="PREPARED_MARK"
              density="approval"
              testid="api-approval-headers"
            />
          </ApprovalDetailItem>
          <ApprovalDetailItem
            v-if="payload.variables.length"
            :label="t('common.module.variables.label')"
          >
            <ApprovalPairs
              :rows="payload.variables.map((v) => ({ name: v.name, value: v.value }))"
              density="approval"
              testid="api-approval-variables"
            />
          </ApprovalDetailItem>
        </ApprovalDetails>
      </div>
    </ApprovalBox>
    <ApprovalBox v-if="payload.inputs.length" testid="api-approval-box-inputs">
      <UserInputsPanel :item-id="item.id" :inputs="payload.inputs" />
    </ApprovalBox>
    <ApprovalBox v-if="payload.script?.prepare" testid="api-approval-box-prepare">
      <JsScriptBlock
        :label="t('studio.dbMcp.approval.api.scriptPrepare.label')"
        :source="payload.script.prepare.source"
        :range-highlights="kept.length ? keptScriptRanges(keptSites, 'prepare') : undefined"
        testid="api-approval-script-prepare"
      />
    </ApprovalBox>
    <ApprovalBox v-if="payload.script?.transform" testid="api-approval-box-transform">
      <TransformScriptBlock
        :label="t('studio.dbMcp.approval.api.scriptTransform.label')"
        :script="payload.script.transform"
        :kept="kept"
        testid="api-approval"
        script-testid="api-approval-script-transform"
      />
    </ApprovalBox>
  </div>
</template>
