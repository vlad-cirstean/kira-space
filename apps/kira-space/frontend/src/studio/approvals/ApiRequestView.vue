<script setup lang="ts">
import type { ApiRequestPayload } from '@shared/domain/dbmcp';
import type { KeptUse } from '@shared/domain/kept';
import HighlightedText from '@theme/components/HighlightedText.vue';
import { t } from '@theme/i18n';
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
  prettyKeptJson,
} from '../kept/keptHighlight';
import FlushEditor from './FlushEditor.vue';
import PreparedList from './PreparedList.vue';
import { PREPARED_BOX, PREPARED_MARK, type PreparedValue } from './preparedValues';
import RequestLine from './RequestLine.vue';

// The request as the agent sends it: line, body, headers and variables. Read-only, shared by the prompt and the
// review's Request section. Every place the request reads a kept variable is marked; `changed` and `prepared`
// mark what a prepare script set. A credential's value never reaches this view.
const props = withDefaults(
  defineProps<{
    method: string;
    url: string;
    headers: ApiRequestPayload['headers'];
    body: ApiRequestPayload['body'];
    variables: ApiRequestPayload['variables'];
    kept: readonly KeptUse[];
    changed?: readonly string[];
    prepared?: readonly PreparedValue[];
  }>(),
  { changed: () => [], prepared: () => [] },
);

const urlKept = computed(() => keptRanges(props.kept, 'url'));
const keptNames = computed(() => new Set(props.kept.map((u) => u.name)));
const json = computed(() => {
  const b = props.body;
  if (!b || b.form || !/json/i.test(b.contentType) || b.truncated) return null;
  return prettyKeptJson(b.text);
});
const bodyParts = computed(() => keptParts(props.body?.text ?? '', keptRanges(props.kept, 'body')));
const formParts = (row: number, part: 'name' | 'value') => {
  const text = props.body?.form?.[row]?.[part] ?? '';
  const ranges = keptRanges(props.kept, 'form', row, part);
  return ranges.length ? keptParts(text, ranges) : null;
};
const headerParts = (row: number, part: 'name' | 'value') => {
  const text = props.headers[row]?.[part] ?? '';
  const ranges = keptRanges(props.kept, 'header', row, part);
  return ranges.length ? keptParts(text, ranges) : null;
};
const isPrepared = (key: string) => props.changed.includes(key);
// Content-Type shows as a header row whenever the body names one the headers do not: it is sent either way.
const headerRows = computed(() => {
  const rows = props.headers.map((h) => ({ name: h.name, value: h.value }));
  const ct = props.body?.contentType;
  if (ct && !rows.some((r) => r.name.toLowerCase() === 'content-type')) {
    rows.push({ name: 'Content-Type', value: ct });
  }
  return rows;
});
const headerPrepared = (row: number) => {
  if (row < props.headers.length) return isPrepared('headers') ? 'headers' : undefined;
  return isPrepared('contentType') ? 'contentType' : undefined;
};
</script>

<template>
  <div class="flex flex-col gap-1">
    <PreparedList v-if="prepared.length" :rows="prepared" testid="api-approval-prepared" />
    <RequestLine :method="method" :url="url" :kept="urlKept" :prepared="changed" />
    <template v-if="body">
      <ApprovalPairs
        v-if="body.form"
        :rows="body.form"
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
        <template v-else>{{ body.text }}</template></ApprovalCode
      >
    </template>
    <ApprovalDetails v-if="headerRows.length || variables.length" class="py-1">
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
      <ApprovalDetailItem v-if="variables.length" :label="t('common.module.variables.label')">
        <ApprovalPairs
          :rows="variables.map((v) => ({ name: v.name, value: v.value }))"
          density="approval"
          testid="api-approval-variables"
        />
      </ApprovalDetailItem>
    </ApprovalDetails>
  </div>
</template>
