<script setup lang="ts">
import type { ConnectionFileMcp } from '@shared/domain/connectionConfig';
import { TABLE_LIST_KINDS } from '@shared/domain/dbmcp';
import { useQuery } from '@tanstack/vue-query';
import SearchField from '@theme/components/SearchField.vue';
import { Checkbox } from '@theme/components/ui/checkbox';
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { compareNatural, matchesQuery } from '@workbench/behaviour/match';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import QueryState from '@workbench/components/QueryState.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { computed, ref, useId } from 'vue';
import { control } from '../../bridge/control';

const connectionMcpTablesAllId = useId();

const props = defineProps<{
  mcp: ConnectionFileMcp;
  kind: string;
  enabled: boolean;
  connectionId?: string;
}>();
const emit = defineEmits<{ change: [change: Partial<ConnectionFileMcp>] }>();

// P532: the table allow list. Absent mcp.tables = all tables (future ones too); a list = exactly those.
const { single: tableRowHeight } = useRowHeight();
const TABLES_SHOWN_MAX = 5000;
const tablesApply = computed(() => TABLE_LIST_KINDS.has(props.kind));
const allTables = computed(() => props.mcp.tables === undefined);
const chosen = computed(() => new Set(props.mcp.tables ?? []));
const tableSearch = ref('');
const tableScroll = ref<HTMLElement | null>(null);

const tablesQuery = useQuery({
  queryKey: computed(() => ['dbmcp-tables', props.connectionId, props.kind]),
  queryFn: () => control.dbMcpTables(props.connectionId ?? ''),
  enabled: computed(() => tablesApply.value && !allTables.value && !!props.connectionId),
});

interface TableRow {
  ref: string;
  stale: boolean;
}
// Listed entries the server no longer shows stay visible, so they can be removed.
const tableRows = computed<TableRow[]>(() => {
  const live = (tablesQuery.data.value?.tables ?? []).map((x) => x.ref);
  const liveSet = new Set(live);
  const stale = [...chosen.value].filter((r) => !liveSet.has(r)).sort(compareNatural);
  return [
    ...stale.map((ref) => ({ ref, stale: true })),
    ...live.map((ref) => ({ ref, stale: false })),
  ].filter((r) => matchesQuery(r.ref, tableSearch.value));
});
const { virtualItems: tableItems, totalSize: tableTotal } = useVirtualRows({
  count: () => tableRows.value.length,
  rowHeight: () => tableRowHeight.value,
  scrollElement: tableScroll,
});
const shownChosen = computed(() => tableRows.value.filter((r) => chosen.value.has(r.ref)).length);
const selectAllState = computed<boolean | 'indeterminate'>(() => {
  if (tableRows.value.length === 0 || shownChosen.value === 0) return false;
  return shownChosen.value === tableRows.value.length ? true : 'indeterminate';
});

function setAllTables(all: boolean): void {
  if (all) {
    emit('change', { tables: undefined });
    return;
  }
  emit('change', { tables: [] });
}
function toggleTable(ref: string, on: boolean): void {
  const next = new Set(chosen.value);
  if (on) next.add(ref);
  else next.delete(ref);
  emit('change', { tables: [...next].sort(compareNatural) });
}
function toggleShown(on: boolean): void {
  const next = new Set(chosen.value);
  for (const r of tableRows.value) {
    if (on) next.add(r.ref);
    else next.delete(r.ref);
  }
  emit('change', { tables: [...next].sort(compareNatural) });
}
</script>

<template>
  <FieldSet class="flex-1" data-testid="connection-mcp-tables">
    <FieldLegend>{{ t('studio.project.connectionDialog.mcp.tables.label') }}</FieldLegend>
    <FieldDescription class="w-full">{{
      t('studio.project.connectionDialog.mcp.tablesNote.label')
    }}</FieldDescription>
    <FieldDescription v-if="!tablesApply" class="w-full" data-testid="connection-mcp-tables-na">
      {{ t('studio.project.connectionDialog.mcp.tablesNa.body') }}
    </FieldDescription>
    <template v-else>
      <Field orientation="horizontal">
        <Checkbox
          :id="connectionMcpTablesAllId"
          :model-value="allTables"
          :disabled="!enabled"
          data-testid="connection-mcp-tables-all"
          @update:model-value="(v) => setAllTables(v === true)"
        />
        <FieldContent>
          <FieldLabel :for="connectionMcpTablesAllId">{{
            t('studio.project.connectionDialog.mcp.tablesAll.label')
          }}</FieldLabel>
          <FieldDescription>{{
            t('studio.project.connectionDialog.mcp.tablesAll.description')
          }}</FieldDescription>
        </FieldContent>
      </Field>
      <template v-if="!allTables">
        <FieldDescription
          v-if="!props.connectionId"
          class="w-full"
          data-testid="connection-mcp-tables-offline"
        >
          {{ t('studio.project.connectionDialog.mcp.tablesUnsaved.body') }}
        </FieldDescription>
        <QueryState
          v-else
          :query="tablesQuery"
          variant="inline"
          :is-empty="() => false"
          :loading-label="t('studio.project.connectionDialog.mcp.tablesLoading.label')"
          :error-label="t('studio.project.connectionDialog.mcp.tablesOffline.body')"
          error-testid="connection-mcp-tables-offline"
        >
          <div class="flex items-center gap-2">
            <Checkbox
              :model-value="selectAllState"
              :disabled="!enabled || tableRows.length === 0"
              :aria-label="t('studio.project.connectionDialog.mcp.tablesSelectAll.label')"
              data-testid="connection-mcp-tables-select-all"
              @update:model-value="(v) => toggleShown(v === true)"
            />
            <SearchField
              v-model="tableSearch"
              class="flex-1"
              :placeholder="t('common.action.filter.label')"
              data-testid="connection-mcp-tables-search"
            />
            <span class="text-kira-sm text-subtle" data-testid="connection-mcp-tables-count">{{
              t('studio.project.connectionDialog.mcp.tablesCount.label', {
                selected: String(chosen.size),
                total: String(tablesQuery.data.value?.tables.length ?? 0),
              })
            }}</span>
          </div>
          <div
            ref="tableScroll"
            class="max-h-48 overflow-auto border border-border rounded-kira-sm"
          >
            <div v-if="tableRows.length === 0" class="p-2 text-kira-sm text-subtle">
              {{ t('studio.project.connectionDialog.mcp.tablesEmpty.label') }}
            </div>
            <div v-else class="relative w-full" :style="{ height: `${tableTotal}px` }">
              <Label
                v-for="item in tableItems"
                :key="tableRows[item.index]?.ref"
                :class="['flex items-center gap-1.5 px-2 h-row', VIRTUAL_ROW_CLASS]"
                :style="{ transform: `translateY(${item.start}px)` }"
                data-testid="connection-mcp-table-row"
                :data-ref="tableRows[item.index]?.ref"
              >
                <Checkbox
                  :model-value="chosen.has(tableRows[item.index]?.ref ?? '')"
                  :disabled="!enabled"
                  @update:model-value="
                    (v) => toggleTable(tableRows[item.index]?.ref ?? '', v === true)
                  "
                />
                <span class="font-data truncate">{{ tableRows[item.index]?.ref }}</span>
                <span v-if="tableRows[item.index]?.stale" class="text-subtle text-kira-sm">{{
                  t('studio.project.connectionDialog.mcp.tablesStale.label')
                }}</span>
              </Label>
            </div>
          </div>
          <FieldDescription v-if="tablesQuery.data.value?.truncated" class="w-full">
            {{
              t('studio.project.connectionDialog.mcp.tablesTruncated.body', {
                max: String(TABLES_SHOWN_MAX),
              })
            }}
          </FieldDescription>
        </QueryState>
      </template>
      <FieldDescription class="w-full" data-testid="connection-mcp-tables-enforce">
        {{ t('studio.project.connectionDialog.mcp.tablesEnforceParse.body') }}
        {{ t('studio.project.connectionDialog.mcp.tablesEnforceScope.body') }}
        {{ t('studio.project.connectionDialog.mcp.tablesEnforceExempt.body') }}
      </FieldDescription>
    </template>
  </FieldSet>
</template>
