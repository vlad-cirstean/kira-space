<script setup lang="ts">
import { dbMcpRunQueryResultSchema } from '@shared/domain/dbmcp';
import { useQuery } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import type { SecondaryTabItem } from '@theme/components/SecondaryTabs.vue';
import TooltipDetails from '@theme/components/TooltipDetails.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Button } from '@theme/components/ui/button';
import { Collapsible, CollapsibleTrigger } from '@theme/components/ui/collapsible';
import { FieldError } from '@theme/components/ui/field';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@theme/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { type MessageKey, t } from '@theme/i18n';
import { useVirtualRows } from '@workbench/behaviour/virtualRows';
import QueryState from '@workbench/components/QueryState.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { computed, ref } from 'vue';
import { control } from '../bridge/control';
import { loadMaskRules, maskRulesQueryKey } from '../state/maskRules';
import FlushEditor from './FlushEditor.vue';
import TabbedContent from './TabbedContent.vue';

// The held run_query result of a response-stage approval: the table is a view of the exact bytes
// the AI client receives on Send. Those bytes are shown on the As sent tab, and nothing here
// re-renders or re-masks them.
const props = defineProps<{ requestId: string; connectionId: string }>();

const response = useQuery({
  queryKey: computed(() => ['dbMcpPendingResponse', props.requestId] as const),
  queryFn: () => control.dbMcpPendingResponse(props.requestId),
  gcTime: 0, // MB-sized text, immutable per request id: drop on close
});
const maskRules = useQuery({
  queryKey: computed(() => maskRulesQueryKey(props.connectionId)),
  queryFn: () => loadMaskRules(props.connectionId),
});
const maskedColumns = computed(
  () => new Set((maskRules.data.value ?? []).map((r) => r.columnName.toLowerCase())),
);

const text = computed(() => response.data.value?.text ?? '');
const isError = computed(() => response.data.value?.isError === true);

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
        columns: [plain('docker.resource.id.label'), plain('common.body.label')],
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
    label: t('studio.dbMcp.approval.asSent.option'),
    testid: 'db-mcp-preview-view-as-sent',
  },
]);
// Never hide the bytes: an unparseable non-error result falls back to As sent alone.
const showTable = computed(() => view.value === 'table' && grid.value !== null);
const unreadable = computed(() => response.data.value && !isError.value && parsed.value === null);

// The rows below the first few hide behind "+N more rows"; open shows all of them, virtualized.
const PLAIN_ROWS = 3;
const open = ref(false);
const hiddenRows = computed(() => Math.max(0, (grid.value?.rows.length ?? 0) - PLAIN_ROWS));
// What sits under the table: truncation, shortened cells, Redis type and TTL, the engine's note.
const footnote = computed(() => {
  const r = parsed.value;
  if (!r) return '';
  if (r.kind === 'empty')
    return [t('studio.common.noRows.empty'), r.note].filter(Boolean).join(' · ');
  const parts: string[] = [];
  if (r.truncated) parts.push(t('studio.dbMcp.preview.truncated.status'));
  if (r.kind === 'tabular' && r.truncatedCells > 0) {
    parts.push(t('studio.dbMcp.preview.truncatedCells.status', { count: r.truncatedCells }));
  }
  if (r.kind === 'keyvalue') {
    parts.push(t('studio.dbMcp.preview.redisType.status', { type: r.redisType }));
    if (r.ttlMs !== undefined) parts.push(t('studio.dbMcp.preview.ttl.status', { ms: r.ttlMs }));
  }
  if (r.note) parts.push(r.note);
  return parts.join(' · ');
});

const { single: rowHeight } = useRowHeight();
const scrollRef = ref<HTMLElement | null>(null);
const {
  virtualItems: virtualRows,
  totalSize,
  onScroll,
} = useVirtualRows({
  count: () => (open.value ? (grid.value?.rows.length ?? 0) : 0),
  rowHeight: () => rowHeight.value,
  scrollElement: scrollRef,
  overscan: 8,
});
const shownRows = computed(() =>
  open.value
    ? virtualRows.value.map((vr) => vr.index)
    : Array.from({ length: Math.min(PLAIN_ROWS, grid.value?.rows.length ?? 0) }, (_, i) => i),
);
const padTop = computed(() => virtualRows.value[0]?.start ?? 0);
const padBottom = computed(() => {
  const last = virtualRows.value[virtualRows.value.length - 1];
  return last ? totalSize.value - last.end : 0;
});
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
          <Collapsible
            v-if="showTable && grid"
            v-model:open="open"
            class="flex flex-col gap-1.5"
            data-testid="db-mcp-preview-table"
          >
            <div
              ref="scrollRef"
              :class="open ? 'max-h-72 overflow-auto' : 'overflow-hidden'"
              @scroll.passive="onScroll"
            >
              <Table class="w-auto">
                <TableHeader class="static bg-transparent">
                  <TableRow>
                    <TableHead
                      v-for="(col, c) in grid.columns"
                      :key="c"
                      class="first:pl-0"
                      data-testid="db-mcp-preview-column"
                    >
                      <Tooltip :disabled="!col.title && !col.masked">
                        <TooltipTrigger as-child>
                          <span class="inline-flex items-center gap-1">
                            {{ col.label }}
                            <CodiconIcon v-if="col.masked" name="lock" :size="12" />
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>
                          <TooltipDetails
                            :title="col.label"
                            :rows="
                              col.title
                                ? [{ label: t('common.field.type.label'), value: col.title }]
                                : []
                            "
                            :hint="col.masked ? t('studio.dbMcp.preview.masked.hint') : undefined"
                          />
                        </TooltipContent>
                      </Tooltip>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow
                    v-if="padTop > 0"
                    aria-hidden="true"
                    class="border-0 hover:bg-transparent"
                    :style="{ height: `${padTop}px` }"
                  >
                    <TableCell :colspan="grid.columns.length" class="p-0" />
                  </TableRow>
                  <TableRow v-for="r in shownRows" :key="r" data-testid="db-mcp-preview-row">
                    <TableCell
                      v-for="(cell, c) in grid.rows[r]"
                      :key="c"
                      class="max-w-80 truncate font-data first:pl-0"
                      :data-null="cell === null ? '' : undefined"
                    >
                      <span v-if="cell === null" class="italic text-muted-foreground">{{
                        t('studio.common.nullValue.label')
                      }}</span>
                      <TruncatedText v-else :text="cell" />
                    </TableCell>
                  </TableRow>
                  <TableRow
                    v-if="padBottom > 0"
                    aria-hidden="true"
                    class="border-0 hover:bg-transparent"
                    :style="{ height: `${padBottom}px` }"
                  >
                    <TableCell :colspan="grid.columns.length" class="p-0" />
                  </TableRow>
                </TableBody>
              </Table>
            </div>
            <CollapsibleTrigger v-if="hiddenRows > 0 && !open" as-child>
              <Button
                variant="link"
                size="inline"
                class="self-start text-kira-sm font-normal text-muted-foreground underline decoration-border underline-offset-3"
                data-testid="db-mcp-preview-more"
              >
                {{ t('studio.dbMcp.preview.moreRows.label', { count: hiddenRows }) }}
              </Button>
            </CollapsibleTrigger>
          </Collapsible>
          <FlushEditor v-else :doc="text" language="json" data-testid="db-mcp-preview-raw" />
          <span
            v-if="footnote"
            class="text-kira-sm text-subtle"
            data-testid="db-mcp-preview-summary"
            >{{
              footnote
            }}</span
          >
        </div>
      </TabbedContent>
    </QueryState>
  </div>
</template>
