<script setup lang="ts">
import {
  CONNECTION_MCP_STATEMENT_TIMEOUT,
  EXPLAIN_SUPPORTED_KINDS,
} from '@shared/domain/connection';
import type { ConnectionFileMcp, ConnectionFileModel } from '@shared/domain/connectionConfig';
import {
  accessSummaryOf,
  enforcementOf,
  expensiveQueryCheckOf,
  hasWriteConsole,
  statementTimeoutEnforcementOf,
  TABLE_LIST_KINDS,
} from '@shared/domain/dbmcp';
import { useQuery } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { Textarea } from '@theme/components/ui/textarea';
import { type MessageKey, t } from '@theme/i18n';
import NumberStepperInput from '@theme/NumberStepperInput.vue';
import { wrapSelectionOnType } from '@theme/wrapSelection';
import { formatCount } from '@workbench/behaviour/format';
import { compareNatural, matchesQuery } from '@workbench/behaviour/match';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import QueryState from '@workbench/components/QueryState.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { computed, ref } from 'vue';
import { control } from '../../bridge/control';
import { useSettingsStore } from '../../state/settings';
import CheckboxRow from './CheckboxRow.vue';

// What an AI client may do through the database MCP server. A key left out takes its default.
const props = defineProps<{
  errorFor: (pointer: string) => string | undefined;
  connectionId?: string;
}>();
const file = defineModel<ConnectionFileModel>({ required: true });
const settingsStore = useSettingsStore();

const mcp = computed(() => file.value.mcp ?? {});
const enabled = computed(() => mcp.value.enabled ?? false);
function setMcp(change: Partial<ConnectionFileMcp>): void {
  file.value.mcp = { ...file.value.mcp, ...change };
}

// Disabled, never hidden, for kinds without a parseable EXPLAIN: the setting stays visible.
const explainSupported = computed(() => EXPLAIN_SUPPORTED_KINDS.has(file.value.kind as never));

// Mirrors internal/dbmcp's expensiveCheckOf: which row-check note applies to this kind and switch.
const rowCheck = computed(() =>
  expensiveQueryCheckOf(file.value.kind, mcp.value.auto_explain ?? true),
);

// Mirrors internal/dbmcp's accessOf: what each declared run_query mode allows for this connection.
const writeConsole = computed(() => hasWriteConsole(file.value.kind));
const enforcement = computed(() => enforcementOf(file.value.kind));
const ENFORCEMENT_KEYS: Record<string, MessageKey> = {
  session: 'studio.dbMcp.approval.enforcement.session.body',
  app: 'studio.dbMcp.approval.enforcement.app.body',
  none: 'studio.dbMcp.approval.enforcement.none.body',
};
const accessSummary = computed(() =>
  enabled.value
    ? accessSummaryOf({
        kind: file.value.kind,
        readOnly: file.value.read_only ?? false,
        read: mcp.value.read ?? 'allow',
        write: mcp.value.write ?? 'prompt',
        ddl: mcp.value.ddl ?? 'deny',
      })
    : 'none',
);
const writeOrDdlOpen = computed(
  () => (mcp.value.write ?? 'prompt') !== 'deny' || (mcp.value.ddl ?? 'deny') !== 'deny',
);
// The known-limit note under each class: reads name their lane, write names the DDL blind spot.
function classNote(cls: 'read' | 'write' | 'ddl'): { name: string; key: MessageKey } | undefined {
  if (cls === 'read' && enforcement.value === 'app')
    return { name: 'limitApp', key: 'studio.project.connectionDialog.mcp.limitApp.body' };
  if (cls === 'read' && enforcement.value === 'session')
    return { name: 'readLane', key: 'studio.project.connectionDialog.mcp.readLane.body' };
  if (cls === 'write' && writeConsole.value && writeOrDdlOpen.value)
    return { name: 'limitDdl', key: 'studio.project.connectionDialog.mcp.limitDdl.body' };
  return undefined;
}

// P532: the table allow list. Absent mcp.tables = all tables (future ones too); a list = exactly those.
const { single: tableRowHeight } = useRowHeight();
const TABLES_SHOWN_MAX = 5000;
const tablesApply = computed(() => TABLE_LIST_KINDS.has(file.value.kind));
const allTables = computed(() => mcp.value.tables === undefined);
const chosen = computed(() => new Set(mcp.value.tables ?? []));
const tableSearch = ref('');
const tableScroll = ref<HTMLElement | null>(null);

const tablesQuery = useQuery({
  queryKey: computed(() => ['dbmcp-tables', props.connectionId, file.value.kind]),
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
    const { tables: _drop, ...rest } = mcp.value;
    file.value.mcp = rest;
    return;
  }
  setMcp({ tables: (tablesQuery.data.value?.tables ?? []).map((x) => x.ref) });
}
function toggleTable(ref: string, on: boolean): void {
  const next = new Set(chosen.value);
  if (on) next.add(ref);
  else next.delete(ref);
  setMcp({ tables: [...next].sort(compareNatural) });
}
function toggleShown(on: boolean): void {
  const next = new Set(chosen.value);
  for (const r of tableRows.value) {
    if (on) next.add(r.ref);
    else next.delete(r.ref);
  }
  setMcp({ tables: [...next].sort(compareNatural) });
}

const CLASSES = [
  {
    cls: 'read',
    label: 'studio.project.connectionDialog.mcp.read.label',
    note: 'studio.project.connectionDialog.mcp.readNote.label',
    fallback: 'allow',
  },
  {
    cls: 'write',
    label: 'studio.project.connectionDialog.mcp.write.label',
    note: 'studio.project.connectionDialog.mcp.writeNote.label',
    fallback: 'prompt',
  },
  {
    cls: 'ddl',
    label: 'studio.project.connectionDialog.mcp.ddl.label',
    note: 'studio.project.connectionDialog.mcp.ddlNote.label',
    fallback: 'deny',
  },
] as const;

// One segmented control per class; per-button testids are connection-mcp-<class>-<mode>.
function modeOptions(cls: (typeof CLASSES)[number]['cls']) {
  const modes = [
    { value: 'deny', label: t('common.action.deny.label') },
    { value: 'allow', label: t('studio.project.connectionDialog.mcp.allow.label') },
    { value: 'prompt', label: t('studio.project.connectionDialog.mcp.prompt.label') },
  ];
  return modes.map((m) => ({
    ...m,
    testid: `connection-mcp-${cls}-${m.value}`,
    disabled: !enabled.value || (cls !== 'read' && !writeConsole.value),
  }));
}

// Inherit's label shows the current app setting.
const reviewOptions = computed(() => {
  const items = [
    {
      value: 'inherit',
      label: t(
        settingsStore.dbMcp.previewResponses
          ? 'studio.project.connectionDialog.mcp.reviewInheritOn.option'
          : 'studio.project.connectionDialog.mcp.reviewInheritOff.option',
      ),
    },
    { value: 'review', label: t('studio.project.connectionDialog.mcp.reviewAlways.option') },
    { value: 'send', label: t('studio.project.connectionDialog.mcp.reviewNever.option') },
  ];
  return items.map((i) => ({
    ...i,
    testid: `connection-mcp-review-${i.value}`,
    disabled: !enabled.value,
  }));
});

const throttleLead = computed(() => {
  const perSec = file.value.throttle_per_sec ?? 0;
  const tab = t('common.advanced.label');
  return perSec > 0
    ? t('studio.project.connectionDialog.mcp.throttle.body', { rate: String(perSec), tab })
    : t('studio.project.connectionDialog.mcp.throttleOff.body', { tab });
});

// Mirrors statementTimeoutEnforcementOf: who stops a statement past the timeout, per mode.
const timeoutValue = computed(
  () => mcp.value.statement_timeout_sec ?? CONNECTION_MCP_STATEMENT_TIMEOUT.default,
);
function setTimeoutSec(value: string): void {
  const n = Number.parseFloat(value);
  setMcp({
    statement_timeout_sec: Number.isNaN(n) ? CONNECTION_MCP_STATEMENT_TIMEOUT.default : n,
  });
}
const timeoutError = computed(() => {
  const v = timeoutValue.value;
  if (!Number.isInteger(v) || v < 0 || v > CONNECTION_MCP_STATEMENT_TIMEOUT.max) {
    return t('studio.project.connectionDialog.mcp.timeoutRange.status', {
      max: CONNECTION_MCP_STATEMENT_TIMEOUT.max,
    });
  }
  return props.errorFor('/mcp/statement_timeout_sec');
});
const timeoutEnforcement = computed(() => {
  if (timeoutValue.value === 0) return t('studio.project.connectionDialog.mcp.timeoutOff.body');
  const by = statementTimeoutEnforcementOf(file.value.kind);
  const modes = ['read', 'write', 'ddl'] as const;
  const engine = modes.filter((m) => by[m] === 'session');
  const app = modes.filter((m) => by[m] === 'app');
  if (engine.length === 0 && app.length === 0)
    return t('studio.project.connectionDialog.mcp.timeoutNone.body');
  if (app.length === 0) return t('studio.project.connectionDialog.mcp.timeoutByEngine.body');
  if (engine.length === 0) return t('studio.project.connectionDialog.mcp.timeoutByApp.body');
  return t('studio.project.connectionDialog.mcp.timeoutSplit.body', {
    engine: engine.join(', '),
    app: app.join(', '),
  });
});
const timeoutThrottled = computed(
  () =>
    (file.value.throttle_per_sec ?? 0) > 0 &&
    (timeoutValue.value === 0 || timeoutValue.value >= 28),
);
</script>

<template>
  <div class="flex flex-col gap-3 p-3" data-testid="connection-section-mcp">
    <CheckboxRow
      :model-value="enabled"
      :label="t('studio.project.connectionDialog.mcp.expose.label')"
      :description="t('studio.project.connectionDialog.mcp.expose.description')"
      testid="connection-mcp-enabled"
      @update:model-value="(v) => setMcp({ enabled: v })"
    />

    <Field class="flex-1">
      <FieldLabel
        >{{ t('common.field.description.label') }}
        <span class="text-subtle"
          >— {{ t('studio.project.connectionDialog.mcp.descriptionNote.label') }}</span
        ></FieldLabel
      >
      <Textarea
        :model-value="mcp.description ?? ''"
        :aria-invalid="props.errorFor('/mcp/description') ? true : undefined"
        class="font-data"
        rows="2"
        maxlength="1000"
        :disabled="!enabled"
        data-testid="connection-mcp-description"
        @update:model-value="(v) => setMcp({ description: String(v) })"
        @keydown="wrapSelectionOnType"
      />
      <FieldError v-if="props.errorFor('/mcp/description')">{{
        props.errorFor('/mcp/description')
      }}</FieldError>
    </Field>

    <CheckboxRow
      :model-value="mcp.auto_explain ?? true"
      :disabled="!enabled || !explainSupported"
      :label="t('studio.project.connectionDialog.mcp.plan.label')"
      testid="connection-mcp-auto-explain"
      @update:model-value="(v) => setMcp({ auto_explain: v })"
    />
    <template v-if="enabled">
      <template v-if="rowCheck === 'plan'">
        <FieldDescription class="w-full" data-testid="connection-mcp-rows-threshold">
          {{ t('studio.settings.advanced.expensiveRows.description') }}
        </FieldDescription>
        <FieldDescription class="w-full" data-testid="connection-mcp-rows-who">
          {{
            t('studio.project.connectionDialog.mcp.rowsWho.body', {
              threshold: formatCount(settingsStore.advanced.expensiveQueryRows),
            })
          }}
        </FieldDescription>
        <FieldDescription class="w-full" data-testid="connection-mcp-rows-hit">
          {{ t('studio.project.connectionDialog.mcp.rowsHit.body') }}
        </FieldDescription>
        <FieldDescription class="w-full" data-testid="connection-mcp-rows-limits">
          {{ t('studio.project.connectionDialog.mcp.rowsLimits.body') }}
        </FieldDescription>
      </template>
      <FieldDescription
        v-else-if="rowCheck === 'off'"
        class="w-full"
        data-testid="connection-mcp-rows-off"
      >
        {{ t('studio.project.connectionDialog.mcp.rowsOff.body') }}
      </FieldDescription>
      <FieldDescription v-else class="w-full" data-testid="connection-mcp-rows-none">
        {{ t('studio.project.connectionDialog.mcp.rowsNone.body') }}
      </FieldDescription>
    </template>

    <Field v-for="c in CLASSES" :key="c.cls" class="flex-1">
      <FieldLabel
        >{{ t(c.label) }}
        <span class="text-subtle">— {{ t(c.note) }}</span></FieldLabel
      >
      <SecondaryTabs
        variant="segmented"
        size="kira-lg"
        :model-value="mcp[c.cls] ?? c.fallback"
        :items="modeOptions(c.cls)"
        :data-testid="`connection-mcp-${c.cls}`"
        @update:model-value="(v) => setMcp({ [c.cls]: String(v) })"
      />
      <FieldDescription
        v-if="classNote(c.cls)"
        class="w-full"
        :data-testid="`connection-mcp-note-${classNote(c.cls)?.name}`"
      >
        {{ t(classNote(c.cls)?.key as MessageKey) }}
      </FieldDescription>
      <FieldDescription
        v-if="c.cls === 'ddl' && writeConsole"
        class="w-full"
        data-testid="connection-mcp-enforce-ddl"
      >
        {{ t('studio.project.connectionDialog.mcp.enforceDdl.body') }}
      </FieldDescription>
    </Field>
    <FieldDescription class="w-full" data-testid="connection-mcp-enforce-modes">
      {{ t('studio.project.connectionDialog.mcp.enforceModes.body') }}
    </FieldDescription>
    <FieldDescription class="w-full" data-testid="connection-mcp-access">
      <strong>{{ t('studio.project.connectionDialog.mcp.access.label') }}</strong>
      <span data-testid="connection-mcp-access-summary"> {{ accessSummary }}.</span>
      <span data-testid="connection-mcp-access-enforcement">
        {{ t(ENFORCEMENT_KEYS[enforcement] as MessageKey) }}</span
      >
    </FieldDescription>
    <FieldDescription
      v-if="!writeConsole"
      class="w-full"
      data-testid="connection-mcp-note-noWriteConsole"
    >
      {{ t('studio.project.connectionDialog.mcp.noWriteConsole.body') }}
    </FieldDescription>
    <FieldDescription class="w-full">
      {{
        t('studio.project.connectionDialog.mcp.strictest.description', {
          tab: t('common.advanced.label'),
        })
      }}
    </FieldDescription>
    <FieldDescription v-if="enabled" class="w-full" data-testid="connection-mcp-throttle">
      {{ throttleLead }}
      {{ t('studio.project.connectionDialog.throttle.description') }}
      <span data-testid="connection-mcp-enforce-throttle">{{
        t('studio.project.connectionDialog.mcp.enforceThrottle.body')
      }}</span>
    </FieldDescription>
    <template v-if="enabled">
      <FieldDescription class="w-full" data-testid="connection-mcp-enforce-exemption">
        {{ t('studio.project.connectionDialog.mcp.enforceExemption.body') }}
      </FieldDescription>
      <FieldDescription class="w-full" data-testid="connection-mcp-enforce-rows">
        {{ t('studio.project.connectionDialog.mcp.enforceRows.body') }}
      </FieldDescription>
      <FieldDescription class="w-full" data-testid="connection-mcp-enforce-masking">
        {{ t('studio.project.connectionDialog.mcp.enforceMasking.body') }}
      </FieldDescription>
    </template>

    <Field class="flex-1">
      <FieldLabel>{{ t('studio.project.connectionDialog.mcp.timeoutLabel.label') }}</FieldLabel>
      <div class="w-24">
        <NumberStepperInput
          :min="0"
          :max="CONNECTION_MCP_STATEMENT_TIMEOUT.max"
          step="1"
          :disabled="!enabled"
          :aria-invalid="timeoutError ? true : undefined"
          data-testid="connection-mcp-timeout"
          :model-value="String(timeoutValue)"
          @update:model-value="(v: string | number) => setTimeoutSec(String(v))"
        />
      </div>
      <FieldError v-if="timeoutError" data-testid="connection-mcp-timeout-error">
        {{ timeoutError }}
      </FieldError>
      <template v-else>
        <FieldDescription class="w-full" data-testid="connection-mcp-timeout-description">
          {{
            t('studio.project.connectionDialog.mcp.timeoutDescription.body', {
              default: CONNECTION_MCP_STATEMENT_TIMEOUT.default,
            })
          }}
        </FieldDescription>
        <FieldDescription class="w-full" data-testid="connection-mcp-timeout-enforcement">
          {{ timeoutEnforcement }}
          <span v-if="timeoutValue > 0">{{
            t('studio.project.connectionDialog.mcp.timeoutHit.body')
          }}</span>
        </FieldDescription>
        <FieldDescription
          v-if="timeoutThrottled"
          class="w-full"
          data-testid="connection-mcp-timeout-throttle"
        >
          {{ t('studio.project.connectionDialog.mcp.timeoutThrottle.body') }}
        </FieldDescription>
      </template>
    </Field>

    <Field class="flex-1" data-testid="connection-mcp-tables">
      <FieldLabel
        >{{ t('studio.project.connectionDialog.mcp.tables.label') }}
        <span class="text-subtle"
          >— {{ t('studio.project.connectionDialog.mcp.tablesNote.label') }}</span
        ></FieldLabel
      >
      <FieldDescription v-if="!tablesApply" class="w-full" data-testid="connection-mcp-tables-na">
        {{ t('studio.project.connectionDialog.mcp.tablesNa.body') }}
      </FieldDescription>
      <template v-else>
        <CheckboxRow
          :model-value="allTables"
          :disabled="!enabled"
          :label="t('studio.project.connectionDialog.mcp.tablesAll.label')"
          :description="t('studio.project.connectionDialog.mcp.tablesAll.description')"
          testid="connection-mcp-tables-all"
          @update:model-value="setAllTables"
        />
        <template v-if="!allTables">
          <FieldDescription
            v-if="!connectionId"
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
          >
            <template #error="{ retry }">
              <div
                class="flex items-center gap-2 text-kira-sm text-subtle"
                data-testid="connection-mcp-tables-offline"
              >
                <span>{{ t('studio.project.connectionDialog.mcp.tablesOffline.body') }}</span>
                <Button size="kira" variant="dialog" @click="retry">{{
                  t('common.action.retry.label')
                }}</Button>
              </div>
            </template>
            <div class="flex items-center gap-2">
              <Checkbox
                :model-value="selectAllState"
                :disabled="!enabled || tableRows.length === 0"
                class="size-3.5"
                :aria-label="t('studio.project.connectionDialog.mcp.tablesSelectAll.label')"
                data-testid="connection-mcp-tables-select-all"
                @update:model-value="(v) => toggleShown(v === true)"
              >
                <CodiconIcon :name="selectAllState === true ? 'check' : 'remove'" :size="12" />
              </Checkbox>
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
                    class="size-3.5"
                    @update:model-value="
                      (v) => toggleTable(tableRows[item.index]?.ref ?? '', v === true)
                    "
                  >
                    <CodiconIcon name="check" :size="12" />
                  </Checkbox>
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
          {{ t('studio.project.connectionDialog.mcp.tablesEnforceFail.body') }}
          {{ t('studio.project.connectionDialog.mcp.tablesEnforceScope.body') }}
          {{ t('studio.project.connectionDialog.mcp.tablesEnforceExempt.body') }}
        </FieldDescription>
      </template>
    </Field>

    <Field class="flex-1">
      <FieldLabel
        >{{ t('studio.project.connectionDialog.mcp.review.label') }}
        <span class="text-subtle"
          >— {{ t('studio.project.connectionDialog.mcp.reviewNote.label') }}</span
        ></FieldLabel
      >
      <SecondaryTabs
        variant="segmented"
        size="kira-lg"
        :model-value="mcp.review_results ?? 'inherit'"
        :items="reviewOptions"
        data-testid="connection-mcp-review"
        @update:model-value="(v) => setMcp({ review_results: String(v) })"
      />
      <FieldDescription class="w-full">{{
        t('studio.project.connectionDialog.mcp.review.description')
      }}</FieldDescription>
    </Field>
  </div>
</template>
