<script setup lang="ts">
import { dbMcpRunQueryResultSchema } from '@shared/domain/dbmcp';
import { useQuery } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs, { type SecondaryTabItem } from '@theme/components/SecondaryTabs.vue';
import TooltipDetails from '@theme/components/TooltipDetails.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { FieldDescription } from '@theme/components/ui/field';
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
import MonacoHost from '../editor/MonacoHost.vue';
import { loadMaskRules, maskRulesQueryKey } from '../state/maskRules';

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

const { single: rowHeight } = useRowHeight();
const scrollRef = ref<HTMLElement | null>(null);
const {
  virtualItems: virtualRows,
  totalSize,
  onScroll,
} = useVirtualRows({
  count: () => grid.value?.rows.length ?? 0,
  rowHeight: () => rowHeight.value,
  scrollElement: scrollRef,
  overscan: 8,
});
const padTop = computed(() => virtualRows.value[0]?.start ?? 0);
const padBottom = computed(() => {
  const last = virtualRows.value[virtualRows.value.length - 1];
  return last ? totalSize.value - last.end : 0;
});
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col gap-1.5" data-testid="db-mcp-preview">
    <QueryState :query="response" variant="inline" :is-empty="() => false">
      <Alert v-if="isError" variant="error" data-testid="db-mcp-preview-error">
        <AlertDescription class="whitespace-pre-wrap">{{ text }}</AlertDescription>
      </Alert>
      <template v-else>
        <SecondaryTabs
          v-if="grid"
          v-model="view"
          class="self-start"
          variant="segmented"
          :items="items"
          :aria-label="t('studio.dbMcp.preview.view.aria')"
        />
        <FieldDescription v-if="unreadable">{{
          t('studio.dbMcp.preview.unreadable.hint')
        }}</FieldDescription>
        <template v-if="parsed">
          <FieldDescription data-testid="db-mcp-preview-summary">
            <template v-if="parsed.kind === 'empty'">{{
              t('studio.common.noRows.empty')
            }}</template>
            <template v-else>
              {{
                t('studio.dbMcp.preview.summary.status', {
                  returned: parsed.returned,
                  count: parsed.rowCount,
                })
              }}
              <template v-if="parsed.truncated">
                · {{ t('studio.dbMcp.preview.truncated.status') }}</template
              >
              <template v-if="parsed.kind === 'tabular' && parsed.truncatedCells > 0">
                ·
                {{
                  t('studio.dbMcp.preview.truncatedCells.status', { count: parsed.truncatedCells })
                }}
              </template>
              <template v-if="parsed.kind === 'keyvalue'">
                · {{ t('studio.dbMcp.preview.redisType.status', { type: parsed.redisType }) }}
                <template v-if="parsed.ttlMs !== undefined">
                  · {{ t('studio.dbMcp.preview.ttl.status', { ms: parsed.ttlMs }) }}
                </template>
              </template>
            </template>
            <template v-if="parsed.note"> · {{ parsed.note }}</template>
          </FieldDescription>
        </template>
        <div
          v-if="showTable && grid"
          ref="scrollRef"
          class="min-h-24 flex-1 overflow-auto rounded-kira"
          data-testid="db-mcp-preview-table"
          @scroll.passive="onScroll"
        >
          <Table columns>
            <TableHeader>
              <TableRow>
                <TableHead
                  v-for="(col, c) in grid.columns"
                  :key="c"
                  class="sticky top-0 z-1 bg-elevated"
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
              <TableRow v-for="vr in virtualRows" :key="vr.index" data-testid="db-mcp-preview-row">
                <TableCell
                  v-for="(cell, c) in grid.rows[vr.index]"
                  :key="c"
                  class="max-w-80 truncate font-data"
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
        <div
          v-else
          class="min-h-24 flex-1 overflow-hidden rounded-kira"
          data-testid="db-mcp-preview-raw"
        >
          <MonacoHost :doc="text" language="json" readonly force-wrap />
        </div>
      </template>
    </QueryState>
  </div>
</template>
