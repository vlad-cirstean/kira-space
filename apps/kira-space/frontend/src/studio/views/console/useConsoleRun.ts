import type { ConnectionKind } from '@shared/domain/connection';
import {
  type SplitSqlOptions,
  splitSqlStatements,
  statementAtCursor,
  statementAtOffset,
} from '@shared/domain/sqlSplit';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { type ComputedRef, computed, type Ref, ref } from 'vue';
import { useConnectionsStore } from '../../state/connections';
import type { ConsoleTabRecord } from '../../state/tabDomain';
import type { SqlDialect } from '../shared/sqlIdent';
import { isExplainable } from './explain';
import { useConsoleViewStore } from './state';

/** Run, run all and explain of a console tab; each reconnects first when the connection dropped. */
export function useConsoleRun(opts: {
  tab: Ref<ConsoleTabRecord>;
  running: ComputedRef<boolean>;
  needsReconnect: Ref<boolean>;
  onReconnectAndLoad: () => Promise<boolean>;
  connectionKind: ComputedRef<ConnectionKind | undefined>;
  dialect: ComputedRef<SqlDialect | undefined>;
  splitOptionsFor: (d: SqlDialect | undefined) => SplitSqlOptions;
  cursorPos: Ref<number>;
}) {
  const consoleViewStore = useConsoleViewStore();
  const connectionsStore = useConnectionsStore();

  const connect = useAsyncAction(async () => {
    if (!opts.needsReconnect.value) return;
    if (await opts.onReconnectAndLoad()) return;
    const id = opts.tab.value.connectionId;
    throw new Error(
      (id ? connectionsStore.states[id]?.error : undefined) ??
        t('studio.console.connectFailed.error'),
    );
  });
  const starting = connect.pending;
  const connectError = connect.error;
  const runBlocked = computed(() => opts.running.value || starting.value);

  // Keyed on text and dialect only, so a caret move runs just the cheap offset scan below.
  const splitStatements = computed(() =>
    opts.dialect.value === undefined
      ? []
      : splitSqlStatements(opts.tab.value.state.text, opts.splitOptionsFor(opts.dialect.value)),
  );
  const statementAtCursorText = computed<string | undefined>(() => {
    if (opts.dialect.value === undefined) return undefined;
    return statementAtOffset(splitStatements.value, opts.tab.value.state.text, opts.cursorPos.value)
      ?.text;
  });
  const canExplain = computed(
    () =>
      !opts.running.value &&
      statementAtCursorText.value !== undefined &&
      isExplainable(statementAtCursorText.value),
  );
  const explainTooltip = computed(() => {
    if (opts.running.value) return t('studio.console.explain.running.tooltip');
    return canExplain.value
      ? t('studio.console.explain.ready.tooltip')
      : t('studio.console.explain.blocked.tooltip');
  });
  const explainError = ref<string | null>(null);

  function runStatement(): void {
    if (runBlocked.value) return;
    const stmt = statementAtCursor(
      opts.tab.value.state.text,
      opts.cursorPos.value,
      opts.splitOptionsFor(opts.dialect.value),
    );
    if (!stmt) return;
    void (async () => {
      if (await connect.run()) await consoleViewStore.run(opts.tab.value.id, [stmt.text]);
    })();
  }

  function runAll(): void {
    if (runBlocked.value) return;
    const statements = splitSqlStatements(
      opts.tab.value.state.text,
      opts.splitOptionsFor(opts.dialect.value),
    ).map((s) => s.text);
    if (statements.length === 0) return;
    void (async () => {
      if (await connect.run()) await consoleViewStore.run(opts.tab.value.id, statements);
    })();
  }

  function onExplain(): void {
    const kind = opts.connectionKind.value;
    const stmt = statementAtCursorText.value;
    if (!kind || !stmt || !canExplain.value || starting.value) return;
    void (async () => {
      if (!(await connect.run())) return;
      const result = await consoleViewStore.explain(opts.tab.value.id, kind, stmt);
      explainError.value = result.ok ? null : result.reason;
    })();
  }

  return {
    starting,
    connectError,
    runBlocked,
    canExplain,
    explainTooltip,
    explainError,
    runStatement,
    runAll,
    onExplain,
  };
}
