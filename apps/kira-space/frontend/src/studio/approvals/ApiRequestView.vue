<script setup lang="ts">
import type { ApiRequestPayload } from '@shared/domain/dbmcp';
import type { KeptStaged, KeptUse } from '@shared/domain/kept';
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
import { PREPARED_BOX, PREPARED_MARK, type PreparedValue, writtenRanges } from './preparedValues';
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
    /** Saved values the prepare script wrote: their tokens in the request carry `data-prepared`. */
    writes?: readonly KeptStaged[];
  }>(),
  { changed: () => [], prepared: () => [], writes: () => [] },
);

const written = (text: string) => writtenRanges(text, props.writes);
const withWritten = (text: string, ranges: ReturnType<typeof keptRanges>) =>
  [...ranges, ...written(text)].sort((a, b) => a.from - b.from);
// The write key of the first token a prepare script wrote in `text`.
const writtenKey = (text: string) => written(text)[0]?.key;
const urlKept = computed(() => withWritten(props.url, keptRanges(props.kept, 'url')));
const urlWrittenKey = computed(() => writtenKey(props.url));
const keptNames = computed(
  () => new Set([...props.kept.map((u) => u.name), ...props.writes.map((w) => w.name)]),
);
const json = computed(() => {
  const b = props.body;
  if (!b || b.form || !/json/i.test(b.contentType) || b.truncated) return null;
  return prettyKeptJson(b.text);
});
const bodyText = computed(() => props.body?.text ?? '');
const bodyParts = computed(() =>
  keptParts(bodyText.value, withWritten(bodyText.value, keptRanges(props.kept, 'body'))),
);
const bodyKey = computed(() => (isPrepared('body') ? 'body' : writtenKey(bodyText.value)));
const hasMarks = computed(() => props.kept.length > 0 || props.writes.length > 0);
const formParts = (row: number, part: 'name' | 'value') => {
  const text = props.body?.form?.[row]?.[part] ?? '';
  const ranges = withWritten(text, keptRanges(props.kept, 'form', row, part));
  return ranges.length ? keptParts(text, ranges) : null;
};
const headerParts = (row: number, part: 'name' | 'value') => {
  const text = props.headers[row]?.[part] ?? '';
  const ranges = withWritten(text, keptRanges(props.kept, 'header', row, part));
  return ranges.length ? keptParts(text, ranges) : null;
};
const isPrepared = (key: string) => props.changed.includes(key);
const pairWrittenKey = (
  pairs: readonly { name: string; value: string }[] | null | undefined,
  row: number,
) => writtenKey(`${pairs?.[row]?.name ?? ''} ${pairs?.[row]?.value ?? ''}`);
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
  if (row < props.headers.length) {
    return isPrepared('headers') ? 'headers' : pairWrittenKey(props.headers, row);
  }
  return isPrepared('contentType') ? 'contentType' : undefined;
};
</script>

<template>
  <div class="flex flex-col gap-1">
    <PreparedList v-if="prepared.length" :rows="prepared" testid="api-approval-prepared" />
    <RequestLine
      :method="method"
      :url="url"
      :kept="urlKept"
      :prepared="changed"
      :written-key="urlWrittenKey"
    />
    <template v-if="body">
      <ApprovalPairs
        v-if="body.form"
        :rows="body.form"
        :highlight="formParts"
        :mark-class="KEPT_MARK_CLASS"
        :prepared-of="(i) => (isPrepared('form') ? 'form' : pairWrittenKey(body?.form, i))"
        :prepared-class="PREPARED_MARK"
        density="approval"
        testid="api-approval-form"
      />
      <FlushEditor
        v-else-if="json !== null"
        :doc="json"
        language="json"
        :range-highlights="hasMarks ? keptEditorRanges(keptNames) : undefined"
        :hover-source="kept.length ? keptHover(kept) : undefined"
        data-testid="api-approval-body-text"
        :class="bodyKey && PREPARED_BOX"
        :data-prepared="bodyKey"
      />
      <ApprovalCode
        v-else
        :class="bodyKey && PREPARED_BOX"
        :data-prepared="bodyKey"
        data-testid="api-approval-body-text"
        ><HighlightedText v-if="hasMarks" :parts="bodyParts" :mark-class="KEPT_MARK_CLASS" />
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
