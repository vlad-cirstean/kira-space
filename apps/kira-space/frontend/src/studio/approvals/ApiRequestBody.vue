<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { apiRequestPayloadSchema } from '@shared/domain/dbmcp';
import HighlightedText from '@theme/components/HighlightedText.vue';
import { t } from '@theme/i18n';
import ApprovalCode from '@workbench/approvals/ApprovalCode.vue';
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
import TransformScriptBlock from './TransformScriptBlock.vue';
import UserInputsPanel from './UserInputsPanel.vue';

// What the agent would send: path and query, then the body. Host, headers and variables sit on
// line 1 and in the details. A credential's value never reaches this view. Kept variables list
// first, and every place the request reads one is marked.
const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => apiRequestPayloadSchema.parse(props.item.payload));
// Every use marks its places in the request; a pending one (a value still to type) has no row to show.
const kept = computed(() => payload.value.kept);
const keptRows = computed(() => kept.value.filter((u) => !u.pending));

// Sliced after the authority, not through `new URL()`: that would percent-encode the braces of a
// reference and move the offsets the sites carry.
const pathStart = computed(() => {
  const url = payload.value.url;
  const scheme = url.indexOf('://');
  const from = scheme < 0 ? 0 : scheme + 3;
  const end = url.slice(from).search(/[/?#]/);
  return end < 0 ? url.length : from + end;
});
const pathAndQuery = computed(() => {
  const rest = payload.value.url.slice(pathStart.value);
  const hash = rest.indexOf('#');
  return (hash < 0 ? rest : rest.slice(0, hash)) || '/';
});
const urlParts = computed(() =>
  keptParts(
    pathAndQuery.value,
    keptRanges(kept.value, 'url').map((r) => ({
      from: r.from - pathStart.value,
      to: r.to - pathStart.value,
    })),
  ),
);

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
const pairName = (field: 'header' | 'query' | 'form', index: number): string => {
  if (field === 'header') return payload.value.headers[index]?.name ?? '';
  if (field === 'query') return payload.value.query[index]?.name ?? '';
  return payload.value.body?.form?.[index]?.name ?? '';
};
</script>

<template>
  <div class="flex flex-col gap-1" data-testid="api-approval-body">
    <UserInputsPanel v-if="payload.inputs.length" :item-id="item.id" :inputs="payload.inputs" />
    <KeptVariablesPanel v-if="keptRows.length" :uses="keptRows" :pair-name="pairName" />
    <ApprovalCode data-testid="api-approval-url"
      ><HighlightedText v-if="kept.length" :parts="urlParts" :mark-class="KEPT_MARK_CLASS" />
      <template v-else>{{ pathAndQuery }}</template></ApprovalCode
    >
    <template v-if="payload.body">
      <ApprovalPairs
        v-if="payload.body.form"
        :rows="payload.body.form"
        :highlight="formParts"
        :mark-class="KEPT_MARK_CLASS"
        testid="api-approval-form"
      />
      <FlushEditor
        v-else-if="json !== null"
        :doc="json"
        language="json"
        :range-highlights="kept.length ? keptEditorRanges(keptNames) : undefined"
        :hover-source="kept.length ? keptHover(kept) : undefined"
        data-testid="api-approval-body-text"
      />
      <ApprovalCode v-else data-testid="api-approval-body-text"
        ><HighlightedText v-if="kept.length" :parts="bodyParts" :mark-class="KEPT_MARK_CLASS" />
        <template v-else>{{ payload.body.text }}</template></ApprovalCode
      >
    </template>
    <JsScriptBlock
      v-if="payload.script?.prepare"
      :label="t('studio.dbMcp.approval.api.scriptPrepare.label')"
      :source="payload.script.prepare.source"
      :range-highlights="kept.length ? keptScriptRanges(keptSites, 'prepare') : undefined"
      testid="api-approval-script-prepare"
    />
    <TransformScriptBlock
      v-if="payload.script?.transform"
      :label="t('studio.dbMcp.approval.api.scriptTransform.label')"
      :script="payload.script.transform"
      :kept="kept"
      testid="api-approval"
      script-testid="api-approval-script-transform"
    />
  </div>
</template>
