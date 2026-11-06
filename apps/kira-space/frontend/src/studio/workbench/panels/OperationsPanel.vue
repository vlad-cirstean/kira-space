<script setup lang="ts">
import type { EditorLanguageId } from '@shared/domain/editor';
import type { OpRecord } from '@shared/domain/ops';
import { splitSqlStatements } from '@shared/domain/sqlSplit';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { type AutomationOp, isAutomationOp } from '@workbench/automations/runs/runOps';
import { triggerLabel, useAutomationOps } from '@workbench/automations/runs/useAutomationOps';
import { displayValue, formatElapsed } from '@workbench/behaviour/format';
import { logError } from '@workbench/bridge/log';
import OpLogPanel from '@workbench/components/OpLogPanel.vue';
import {
  type OpLogColumn,
  type OpLogDetailLine,
  type OpLogField,
  opLogMenuItems,
} from '@workbench/lib/opLog';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed } from 'vue';
import { useConnectionsStore } from '../../state/connections';
import { useModeStore, workspaceKeyOf } from '../../state/mode';
import { useOpsStore } from '../../state/ops';
import { TAB_KINDS } from '../../state/tabKinds';
import { useTabsStore } from '../../state/tabs';
import { consoleLanguageFor } from '../../views/console/format';
import { useConsoleViewStore } from '../../views/console/state';
import { lexOptionsFor, sqlDialectFor } from '../../views/shared/sqlIdent';
import { ensureConnectedOnce } from '../../views/shared/useConnectionGate';

// The shared OpLogPanel over Kira Studio's op log: this file keeps the connection, source and rows
// columns, the detail fields and syntax language, the connection facet, and Reveal tab / Re-run.
const opsStore = useOpsStore();
const connectionsStore = useConnectionsStore();
const tabsStore = useTabsStore();
const modeStore = useModeStore();
const consoleViewStore = useConsoleViewStore();
const auto = useAutomationOps(opsStore);
const runningCount = computed(() => opsStore.runningCount + auto.runningCount.value);

const columns = computed<OpLogColumn[]>(() => [
  { id: 'status', label: t('common.field.status.label'), width: '44px' },
  { id: 'time', label: t('common.field.time.label'), width: '84px', priority: 3 },
  {
    id: 'connection',
    label: t('studio.common.nodeKind.connection.label'),
    width: '150px',
    priority: 1,
  },
  { id: 'source', label: t('common.field.source.label'), width: '56px', priority: 5 },
  { id: 'kind', label: t('common.field.kind.label'), width: '80px', priority: 4 },
  {
    id: 'duration',
    label: t('common.field.duration.label'),
    width: '64px',
    priority: 3,
    align: 'end',
  },
  { id: 'rows', label: t('studio.keyValue.rows.aria'), width: '56px', priority: 2, align: 'end' },
  { id: 'command', label: t('common.field.command.label'), width: '1fr' },
]);

const AUTOMATION_FACET = 'automation';
const showAutomation = computed(() => opsStore.facetAllows(AUTOMATION_FACET));
const records = computed(() =>
  auto.merge(opsStore.visibleOps).filter((r) => showAutomation.value || !isAutomationOp(r)),
);

function connectionFor(record: OpRecord) {
  return connectionsStore.connectionRecord(record.connectionId);
}

function onCancel(record: OpRecord | AutomationOp): void {
  if (isAutomationOp(record)) auto.cancel(record);
  else void opsStore.cancelOp(record.id);
}

// P2 D14/F7: TAB_KINDS[...].title — the registry P1 D4 made the per-kind source of truth —
// instead of tabs.ts's own tabTitle, which falls back to the raw path (F4) and would print an
// HTTP request tab's opaque constant 'request' path here.
function tabTitleFor(record: OpRecord): string {
  const tab = record.tabId ? tabsStore.tabs.find((t) => t.id === record.tabId) : undefined;
  return tab ? TAB_KINDS[tab.kind].title(tab) : '—';
}

// Reveal is right-click-only (the row's context menu) — a no-op when the tab has since been
// closed, never an error. A plain click just expands the row's command/error detail; it used to
// also jump to the originating tab, which surprised anyone just trying to read a log entry.
function revealTab(record: OpRecord): void {
  const tab = record.tabId ? tabsStore.tabs.find((t) => t.id === record.tabId) : undefined;
  if (!tab) return;
  // The dock lists every mode's ops: switch to the tab's own workspace so the reveal is visible.
  modeStore.setMode(workspaceKeyOf(tab));
  tabsStore.activateTab(tab.id);
}

function opSqlDialect(record: OpRecord) {
  return sqlDialectFor(connectionFor(record)?.kind);
}

// D10: Re-run reopens the exact command text through a fresh console tab and runs it
// immediately — the one execution path built for arbitrary, operator-supervised statement
// replay, rather than blindly re-invoking whatever op kind (read/count/mutate/execute)
// produced the row. P23 D1(c): commandTruncated means record.command is only a 64 KiB prefix of
// what actually ran, so re-running it would silently execute part of a script as though it were
// the whole thing — refused here too, not only via the disabled context-menu item below.
//
// P108 Part 11 F5: two fixes on top of D10's original shape. (1) The tab now opens at
// record.path — the same database/schema the op actually ran against (op_log.path, F5) — not the
// connection's bare default, which could silently replay unqualified DML against a different
// database's same-named table. A pre-F5 row has no recorded path (null/undefined); it falls back
// to '' exactly as D10 always did. There is no separate "does this path still exist" pre-check:
// the reconnect step below dials this exact path the same way Run always does, so a path that no
// longer resolves (a dropped database/schema) surfaces as that same real, descriptive connect
// error instead of silently running somewhere else — refusing to run against the wrong target
// covers the finding's own "refuse" requirement without a second, parallel existence check. (2)
// Routes through the same reconnect step ConsoleView.vue's own ensureConnectedForRun wraps — Run
// itself never fires against a disconnected connection, and Re-run must not either.
//
// P108 Part 12 F16: ensureConnectedOnce (not useConnectionGate — see its own doc comment) is used
// here rather than the reactive gate every live tab's own view binds to a template: Re-run fires
// from a context-menu click, well after this component's setup has already returned, so a fresh
// useConnectionGate() call here built each of its two computed()s with no owning effect scope to
// ever dispose them — a small, permanent leak on every Re-run. It also now checks whether the
// connect actually succeeded: Go's Connect never rejects for a bad connection, it resolves with
// that state, so a reconnect that lands on 'error' used to still run markHydrated and the SQL
// below against a connection that was never actually connected. A genuine rejection (the IPC call
// itself failing) is caught rather than left as an unhandled promise rejection — onRerun runs
// fire-and-forget from the context menu (`run: () => void onRerun(record)`), so nothing else would
// ever see it. Either way the tab this already opened is left showing its own reconnect gate
// (ensureConnectedOnce never marks it hydrated on a failure) — no toast to build for it: that is
// the same feedback every other reconnect-gated view already gives on a failed connect.
async function onRerun(record: OpRecord): Promise<void> {
  if (!record.connectionId || !record.command || record.commandTruncated) return;
  const dialect = opSqlDialect(record);
  // A Discover statement is one JSON request; splitting it on ';' would cut a query string.
  const statements =
    connectionFor(record)?.kind === 'kibana'
      ? [record.command]
      : splitSqlStatements(record.command, {
          ...lexOptionsFor(dialect),
          slashSlashComments: connectionFor(record)?.kind === 'mongodb',
        }).map((s) => s.text);
  if (statements.length === 0) return;
  const tabId = tabsStore.openConsoleTab(record.connectionId, record.path ?? '');
  try {
    const connected = await ensureConnectedOnce(tabId, record.connectionId);
    if (!connected) return;
    await consoleViewStore.run(tabId, statements);
  } catch (err) {
    logError('operations.rerun', 'Re-run: failed to reconnect/run', err);
  }
}

const facet = computed(() => {
  const ids = [...new Set(opsStore.records.map((r) => r.connectionId ?? ''))];
  const options = ids.map((id) => ({
    value: id,
    label:
      id === '' ? t('studio.operations.noConnection.label') : displayValue(connectionNameFor(id)),
  }));
  if (auto.merge([]).length > 0) {
    options.push({ value: AUTOMATION_FACET, label: t('common.module.automations.label') });
  }
  return { label: t('studio.common.nodeKind.connection.label'), options };
});

function connectionNameFor(id: string): string | undefined {
  return connectionsStore.connectionRecord(id)?.name;
}

function sourceText(record: OpRecord): string {
  return record.source === 'mcp' ? t('common.mcp.label') : t('common.user.label');
}

function fieldsFor(record: OpRecord | AutomationOp): OpLogField[] {
  if (isAutomationOp(record)) {
    return [
      { label: t('common.script.label'), value: record.run.scriptName },
      { label: t('common.field.source.label'), value: triggerLabel(record.run) },
    ];
  }
  const fields: OpLogField[] = [
    {
      label: t('studio.common.nodeKind.connection.label'),
      value: connectionNameFor(record.connectionId ?? '') ?? '',
    },
    { label: t('studio.operations.field.path.label'), value: record.path ?? '', code: true },
    { label: t('common.key.tab.label'), value: record.tabId ? tabTitleFor(record) : '' },
    { label: t('common.field.source.label'), value: sourceText(record) },
    {
      label: t('studio.keyValue.rows.aria'),
      value: record.rows === null ? '' : String(record.rows),
    },
  ];
  return fields.filter((f) => f.value !== '');
}

function codeLanguage(record: OpRecord | AutomationOp): EditorLanguageId | null {
  if (
    isAutomationOp(record) ||
    record.kind === 'http' ||
    record.kind === 'preconnect' ||
    record.kind === 'kubernetes'
  )
    return null;
  return consoleLanguageFor(connectionFor(record)?.kind);
}

function detailsText(record: OpRecord): string {
  return [
    record.command,
    record.error,
    connectionNameFor(record.connectionId ?? ''),
    record.path,
    record.tabId ? tabTitleFor(record) : null,
  ]
    .filter(Boolean)
    .join('\n');
}

function menuFor(record: OpRecord | AutomationOp): MenuItem[] {
  if (isAutomationOp(record)) return auto.menuFor(record);
  const hasTab = record.tabId !== null && tabsStore.tabs.some((t) => t.id === record.tabId);
  const canSql =
    !!record.connectionId &&
    connectionsStore.states[record.connectionId]?.caps?.sql &&
    // Only execute() carries a Discover request; the other kibana op kinds log metadata calls.
    (connectionFor(record)?.kind !== 'kibana' || record.kind === 'execute');
  return [
    {
      type: 'item',
      id: 'reveal-tab',
      label: t('studio.operations.menu.revealTab.label'),
      disabled: !hasTab,
      run: () => revealTab(record),
    },
    {
      type: 'item',
      id: 're-run',
      label: t('studio.operations.menu.rerun.label'),
      icon: ICON.run,
      // P23 D1(c): a truncated command is only a 64 KiB prefix of what actually ran — Copy
      // command stays enabled (copying a prefix is honest and useful), but Re-run must not.
      disabled:
        !record.command || !canSql || record.commandTruncated || record.kind === 'preconnect',
      run: () => void onRerun(record),
    },
    { type: 'separator' },
    ...opLogMenuItems(
      record,
      () => void opsStore.cancelOp(record.id),
      true,
      undefined,
      detailsText(record),
    ),
  ];
}

function detailLines(record: OpRecord | AutomationOp): OpLogDetailLine[] {
  if (isAutomationOp(record)) return auto.detailLines(record);
  return record.commandTruncated
    ? [{ text: t('studio.operations.commandTruncated.status'), tone: 'error' }]
    : [];
}

// Counts the preconnect delay down in the command cell; falls back to the command once it elapses.
function preconnectNote(record: OpRecord | AutomationOp, now: number): string | null {
  if (isAutomationOp(record)) return auto.runningNote(record);
  if (record.kind !== 'preconnect' || !record.delayMs) return null;
  const left = Date.parse(record.startedAt) + record.delayMs - now;
  if (left <= 0) return null;
  return t('studio.operations.preconnectDelay.status', {
    remaining: formatElapsed(Math.ceil(left / 1000) * 1000),
  });
}
</script>

<template>
  <OpLogPanel
    v-model:filter-text="opsStore.filterText"
    v-model:status-filter="opsStore.statusFilter"
    v-model:facet-values="opsStore.facetValues"
    :records="records"
    :running-count="runningCount"
    :loading="!opsStore.hydrated"
    :empty-hint="t('studio.operations.empty.hint')"
    :columns="columns"
    :clear-hint="t('studio.operations.clear.hint')"
    :menu-for="menuFor"
    :running-note="preconnectNote"
    :can-cancel="(record) => (isAutomationOp(record) ? auto.canCancel(record) : true)"
    :kind-label="(record) => (isAutomationOp(record) ? auto.kindLabel() : record.kind)"
    :status-note="(record) => (isAutomationOp(record) ? auto.statusNote(record) : null)"
    :detail-lines="detailLines"
    :fields-for="fieldsFor"
    :code-language="codeLanguage"
    :facet="facet"
    @clear="opsStore.clearOps"
    @reset-filters="opsStore.resetFilters"
    @cancel="onCancel"
  >
    <template #cell="{ column, record }">
      <template v-if="isAutomationOp(record)">
        <span
          v-if="column.id === 'connection'"
          class="truncate min-w-0"
          data-testid="op-automation-name"
          >{{
            record.run.scriptName
          }}</span
        >
        <span
          v-else-if="column.id === 'source'"
          class="truncate min-w-0"
          data-testid="op-source-cell"
          >{{
            triggerLabel(record.run)
          }}</span
        >
        <span v-else-if="column.id === 'rows'" />
      </template>
      <span v-else-if="column.id === 'connection'" class="flex items-center min-w-0 gap-1">
        <span
          v-if="connectionFor(record)"
          :class="colorMarkClass('dot', connectionFor(record)?.color)"
        />
        <span class="truncate min-w-0">{{ displayValue(connectionFor(record)?.name) }}</span>
      </span>
      <span
        v-else-if="column.id === 'source'"
        class="truncate min-w-0"
        data-testid="op-source-cell"
        >{{
          sourceText(record)
        }}</span
      >
      <span v-else-if="column.id === 'rows'" class="justify-self-end text-right tabular-nums">{{
        displayValue(record.rows)
      }}</span>
    </template>
  </OpLogPanel>
</template>
