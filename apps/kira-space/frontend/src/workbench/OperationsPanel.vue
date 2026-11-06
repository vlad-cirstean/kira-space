<script setup lang="ts">
import type { EditorLanguageId } from '@shared/domain/editor';
import type { AppMode } from '@shared/domain/mode';
import { splitSqlStatements } from '@shared/domain/sqlSplit';
import { Badge } from '@theme/components/ui/badge';
import { Toggle } from '@theme/components/ui/toggle';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colorMarkClass } from '@theme/connColor';
import { type MessageKey, t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { type AutomationOp, isAutomationOp } from '@workbench/automations/runs/runOps';
import { triggerLabel, useAutomationOps } from '@workbench/automations/runs/useAutomationOps';
import { displayValue, formatDuration, formatElapsed } from '@workbench/behaviour/format';
import { logError } from '@workbench/bridge/log';
import OpLogPanel from '@workbench/components/OpLogPanel.vue';
import {
  type OpLogColumn,
  type OpLogDetailLine,
  type OpLogField,
  type OpLogStep,
  opLogMenuItems,
} from '@workbench/lib/opLog';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed } from 'vue';
import { useModeStore } from '../state/mode';
import { DB_FACET_PREFIX, useOpsStore } from '../state/ops';
import type { SpaceOpRecord, SpaceOpStep } from '../state/opsDomain';
import { TAB_KIND_MODE } from '../state/tabDomain';
import { TAB_KINDS } from '../state/tabKinds';
import { useTabsStore } from '../state/tabs';
import { useConnectionsStore } from '../studio/state/connections';
import { consoleLanguageFor } from '../studio/views/console/format';
import { useConsoleViewStore } from '../studio/views/console/state';
import { lexOptionsFor, sqlDialectFor } from '../studio/views/shared/sqlIdent';
import { ensureConnectedOnce } from '../studio/views/shared/useConnectionGate';

// The shared OpLogPanel over the one op log: git spawns, agent sessions, in-app MCP calls and every
// database, API, Docker and Kubernetes op. Origin (module badge + source), subject, fields, steps,
// the module and connection facet, and a database row's Reveal tab / Re-run are this app's own.
const opsStore = useOpsStore();
const connectionsStore = useConnectionsStore();
const tabsStore = useTabsStore();
const modeStore = useModeStore();
const consoleViewStore = useConsoleViewStore();
const auto = useAutomationOps(opsStore);
const showAutomation = computed(() => opsStore.facetAllows(AUTOMATION_FACET));
const records = computed(() =>
  auto.merge(opsStore.visibleOps).filter((r) => showAutomation.value || !isAutomationOp(r)),
);
const runningCount = computed(() => opsStore.runningCount + auto.runningCount.value);

type Row = SpaceOpRecord | AutomationOp;

const AUTOMATION_FACET = 'automation';

const columns = computed<OpLogColumn[]>(() => [
  { id: 'status', label: t('common.field.status.label'), width: '44px' },
  { id: 'time', label: t('common.field.time.label'), width: '84px', priority: 3 },
  { id: 'origin', label: t('common.field.source.label'), width: '170px', priority: 2 },
  { id: 'subject', label: t('git.search.field.subject.label'), width: '150px', priority: 1 },
  { id: 'kind', label: t('common.field.kind.label'), width: '110px', priority: 4 },
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

const MODULE_NAME: Record<string, MessageKey> = {
  git: 'common.module.git.label',
  ade: 'space.operations.moduleName.ade.label',
  files: 'common.module.files.label',
  vscode: 'space.operations.moduleName.vscode.label',
  memory: 'common.module.memory.label',
  app: 'space.operations.moduleName.app.label',
  database: 'common.module.connections.label',
};

function moduleName(id: string): string {
  const key = MODULE_NAME[id];
  return key ? t(key) : id;
}

const facet = computed(() => {
  const modules = [
    ...new Set(opsStore.records.filter((r) => r.module !== 'database').map((r) => r.module)),
  ];
  const options = modules.map((id) => ({ value: id, label: moduleName(id) }));
  const connectionIds = [
    ...new Set(
      opsStore.records.filter((r) => r.module === 'database').map((r) => r.connectionId ?? ''),
    ),
  ];
  for (const id of connectionIds) {
    options.push({
      value: `${DB_FACET_PREFIX}${id}`,
      label:
        id === '' ? t('studio.operations.noConnection.label') : displayValue(connectionNameFor(id)),
    });
  }
  if (auto.merge([]).length > 0) {
    options.push({ value: AUTOMATION_FACET, label: t('common.module.automations.label') });
  }
  return { label: t('space.operations.module.label'), options };
});

function isDb(record: SpaceOpRecord): boolean {
  return record.category === 'db';
}

function connectionFor(record: SpaceOpRecord) {
  return connectionsStore.connectionRecord(record.connectionId);
}

function connectionNameFor(id: string): string | undefined {
  return connectionsStore.connectionRecord(id)?.name;
}

function tabFor(record: SpaceOpRecord) {
  return record.tabId ? tabsStore.tabs.find((x) => x.id === record.tabId) : undefined;
}

// TAB_KINDS[...].title, not the raw path: an HTTP request tab's path is an opaque constant.
function tabTitleFor(record: SpaceOpRecord): string {
  const tab = tabFor(record);
  return tab ? TAB_KINDS[tab.kind].title(tab) : '—';
}

function sourceText(record: SpaceOpRecord): string {
  if (!isDb(record)) return record.source;
  return record.source === 'mcp' ? t('common.mcp.label') : t('common.user.label');
}

// Right-click only; a no-op once the tab is closed.
function revealTab(record: SpaceOpRecord): void {
  const tab = tabFor(record);
  if (!tab) return;
  modeStore.setMode((tab.workspaceId ?? TAB_KIND_MODE[tab.kind]) as AppMode);
  tabsStore.activateTab(tab.id);
}

// Re-run reopens the exact command through a fresh console tab at the recorded path and runs it. A
// truncated command is only a prefix of what ran, so it is refused. The reconnect step is the one
// Run itself uses; a failed connect leaves the tab on its own reconnect gate.
async function onRerun(record: SpaceOpRecord): Promise<void> {
  if (!record.connectionId || !record.command || record.commandTruncated) return;
  const kind = connectionFor(record)?.kind;
  const dialect = sqlDialectFor(kind);
  // A Discover statement is one JSON request; splitting it on ';' would cut a query string.
  const statements =
    kind === 'kibana'
      ? [record.command]
      : splitSqlStatements(record.command, {
          ...lexOptionsFor(dialect),
          slashSlashComments: kind === 'mongodb',
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

function codeLanguage(record: Row): EditorLanguageId | null {
  if (
    isAutomationOp(record) ||
    !isDb(record) ||
    record.kind === 'http' ||
    record.kind === 'preconnect' ||
    record.kind === 'kubernetes'
  )
    return null;
  return consoleLanguageFor(connectionFor(record)?.kind);
}

const byId = computed(() => new Map(opsStore.records.map((r) => [r.id, r])));

function canCancel(record: Row): boolean {
  return isAutomationOp(record) ? auto.canCancel(record) : record.cancellable;
}

function onCancel(record: Row): void {
  if (isAutomationOp(record)) auto.cancel(record);
  else void opsStore.cancelOp(record.id);
}

function parentTool(record: SpaceOpRecord): string | null {
  return record.parentId ? (byId.value.get(record.parentId)?.kind ?? null) : null;
}

// Counts the preconnect delay down in the command cell; falls back to the command once it elapses.
function runningNote(record: Row, now: number): string | null {
  if (isAutomationOp(record)) return auto.runningNote(record);
  if (record.kind !== 'preconnect' || !record.delayMs) return null;
  const left = Date.parse(record.startedAt) + record.delayMs - now;
  if (left <= 0) return null;
  return t('studio.operations.preconnectDelay.status', {
    remaining: formatElapsed(Math.ceil(left / 1000) * 1000),
  });
}

function statusNote(record: Row): string | null {
  if (isAutomationOp(record)) return auto.statusNote(record);
  return record.repeat > 1 ? t('space.operations.repeat.label', { count: record.repeat }) : null;
}

function stepText(step: SpaceOpStep): string {
  const duration = step.durationMs === null ? '' : formatDuration(step.durationMs);
  return step.exitCode === null
    ? t('common.dotPair.label', { first: step.argv, second: duration })
    : t('space.operations.step.label', { argv: step.argv, exit: step.exitCode, duration });
}

function fieldsFor(record: Row): OpLogField[] {
  const fields: OpLogField[] = isAutomationOp(record)
    ? [
        { label: t('common.script.label'), value: record.run.scriptName },
        { label: t('space.operations.field.trigger.label'), value: triggerLabel(record.run) },
        { label: t('space.operations.field.cwd.label'), value: record.run.cwd, code: true },
      ]
    : isDb(record)
      ? [
          {
            label: t('studio.common.nodeKind.connection.label'),
            value: connectionNameFor(record.connectionId ?? '') ?? record.subject,
          },
          { label: t('studio.operations.field.path.label'), value: record.path ?? '', code: true },
          { label: t('common.key.tab.label'), value: record.tabId ? tabTitleFor(record) : '' },
          { label: t('common.field.source.label'), value: sourceText(record) },
          {
            label: t('studio.keyValue.rows.aria'),
            value: record.rows === null || record.rows === undefined ? '' : String(record.rows),
          },
        ]
      : [
          { label: t('space.operations.module.label'), value: moduleName(record.module) },
          { label: t('space.operations.field.trigger.label'), value: record.trigger },
          { label: t('common.field.source.label'), value: record.source },
          { label: t('space.repo.config.repository.label'), value: record.repoRoot, code: true },
          { label: t('common.task.label'), value: record.subject },
          { label: t('space.operations.field.category.label'), value: record.category },
        ];
  return fields.filter((f) => f.value !== '');
}

function stepsFor(record: Row): readonly OpLogStep[] {
  return isAutomationOp(record) ? [] : record.steps;
}

function detailLines(record: Row): OpLogDetailLine[] {
  if (isAutomationOp(record)) return auto.detailLines(record);
  if (record.commandTruncated) {
    return [{ text: t('studio.operations.commandTruncated.status'), tone: 'error' }];
  }
  return record.stepsDropped > 0
    ? [{ text: t('space.operations.stepsDropped.label', { count: record.stepsDropped }) }]
    : [];
}

function detailsText(record: SpaceOpRecord): string {
  if (isDb(record)) {
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
  const parts = [record.command ?? '', ...record.steps.map(stepText)];
  for (const step of record.steps) if (step.stderr) parts.push(step.stderr);
  if (record.error) parts.push(record.error);
  return parts.filter(Boolean).join('\n');
}

function menuFor(record: Row): MenuItem[] {
  if (isAutomationOp(record)) return auto.menuFor(record);
  const base = opLogMenuItems(
    record,
    () => void opsStore.cancelOp(record.id),
    canCancel(record),
    undefined,
    detailsText(record),
  );
  if (!isDb(record)) return base;
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
      disabled: !tabFor(record),
      run: () => revealTab(record),
    },
    {
      type: 'item',
      id: 're-run',
      label: t('studio.operations.menu.rerun.label'),
      icon: ICON.run,
      disabled:
        !record.command || !canSql || record.commandTruncated || record.kind === 'preconnect',
      run: () => void onRerun(record),
    },
    { type: 'separator' },
    ...base,
  ];
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
    :columns="columns"
    :clear-hint="t('space.operations.clearHint.hint')"
    :empty-hint="t('space.operations.empty.hint')"
    :can-cancel="canCancel"
    :menu-for="menuFor"
    :kind-label="(record) => (isAutomationOp(record) ? auto.kindLabel() : record.kind)"
    :running-note="runningNote"
    :status-note="statusNote"
    :detail-lines="detailLines"
    :fields-for="fieldsFor"
    :code-language="codeLanguage"
    :steps-for="stepsFor"
    :parent-of="(record) => (isAutomationOp(record) ? null : record.parentId || null)"
    :facet="facet"
    @clear="opsStore.clearOps"
    @cancel="onCancel"
    @reset-filters="opsStore.resetFilters"
  >
    <template #toolbar>
      <Toggle
        v-model="opsStore.showBackground"
        variant="outline"
        size="kira"
        data-testid="ops-background"
      >
        {{
          opsStore.backgroundCount > 0
            ? t('space.operations.backgroundCount.label', { count: opsStore.backgroundCount })
            : t('space.operations.background.label')
        }}
      </Toggle>
    </template>
    <template #cell="{ column, record, viaParent }">
      <template v-if="isAutomationOp(record)">
        <span v-if="column.id === 'origin'" class="flex min-w-0 items-center gap-1">
          <Badge>{{ auto.kindLabel() }}</Badge>
          <span class="min-w-0 truncate" data-testid="op-source-cell">{{
            triggerLabel(record.run)
          }}</span>
        </span>
        <Tooltip v-else-if="column.id === 'subject'">
          <TooltipTrigger as-child>
            <span class="min-w-0 truncate">{{
              record.run.taskTitle || record.run.scriptName
            }}</span>
          </TooltipTrigger>
          <TooltipContent>{{ record.run.cwd }}</TooltipContent>
        </Tooltip>
      </template>
      <span v-else-if="column.id === 'rows'" class="justify-self-end text-right tabular-nums">{{
        isDb(record) ? displayValue(record.rows) : ''
      }}</span>
      <span
        v-else-if="column.id === 'subject' && isDb(record)"
        class="flex min-w-0 items-center gap-1"
      >
        <span
          v-if="connectionFor(record)"
          :class="colorMarkClass('dot', connectionFor(record)?.color)"
        />
        <span class="min-w-0 truncate" data-testid="op-connection-cell">{{
          displayValue(connectionFor(record)?.name ?? record.subject)
        }}</span>
      </span>
      <span v-else-if="column.id === 'origin'" class="flex min-w-0 items-center gap-1">
        <Badge>{{ moduleName(record.module) }}</Badge>
        <span class="min-w-0 truncate" data-testid="op-source-cell">{{ sourceText(record) }}</span>
        <span v-if="viaParent && parentTool(record)" class="shrink-0 text-muted-foreground">{{
          t('space.operations.via.label', { tool: parentTool(record) ?? '' })
        }}</span>
      </span>
      <Tooltip v-else-if="column.id === 'subject'">
        <TooltipTrigger as-child>
          <span class="min-w-0 truncate">{{ record.subject || record.repoName }}</span>
        </TooltipTrigger>
        <TooltipContent>{{ record.repoRoot || record.subject }}</TooltipContent>
      </Tooltip>
    </template>
  </OpLogPanel>
</template>
