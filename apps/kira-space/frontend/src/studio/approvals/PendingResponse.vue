<script setup lang="ts">
import { dbMcpRunQueryResultSchema } from '@shared/domain/dbmcp';
import { useQuery } from '@tanstack/vue-query';
import type { SecondaryTabItem } from '@theme/components/SecondaryTabs.vue';
import { FieldError } from '@theme/components/ui/field';
import { type MessageKey, t } from '@theme/i18n';
import QueryState from '@workbench/components/QueryState.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { computed, ref } from 'vue';
import { loadMaskRules, maskRulesQueryKey } from '../state/maskRules';
import FlushEditor from './FlushEditor.vue';
import { usePendingResponse } from './pendingResponse';
import ReviewTable from './ReviewTable.vue';
import TabbedContent from './TabbedContent.vue';

// The held run_query result of a response-stage approval: the table is a view of the exact bytes
// the AI client receives on Send. Those bytes are shown on the Raw tab, and nothing here
// re-renders or re-masks them. `field` picks the held text: the exact result, or with a script the raw
// result it read.
const props = withDefaults(
  defineProps<{ requestId: string; connectionId: string; field?: 'text' | 'raw' }>(),
  { field: 'text' },
);

const response = usePendingResponse(() => props.requestId);
const maskRules = useQuery({
  queryKey: computed(() => maskRulesQueryKey(props.connectionId)),
  queryFn: () => loadMaskRules(props.connectionId),
});
const maskedColumns = computed(
  () => new Set((maskRules.data.value ?? []).map((r) => r.columnName.toLowerCase())),
);

const text = computed(() => response.data.value?.[props.field] ?? '');
// A script that failed leaves the raw result readable; only the sent text is the error.
const isError = computed(() => props.field === 'text' && response.data.value?.isError === true);

const parsed = computed(() => {
  if (!response.data.value || isError.value) return null;
  try {
    const result = dbMcpRunQueryResultSchema.safeParse(JSON.parse(text.value));
    return result.success ? result.data : null;
  } catch {
    return null;
  }
});

interface Column {
  label: string;
  title?: string;
  masked: boolean;
}
type Cell = string | null;

const grid = computed<{ columns: Column[]; rows: Cell[][] } | null>(() => {
  const r = parsed.value;
  if (!r) return null;
  const plain = (key: MessageKey): Column => ({ label: t(key), masked: false });
  switch (r.kind) {
    case 'empty':
      return null;
    case 'tabular':
      return {
        columns: r.columns.map((c) => ({
          label: c.name,
          title: c.dataType,
          masked: maskedColumns.value.has(c.name.toLowerCase()),
        })),
        rows: r.rows,
      };
    case 'document':
      return {
        columns: [plain('common.field.id.label'), plain('common.body.label')],
        rows: r.documents.map((d) => [d.id, d.body]),
      };
    case 'keyvalue':
      return {
        columns: [plain('studio.keyValue.column.field.label'), plain('common.field.value.label')],
        rows: r.entries.map((e) => [e.field, e.value]),
      };
    case 'stream':
      return {
        columns: [
          plain('common.field.key.label'),
          plain('studio.stream.column.timestamp.label'),
          plain('studio.http.pane.headers.label'),
          plain('studio.stream.column.attributes.label'),
          plain('common.body.label'),
        ],
        rows: r.messages.map((m) => [m.key, m.timestamp, m.headers, m.attrs, m.body]),
      };
  }
});

const view = ref<'table' | 'asSent'>('table');
const items = computed<SecondaryTabItem[]>(() => [
  {
    value: 'table',
    label: t('studio.definition.table.label'),
    testid: 'db-mcp-preview-view-table',
  },
  {
    value: 'asSent',
    label: t('common.raw.label'),
    testid: 'db-mcp-preview-view-as-sent',
  },
]);
// Never hide the bytes: an unparseable non-error result falls back to Raw alone.
const showTable = computed(() => view.value === 'table' && grid.value !== null);
const unreadable = computed(() => response.data.value && !isError.value && parsed.value === null);

// What sits under the table: truncation, shortened cells, Redis type and TTL, the engine's note.
const footnote = computed(() => {
  const r = parsed.value;
  if (!r) return '';
  if (r.kind === 'empty')
    return [t('studio.common.noRows.empty'), r.note].filter(Boolean).join(' · ');
  const parts: string[] = [];
  if (r.truncated) parts.push(t('studio.dbMcp.preview.truncated.status'));
  if (r.kind === 'tabular' && (r.truncatedCells ?? 0) > 0) {
    parts.push(t('studio.dbMcp.preview.truncatedCells.status', { count: r.truncatedCells ?? 0 }));
  }
  if (r.kind === 'keyvalue') {
    parts.push(t('studio.dbMcp.preview.redisType.status', { type: r.redisType }));
    if (r.ttlMs !== undefined) parts.push(t('studio.dbMcp.preview.ttl.status', { ms: r.ttlMs }));
  }
  if (r.note) parts.push(r.note);
  return parts.join(' · ');
});
const maskedNames = computed(() =>
  (grid.value?.columns ?? []).filter((c) => c.masked).map((c) => c.label),
);
const { single: rowHeight } = useRowHeight();
</script>

<template>
  <div data-testid="db-mcp-preview">
    <QueryState :query="response" variant="inline" :is-empty="() => false">
      <FieldError v-if="isError" class="whitespace-pre-wrap" data-testid="db-mcp-preview-error">{{
        text
      }}</FieldError>
      <TabbedContent
        v-else
        v-model="view"
        :tabs="grid !== null"
        :first-line="showTable ? rowHeight : undefined"
        :items="items"
        :label="t('studio.dbMcp.preview.view.aria')"
      >
        <div class="flex flex-col gap-1.5">
          <span v-if="unreadable" class="text-kira-sm text-subtle">{{
            t('studio.dbMcp.preview.unreadable.hint')
          }}</span>
          <ReviewTable
            v-if="showTable && grid"
            :columns="grid.columns"
            :rows="grid.rows"
            :mask-hint="t('studio.dbMcp.preview.masked.hint')"
            testid="db-mcp-preview"
          />
          <FlushEditor v-else :doc="text" language="json" data-testid="db-mcp-preview-raw" />
          <span
            v-if="footnote"
            class="text-kira-sm text-subtle"
            data-testid="db-mcp-preview-summary"
            >{{
              footnote
            }}</span
          >
          <span
            v-if="maskedNames.length"
            class="text-kira-sm text-subtle"
            data-testid="db-mcp-preview-mask"
            >{{
              t('studio.dbMcp.preview.maskedColumns.status', { names: maskedNames.join(', ') })
            }}</span
          >
        </div>
      </TabbedContent>
    </QueryState>
  </div>
</template>
