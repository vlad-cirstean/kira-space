import type { ConnectionKind } from '@shared/domain/connection';
import type { KibanaConsoleState } from '@shared/domain/kibana';
import { newUuid } from '@shared/ids';
import type { Page } from '@shared/protocol/page';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { control } from '../../bridge/control';
import { data } from '../../bridge/data';
import { useConnectionsStore } from '../../state/connections';
import { useSettingsStore } from '../../state/settings';
import { useStaleDataStore } from '../../state/staleData';
import type { ConsoleTabRecord } from '../../state/tabDomain';
import { useTabsStore } from '../../state/tabs';
import { useDocumentRowsStore } from '../shared/document/rows';
import { applyLoadFailure, classifyLoadError, createRuntimeStore, stopOp } from '../shared/viewOp';
import { explainStatementsFor, isExplainable } from './explain';
import { dropPlan, setPlan } from './explainResults';
import { ExplainTruncatedError, parseExplainPages } from './plan';
import type { QueryPlan } from './planModel';
import { bumpPageVersion, documentRow, drop as dropPage, getPage, setPage } from './resultPages';

/** One result set of a run. `key` is identity and never changes while the result is open — the
 *  "Result N" label the strip prints is its *position*, which renumbers when a sibling
 *  closes, so it is deliberately not stored here. `kind` distinguishes an ordinary
 *  query-result page (resultPages.ts) from a plan result (explainResults.ts) — both share every
 *  other piece of this record's own lifecycle (close/close-others/close-to-the-right/eviction). */
interface ConsoleResult {
  key: string;
  rowCount: number;
  kind: 'page' | 'plan';
}

/** The outcome of the pre-run EXPLAIN batch auto-explain issues for every qualifying
 *  statement in the run that just fired. `kind: 'plans'` is the ordinary case — `worstIndex` names
 *  the one whose own overThreshold or warn-severity issue triggered the strip; the whole array is
 *  kept (not just the worst plan) so a future "show every plan" affordance costs nothing to add,
 *  today only `plans[worstIndex]` is read (the strip's own Show plan action). `kind: 'truncated'`
 *  is a distinct outcome from "nothing to warn about" (null, no strip at
 *  all): at least one qualifying statement's own plan was too large to parse (page/chunk.go's 64
 *  KiB MaxCellBytes), so whether it would have warned is genuinely unknown — the strip says so
 *  rather than silently claiming there was nothing to check. */
type AutoExplainPlans = Array<{ statement: string; plan: QueryPlan }>;
type AutoExplainState =
  | { kind: 'plans'; plans: AutoExplainPlans; worstIndex: number }
  | { kind: 'truncated' };

interface ConsoleViewRuntime {
  status: 'idle' | 'running' | 'error' | 'cancelled';
  error: { code: string; message: string } | null;
  opId: string | null; // the in-flight op, for the stop button
  // The statements of the latest run, so a view that builds statements itself (kibana Discover)
  // can tell a Re-run it did not issue.
  lastStatements: string[] | null;
  // P12 round 1 finding #5: the pre-run auto-explain batch's own op id, tracked separately from
  // opId (which already holds the *real* run's future id by the time this fires) — without it,
  // Stop had nothing registered on the backend to cancel while this batch was in flight.
  explainOpId: string | null;
  results: ConsoleResult[]; // the last run's result sets — runtime-only, never saved (§8.4)
  activeKey: string | null; // which result set the single mounted grid shows (P40 D2)
  searchOpen: boolean; // mirrors views/{grid,documents,keyvalue}/state.ts's own flag (P40 D8)
  nextSeq: number; // per-tab monotonic result-set counter (P40 D1) — never reused
  // `${resultKey}:${docId}`, present = expanded. Unlike a Mongo data tab's own
  // documents/state.ts (persisted, default-expanded), a console result set is
  // runtime-only and starts collapsed — a find() result is usually skimmed for shape, and
  // expanding a couple hundred documents by default is a very tall list nobody asked for.
  expandedDocIds: Set<string>;
  // P18 D19: null except right after a run whose pre-flight EXPLAIN found something worth a
  // warning — cleared on the next run and on the next document edit (ConsoleView.vue's own
  // formatError/explainError precedent).
  autoExplain: AutoExplainState | null;
  // Real-interaction fix (reported bug — column widths reset on every subsequent query in the
  // same session): ConsoleSlickGrid.vue's own header used to state "no persisted column widths —
  // always the measured/default width, reset on every remount" as a deliberate design
  // decision, because a console result set has no addressable table/tab state the way
  // SlickGridHost.vue's own DataTabState.columnWidths does (grid/SlickGridHost.vue's `buildColumns`
  // already prefers `tab().state.columnWidths` over a fresh measurement, so that surface was
  // already correct). Every query run replaces `activeKey` with a brand-new `pageKey`
  // (resultPageKey's own monotonic `seq`), which remounts ConsoleSlickGrid.vue via
  // ConsoleResultGrid.vue's `:key="pageKey"` and re-measures from scratch — losing both the
  // first run's auto-sized widths and any manual resize the very next time a statement runs.
  // Keyed by column name, living on this per-tab runtime record (not per-result) so it survives
  // every subsequent run in the same still-open tab — the "session" the bug report names —
  // and is discarded only when the runtime itself is (a genuinely new session: the tab closes,
  // `registerTabRuntimeCleanup` below deletes the whole record). Never touched by `dropResults`
  // (replace-mode's own per-run reset) — a width choice is not a query result.
  columnWidths: Record<string, number>;
}

function defaultRuntime(): ConsoleViewRuntime {
  return {
    status: 'idle',
    error: null,
    opId: null,
    lastStatements: null,
    explainOpId: null,
    results: [],
    activeKey: null,
    searchOpen: false,
    nextSeq: 0,
    expandedDocIds: new Set(),
    autoExplain: null,
    columnWidths: {},
  };
}

// P43 iter2 F23a: `rt.expandedDocIds` is keyed `${resultKey}:${docId}` — a result's own keys are
// contiguous under one prefix by construction (resultPageKey's `seq` never repeats), so pruning
// by prefix is correct without touching any other result's entries.
function pruneExpandedDocIds(rt: ConsoleViewRuntime, key: string): void {
  const prefix = `${key}:`;
  for (const id of rt.expandedDocIds) {
    if (id.startsWith(prefix)) rt.expandedDocIds.delete(id);
  }
}

/** `views/grid/page.ts`-style key for one result set. `seq` is the tab's own monotonic
 *  `nextSeq` (P40 D1), not an array index — a result keeps the same key for its whole lifetime
 *  even after an earlier sibling result set closes. */
export function resultPageKey(tabId: string, seq: number): string {
  return `${tabId}:result:${seq}`;
}

/** Releases everything a result set retains outside its own `{key, rowCount}` record: its page
 *  (resultPages.ts), its document-row parse cache, and its expanded-doc-id entries. Every place
 *  that removes a result from `rt.results` — a full clear, a strip ×, closeOthers/closeToTheRight,
 *  and evictOldestResults below — goes through this one release path. */
function releaseResult(rt: ConsoleViewRuntime, result: ConsoleResult): void {
  const documentRowsStore = useDocumentRowsStore();
  dropPage(result.key);
  dropPlan(result.key);
  documentRowsStore.unregisterDocumentRows(result.key);
  documentRowsStore.dropRows(result.key);
  pruneExpandedDocIds(rt, result.key);
}

// Append mode (the default) never otherwise frees an old result's page — nothing
// short of the user's own close/close-others action or closing the tab did before this. A session
// of many small runs in one left-open tab would retain every one of their full decoded pages
// forever. `protectedCount` is always the run that just completed: only *earlier* runs' results are
// ever evicted here, however many statements the new run itself produced.
const MAX_RESULTS_PER_TAB = 50;

function evictOldestResults(rt: ConsoleViewRuntime, protectedCount: number): void {
  const maxEvictable = rt.results.length - protectedCount;
  const overflow = Math.min(maxEvictable, rt.results.length - MAX_RESULTS_PER_TAB);
  if (overflow <= 0) return;
  for (const result of rt.results.splice(0, overflow)) releaseResult(rt, result);
}

export function setText(tabId: string, text: string): void {
  useTabsStore().patchConsoleTabState(tabId, { text });
}

export function setKibana(tabId: string, kibana: KibanaConsoleState): void {
  useTabsStore().patchConsoleTabState(tabId, { kibana });
}

export function setNewResultSet(tabId: string, on: boolean): void {
  useTabsStore().patchConsoleTabState(tabId, { newResultSet: on });
}

// D19: a pasted 200-statement script must not become 200 EXPLAINs — auto-explain skips itself
// entirely rather than issuing a batch this size.
const AUTO_EXPLAIN_MAX_STATEMENTS = 10;

function isFlaggedPlan(plan: QueryPlan): boolean {
  return plan.overThreshold || plan.issues.some((issue) => issue.severity === 'warn');
}

// On a multi-statement Run-all, the strip's "Show plan" action should
// point at the *worst* offender, not just the first one flagged — an early statement tripping a
// mild warning must not eclipse a much more expensive one later in the same run. Picks the
// flagged plan with the highest estimatedRowsRead, falling back to the first flagged index when
// none of them carries a row estimate at all (e.g. every statement is SQLite, which never reports
// one).
function worstFlaggedIndex(plans: AutoExplainPlans): number {
  let best = -1;
  let bestRows = -1;
  for (let i = 0; i < plans.length; i++) {
    const plan = plans[i]?.plan;
    if (!plan || !isFlaggedPlan(plan)) continue;
    if (best === -1) best = i; // the fallback, if nothing below ever beats it
    const rows = plan.estimatedRowsRead;
    if (rows !== undefined && rows > bestRows) {
      bestRows = rows;
      best = i;
    }
  }
  return best;
}

// Filters to explainable statements, composes every one of their own EXPLAIN
// statements (one for most dialects, two for ClickHouse) into a *single* data:execute call,
// then re-slices the returned pages back per originating statement to parse each one. Returns
// null whenever there is nothing to warn about — no qualifying statement, too many of them, or
// the EXPLAIN call itself failed (swallowed, never surfaced for this path) — or
// `{kind: 'truncated'}` when a plan was too large to parse at all: genuinely unknown
// whether it would have warned, so this is deliberately not folded into the null case.
async function autoExplainCheck(
  tab: ConsoleTabRecord,
  kind: ConnectionKind,
  statements: string[],
  opId: string,
): Promise<AutoExplainState | null> {
  if (!tab.connectionId) return null;
  const explainable = statements.filter((s) => isExplainable(s));
  if (explainable.length === 0 || explainable.length > AUTO_EXPLAIN_MAX_STATEMENTS) return null;

  const perStatementSql = explainable.map((s) => explainStatementsFor(kind, s));
  const allExplainStatements = perStatementSql.flat();
  if (allExplainStatements.length === 0) return null;

  try {
    const response = await data.execute({
      opId,
      tabId: tab.id,
      connectionId: tab.connectionId,
      path: tab.path,
      statements: allExplainStatements,
    });
    const plans: AutoExplainPlans = [];
    let cursor = 0;
    for (let i = 0; i < explainable.length; i++) {
      const pageCount = perStatementSql[i]?.length ?? 0;
      const pages = response.pages.slice(cursor, cursor + pageCount);
      cursor += pageCount;
      plans.push({
        statement: explainable[i],
        plan: parseExplainPages(kind, pages, useSettingsStore().advanced.expensiveQueryRows),
      });
    }
    const worstIndex = worstFlaggedIndex(plans);
    return worstIndex === -1 ? null : { kind: 'plans', plans, worstIndex };
  } catch (err) {
    // An EXPLAIN-call failure never blocks the real run — except a cancellation. That
    // is the user pressing Stop while this pre-run batch is in flight, not the batch itself
    // failing, and must stop the real run too rather than let it fire the moment this batch
    // settles — rethrown so run()'s own catch can react.
    if (classifyLoadError(err).kind === 'cancelled') throw err;
    // Distinct from "nothing to warn about" — whether this statement would have warned is genuinely
    // unknown, not "no issue found", so the strip must say so rather than silently showing nothing
    // (the previous behavior every other parse failure here still gets, since those are genuine
    // EXPLAIN-call failures, not this).
    if (err instanceof ExplainTruncatedError) return { kind: 'truncated' };
    return null;
  }
}

// The pre-run gate, split out of run() (below) to keep its cognitive complexity in bounds.
// Returns 'stop' when run() must return without ever issuing the real statement: the EXPLAIN batch
// was cancelled, or the tab closed or was superseded by a newer run while it was in flight.
// 'proceed' otherwise, with `rt.autoExplain` already set to whatever the batch found.
async function runAutoExplainGate(
  tab: ConsoleTabRecord,
  kind: ConnectionKind,
  rt: ConsoleViewRuntime,
  opId: string,
  statements: string[],
  isTabOpen: () => boolean,
): Promise<'proceed' | 'stop'> {
  const explainOpId = newUuid();
  rt.explainOpId = explainOpId;
  let autoExplainResult: AutoExplainState | null;
  try {
    autoExplainResult = await autoExplainCheck(tab, kind, statements, explainOpId);
  } catch (err) {
    // Only a cancellation reaches here (autoExplainCheck's own catch swallows everything else) —
    // Stop was pressed while this batch was the only op registered on the backend, so the real run
    // must not fire at all, not just lose its warning.
    // Guarded by identity, not a bare clear — a second, overlapping run
    // (runStatement/runAll have no `running` guard against each other) may have already stamped
    // its own explainOpId here by the time this catch runs; clearing unconditionally would discard
    // *that* run's id, leaving Stop with nothing registered on the backend to cancel.
    if (rt.explainOpId === explainOpId) rt.explainOpId = null;
    applyLoadFailure(rt, opId, err, tab.id, {
      onDisconnected: () => {
        rt.status = 'idle';
      },
    });
    return 'stop';
  }
  // P12 round 2 finding #4: same identity guard as above — an overlapping run's own explainOpId
  // must survive this run's clear.
  if (rt.explainOpId === explainOpId) rt.explainOpId = null;
  // P108 Part 11 F1: same tab-liveness check the real execute's own guard makes below — status
  // alone would already catch a tab close now that cleanup marks the detached rt 'cancelled', but
  // this keeps the two guards symmetric rather than relying on that indirectly.
  if (!isTabOpen()) return 'stop';
  // Not only opId (a newer run superseding this one) — status too, since a Stop press during this
  // batch is only visible through status, not through opId changing (finding #5).
  if (rt.opId !== opId || rt.status !== 'running') return 'stop';
  // P12 round 1 finding #6: assigned only after the supersession check above, not before — a
  // superseded run's own (possibly slower) EXPLAIN result must never overwrite whatever the run
  // that actually superseded it already put here (including having cleared it to null).
  rt.autoExplain = autoExplainResult;
  return 'proceed';
}

export const useConsoleViewStore = defineStore('consoleView', () => {
  const { runtime, ensureRuntime, toggleSearchOpen, setSearchOpen } =
    createRuntimeStore<ConsoleViewRuntime>(defaultRuntime);

  // closeTab has no way to import this leaf module directly — registers here.
  // state/tabs.ts's dropAllPagesForTab already frees this tab's entries in resultPages.ts's own
  // `pages` map directly (rt.results holds only { key, rowCount } now, never a Page, so
  // there is no second reference here left to release before the record itself is dropped).
  // dropRows(result.key) is the same release for views/shared/document/rows.ts's
  // own per-result parse cache — unregisterDocumentRows alone only drops the *source* pointer
  // (rows.ts:25-27's own `rowSources.delete`), leaving every already-parsed document tree for that
  // result retained under a key `nextSeq` guarantees is never reused, for the life of the process.
  registerTabRuntimeCleanup((tabId) => {
    const rt = runtime[tabId];
    if (rt) {
      // P12 round 1 finding #10: routed through the one release path every user-driven removal
      // (closeResult, closeOtherResults, closeResultsToTheRight, dropResults, evictOldestResults)
      // already uses — this used to call unregisterDocumentRows/dropRows directly and skip dropPage
      // and dropPlan, so a closed tab's decoded pages and (especially) its explainResults.ts plan
      // entries — QueryPlan.raw alone tens of KB each, nextSeq never reused — were retained in their
      // module-level maps for the life of the process.
      for (const result of rt.results) releaseResult(rt, result);
      // Deleting the entry below detaches `rt` — it stays exactly as it is, so an
      // in-flight run() or explain() awaiting on this same object still reads status 'running' and
      // its own opId unchanged forever. Their post-await guards only check identity against that
      // detached object, never `runtime[tabId]`'s own presence, so without this the real statement
      // (or auto-explain's EXPLAIN batch) fires server-side after the tab is already gone. Cancel
      // both possible in-flight ops and flip status so every such guard sees the close.
      if (rt.status === 'running') {
        if (rt.explainOpId) fireAndForget(control.opsCancel(rt.explainOpId), 'cancelExplain');
        if (rt.opId) fireAndForget(control.opsCancel(rt.opId), 'cancelQuery');
        rt.status = 'cancelled';
      }
    }
    delete runtime[tabId];
  });

  function isResultDocExpanded(tabId: string, resultKey: string, id: string): boolean {
    return runtime[tabId]?.expandedDocIds.has(`${resultKey}:${id}`) ?? false;
  }

  // ConsoleSlickGrid.vue's own onMounted/onColumnsResized read and write
  // through these two rather than reaching into `runtime[tabId]` directly, mirroring
  // grid/SlickGridHost.vue's own `currentWidths()`/`onColumnsResized` pair (tab.state.columnWidths)
  // as closely as a runtime-only, no-tab-state console result can.
  function consoleColumnWidths(tabId: string): Record<string, number> {
    return runtime[tabId]?.columnWidths ?? {};
  }

  function setConsoleColumnWidths(tabId: string, widths: Record<string, number>): void {
    ensureRuntime(tabId).columnWidths = widths;
  }

  function toggleResultDocExpanded(tabId: string, resultKey: string, id: string): void {
    const rt = ensureRuntime(tabId);
    const key = `${resultKey}:${id}`;
    if (rt.expandedDocIds.has(key)) rt.expandedDocIds.delete(key);
    else rt.expandedDocIds.add(key);
  }

  // Item (regression pass, task batch P46-4): the expand-all/collapse-all toolbar pair
  // DocumentView.vue's own document tab has (its own state.ts's setAllExpanded) — added here once
  // the console's document results lost their other way to see a whole document at a glance (the
  // cell editor dock, now removed as a redundant second copy of the same DocumentTree, P42 D11).
  // Unlike that tab's map (absent = expanded, D2/D32's own comment), this Set's model is the
  // opposite — absent = collapsed (this file's own defaultRuntime comment) — so *expand* all adds
  // every id instead of clearing the set, and *collapse* all prunes by prefix same as a result close.
  function setAllResultDocsExpanded(
    tabId: string,
    resultKey: string,
    ids: string[],
    expand: boolean,
  ): void {
    const rt = ensureRuntime(tabId);
    if (!expand) {
      pruneExpandedDocIds(rt, resultKey);
      return;
    }
    for (const id of ids) rt.expandedDocIds.add(`${resultKey}:${id}`);
  }

  function dropResults(tabId: string): void {
    const rt = runtime[tabId];
    if (!rt) return;
    for (const result of rt.results) releaseResult(rt, result);
    rt.results = [];
    rt.activeKey = null;
  }

  /** The strip's ×  (P40 D5) — drops the result's page (so the retained-byte guard, F21, sees it
   *  freed), removes its entry, and re-selects a neighbour: the next result, else the previous,
   *  else none, mirroring what happens today when a tab ends up with zero results. */
  function closeResult(tabId: string, key: string): void {
    const rt = runtime[tabId];
    if (!rt) return;
    const index = rt.results.findIndex((r) => r.key === key);
    if (index === -1) return;
    releaseResult(rt, rt.results[index]);
    rt.results.splice(index, 1);
    if (rt.activeKey === key) {
      rt.activeKey = (rt.results[index] ?? rt.results[index - 1])?.key ?? null;
    }
  }

  /** Result-strip context menu (P42 D8), mirroring TabStrip.vue's own closeOthers/closeToTheRight
   *  over one tab's result sets rather than the app's whole tab list. Keeps `key` active if it
   *  survives; re-selects the last survivor otherwise. */
  function closeOtherResults(tabId: string, key: string): void {
    const rt = runtime[tabId];
    if (!rt) return;
    const keep = rt.results.find((r) => r.key === key);
    if (!keep) return;
    for (const result of rt.results) {
      if (result.key !== key) releaseResult(rt, result);
    }
    rt.results = [keep];
    rt.activeKey = key;
  }

  function closeResultsToTheRight(tabId: string, key: string): void {
    const rt = runtime[tabId];
    if (!rt) return;
    const index = rt.results.findIndex((r) => r.key === key);
    if (index === -1) return;
    const dropped = rt.results.slice(index + 1);
    for (const result of dropped) releaseResult(rt, result);
    rt.results = rt.results.slice(0, index + 1);
    if (dropped.some((r) => r.key === rt.activeKey)) rt.activeKey = key;
  }

  /** Selects which result set the single mounted grid shows (P40 D2). Bumps resultPages'
   *  pageVersion (D9): to every reader of that store — the find toolbar above all — "the page this
   *  scope resolves to has changed" is the same event as a page being replaced under a key. */
  function setActiveResult(tabId: string, key: string): void {
    const rt = runtime[tabId];
    if (!rt) return;
    rt.activeKey = key;
    bumpPageVersion();
  }

  /** The page the tab's active result set holds — the console's answer to the other three views'
   *  `getPage(tabId)`, and the one place "which of this tab's N pages" is resolved (D9). */
  function activePage(tabId: string): Page | null {
    const rt = runtime[tabId];
    if (!rt?.activeKey) return null;
    return getPage(rt.activeKey);
  }

  /** Pushes one plan result the same way `run()` pushes a page result — same append/replace
   *  toggle, same eviction, same close/close-others machinery. Reused by the auto-explain
   *  "Show plan" action (below, same file) so it can push a plan it already parsed without a second
   *  round trip. */
  function pushPlanResult(tabId: string, statement: string, plan: QueryPlan): void {
    const tab = useTabsStore().findConsoleTab(tabId);
    if (!tab) return;
    const rt = ensureRuntime(tabId);
    if (!tab.state.newResultSet) dropResults(tabId);
    const key = resultPageKey(tabId, rt.nextSeq++);
    setPlan(key, { plan, statement });
    rt.results.push({ key, rowCount: 0, kind: 'plan' });
    evictOldestResults(rt, 1);
    rt.activeKey = key;
  }

  // One execute() call per run, covering both "Run statement" (one-element array) and "Run all"
  // (the caller pre-splits via sql-split.ts) — the adapter's own all-or-nothing semantics (P5.5
  // D-plan) mean there is exactly one op-log row and one success/failure outcome per call.
  async function run(tabId: string, statements: string[]): Promise<void> {
    if (statements.length === 0) return;
    const tab = useTabsStore().findConsoleTab(tabId);
    if (!tab?.connectionId) return;
    const rt = ensureRuntime(tabId);
    const opId = newUuid();
    rt.status = 'running';
    rt.opId = opId;
    rt.lastStatements = statements;
    rt.error = null;
    // D19: cleared on every new run, the same "next action supersedes the last one" discipline
    // ConsoleView.vue's own formatError/explainError already follow on a document edit.
    rt.autoExplain = null;

    // D19 rules 1-5: issued and awaited *before* the real run — the query still runs regardless of
    // what this finds (rule 5: warn, never block) — and only when the connection has opted in.
    const connection = useConnectionsStore().connectionRecord(tab.connectionId);
    if (connection?.autoExplain) {
      const gate = await runAutoExplainGate(tab, connection.kind, rt, opId, statements, () =>
        Boolean(runtime[tabId]),
      );
      if (gate === 'stop') return;
    }

    try {
      const response = await data.execute({
        opId,
        tabId,
        connectionId: tab.connectionId,
        path: tab.path,
        statements,
      });
      // The tab may have closed while this run was in flight — `rt` is still a live reference to
      // the detached runtime object (deleting `runtime[tabId]` doesn't touch it), so
      // `rt.opId !== opId` alone doesn't catch this and every write below would leak a result
      // nothing can ever reach again (resultPages.ts's `nextSeq` never repeats).
      if (!runtime[tabId]) return;
      if (rt.opId !== opId) return; // superseded by a newer run

      // P40 D6, default re-flipped back on P46-2: the toolbar toggle decides append vs. replace — on
      // (the default, shown unpressed — see ConsoleView.vue) keeps stacking each run's result
      // set(s) on top of the last; pressing it drops what came before so every run starts fresh.
      if (!tab.state.newResultSet) dropResults(tabId);
      const newResults = response.pages.map((page) => {
        const key = resultPageKey(tabId, rt.nextSeq++);
        setPage(key, page);
        // P42 D11: a document-kind result renders through views/shared/document/'s row model,
        // which resolves a scope key through a registered source rather than an import — this
        // result's own key is that scope, and resultPages.ts's documentRow is its source.
        if (page.kind === 'document') {
          useDocumentRowsStore().registerDocumentRows(key, (row) => documentRow(key, row));
        }
        return { key, rowCount: page.rowCount, kind: 'page' as const };
      });
      rt.results.push(...newResults);
      evictOldestResults(rt, newResults.length);
      rt.activeKey = newResults[0]?.key ?? rt.activeKey;
      useStaleDataStore().stamp(tabId, tab.connectionId);
      rt.status = 'idle';
      rt.opId = null;
    } catch (err) {
      // Cancelled: same discipline as the data grid's stop button — the previous results stay
      // exactly as they were rather than being blanked. Disconnected: `status` has to drop out of
      // 'running' before unmarkHydrated swaps ViewChrome out for ReconnectGate — ConsoleView's
      // `running`/`canStop` read it directly, and onReconnectAndLoad only ever calls
      // markHydrated(), never touches `rt`. Left as 'running', the Stop button would come back
      // permanently enabled (and, since it now tints red while live) permanently red the moment the
      // tab reconnects, for as long as the tab stays open.
      applyLoadFailure(rt, opId, err, tabId, {
        onDisconnected: () => {
          rt.status = 'idle';
        },
      });
    }
  }

  function stop(tabId: string): void {
    const rt = runtime[tabId];
    // Set synchronously, not left to the in-flight promise's own eventual
    // rejection — if the auto-explain batch happens to resolve normally before the cancel signal
    // reaches it (rather than rejecting with E_CANCELLED), rt.status would otherwise still read
    // 'running' when run()'s post-await guard checks it, and the real (possibly expensive) query
    // would fire anyway despite the Stop press. Marking it here makes that guard see the Stop
    // regardless of how the batch's own promise happens to settle.
    if (rt?.status === 'running') rt.status = 'cancelled';
    // The auto-explain batch's own op id — the real run's opId isn't registered on the backend yet
    // while this batch is in flight, so cancelling only opId (below) would be a no-op (finding #5).
    if (rt?.explainOpId) fireAndForget(control.opsCancel(rt.explainOpId), 'cancelExplain');
    stopOp(rt);
  }

  /** D19: the strip clears on the next run (already handled inside `run()` itself) and on the next
   *  document edit — ConsoleView.vue's own onDocChange calls this alongside its formatError/
   *  explainError resets, the same "next action supersedes the last one" discipline. */
  function clearAutoExplain(tabId: string): void {
    const rt = runtime[tabId];
    if (rt) rt.autoExplain = null;
  }

  /** D19's own "Show plan" action: the plan is already parsed (it was needed to decide whether to
   *  warn at all), so this is `pushPlanResult` over the one that triggered the warning — no second
   *  round trip. */
  function showAutoExplainPlan(tabId: string): void {
    const rt = runtime[tabId];
    const state = rt?.autoExplain;
    if (state?.kind !== 'plans') return; // 'truncated' has no plan to show
    const worst = state.plans[state.worstIndex];
    if (!worst) return;
    pushPlanResult(tabId, worst.statement, worst.plan);
  }

  /** Composes `kind`'s own EXPLAIN for `statement`, issues it through the same
   *  `data:execute` op *Run statement* uses, parses the result, and pushes it as a plan result set.
   *  Unlike `run()`, a failure here is returned rather than written into `rt.error` — the silent-
   *  degrade rule is for auto-explain specifically; the manual Explain button surfaces its own
   *  failure the same way Format does (the component-local strip, ConsoleView.vue). */
  async function explain(
    tabId: string,
    kind: ConnectionKind,
    statement: string,
  ): Promise<{ ok: true } | { ok: false; reason: string }> {
    const tab = useTabsStore().findConsoleTab(tabId);
    if (!tab?.connectionId)
      return { ok: false, reason: t('studio.console.explain.noConnection.error') };
    const statements = explainStatementsFor(kind, statement);
    if (statements.length === 0)
      return { ok: false, reason: t('studio.console.explain.nothingToExplain.error') };

    // P12 round 1 finding #5: the same opId/status bookkeeping run() uses, so Explain is cancellable
    // via Stop and shows the same busy state — before, this had no opId registered on the runtime at
    // all, so Stop was inert against it and nothing in the UI showed it was in flight. rt.error is
    // deliberately left untouched either way (see the docstring above): a failure here is reported
    // through this function's own return value, not the shared error strip.
    const rt = ensureRuntime(tabId);
    const opId = newUuid();
    rt.status = 'running';
    rt.opId = opId;

    try {
      const response = await data.execute({
        opId,
        tabId,
        connectionId: tab.connectionId,
        path: tab.path,
        statements,
      });
      if (rt.opId !== opId) return { ok: true }; // superseded by a newer op — nothing left to report into
      const plan = parseExplainPages(
        kind,
        response.pages,
        useSettingsStore().advanced.expensiveQueryRows,
      );
      pushPlanResult(tabId, statement, plan);
      useStaleDataStore().stamp(tabId, tab.connectionId);
      rt.status = 'idle';
      rt.opId = null;
      return { ok: true };
    } catch (err) {
      if (rt.opId === opId) {
        rt.opId = null;
        rt.status = classifyLoadError(err).kind === 'cancelled' ? 'cancelled' : 'idle';
      }
      return { ok: false, reason: errorText(err) };
    }
  }

  /** Marks shown results current for the connection's present session (reconnect; no rerun). */
  function restamp(tabId: string): void {
    const connectionId = useTabsStore().findConsoleTab(tabId)?.connectionId;
    if (connectionId) useStaleDataStore().stamp(tabId, connectionId);
  }

  /** Shown results are stale: connection gone, or produced under another connection session. */
  function isStale(tabId: string): boolean {
    const connectionId = useTabsStore().findConsoleTab(tabId)?.connectionId;
    return connectionId ? useStaleDataStore().isStale(tabId, tabId, connectionId) : false;
  }

  return {
    runtime,
    isStale,
    restamp,
    toggleSearchOpen,
    setSearchOpen,
    isResultDocExpanded,
    consoleColumnWidths,
    setConsoleColumnWidths,
    toggleResultDocExpanded,
    setAllResultDocsExpanded,
    closeResult,
    closeOtherResults,
    closeResultsToTheRight,
    setActiveResult,
    activePage,
    run,
    stop,
    clearAutoExplain,
    showAutoExplainPlan,
    explain,
  };
});
