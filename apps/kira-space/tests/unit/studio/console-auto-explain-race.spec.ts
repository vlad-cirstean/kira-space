// run() used to write `rt.autoExplain = await autoExplainCheck(...)`
// *before* checking whether this run had been superseded — so a superseded, slower run's own
// EXPLAIN result could land after the run that superseded it had already finished and
// cleared/repopulated rt.autoExplain, silently overwriting it with a warning for a statement that
// was never the one executed (its own "Show plan" action would show a plan for a
// statement that was never run as the current result set). This drives run() end to end (same
// style as console-result-cap.spec.ts/console-stop-auto-explain.spec.ts) with the exact
// interleaving: a slow, wide-scan run (A) started first, then superseded by
// a fast, non-explainable run (B) whose own explain check resolves — and finishes entirely —
// before A's slow EXPLAIN finally settles.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { ConnectionSummary } from '@shared/domain/connection';
import type { ExecuteResponse } from '@shared/protocol/dataOps';
import { createTabularPageBuilder, type Page, unpagedPosition } from '@shared/protocol/page';
import { deferred, sleep } from '@workbench/testing/unit/async';
import { bootstrapConsole } from './support/consoleHarness.ts';

const { data, connectionsStore, tabsStore, consoleViewStore } = await bootstrapConsole();

// A minimal Postgres EXPLAIN (FORMAT JSON) payload whose lone Seq Scan estimate clears the
// default 100,000-row threshold (packages/shared/domain/settings.ts) — enough on its own to make
// parsePostgresPlan report overThreshold: true, which is all autoExplainCheck needs to warn.
const WIDE_SCAN_PLAN = JSON.stringify([
  { Plan: { 'Node Type': 'Seq Scan', 'Relation Name': 'big', 'Plan Rows': 999_999 } },
]);

function explainPage(planJson: string): Page {
  const builder = createTabularPageBuilder([
    {
      name: 'QUERY PLAN',
      dataType: 'json',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ]);
  builder.appendRow([planJson]);
  return builder.finish(unpagedPosition(1));
}

function fakeResultPage(): Page {
  // biome-ignore lint/suspicious/noExplicitAny: minimal fake page, real run() only reads kind/rowCount
  return { kind: 'tabular', rowCount: 0 } as any;
}

describe('console auto-explain: a superseded run must not overwrite the current warning (P12 round 1 F6)', () => {
  test('a slow wide-scan run resolving after a fast cheap run leaves the cheap run own state alone', async () => {
    const connectionId = 'conn-auto-explain-race';
    connectionsStore.records.push({
      id: connectionId,
      // biome-ignore lint/suspicious/noExplicitAny: a minimal fixture, not a real ConnectionSummary
      ...({ kind: 'postgres', autoExplain: true, name: 'x', color: 'blue' } as any),
    } as ConnectionSummary);
    const tabId = tabsStore.openConsoleTab(connectionId, 'db');

    type Call = { statements: string[]; resolve: (r: ExecuteResponse) => void };
    const calls: Call[] = [];
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real data.execute
    (data as any).execute = (req: { statements: string[] }) => {
      const d = deferred<ExecuteResponse>();
      calls.push({ statements: req.statements, resolve: d.resolve });
      return d.promise;
    };

    // Run A: explainable, slow — its own EXPLAIN batch is issued but deliberately left unresolved.
    const runA = consoleViewStore.run(tabId, ['SELECT * FROM big']);
    await sleep(10);
    expect(calls).toHaveLength(1); // A's own EXPLAIN batch, in flight

    // Run B: not explainable (isExplainable requires a leading SELECT/WITH) — its own explain
    // check resolves null with no data.execute call at all, so it reaches the real run
    // immediately, superseding A.
    const runB = consoleViewStore.run(tabId, ['UPDATE big SET x = 1']);
    await sleep(10);
    expect(calls).toHaveLength(2); // B's own real run, the only second call
    calls[1]?.resolve({ pages: [fakeResultPage()] });
    await runB;

    expect(consoleViewStore.runtime[tabId]?.status).toBe('idle');
    expect(consoleViewStore.runtime[tabId]?.autoExplain).toBeNull(); // B was never explainable — nothing to warn about

    // A's slow EXPLAIN finally settles, long after B has already finished and shown its own
    // (warning-free) state — this must not resurrect a warning for a statement that never ran.
    calls[0]?.resolve({ pages: [explainPage(WIDE_SCAN_PLAN)] });
    await runA;

    expect(consoleViewStore.runtime[tabId]?.autoExplain).toBeNull();
    expect(calls).toHaveLength(2); // A's own real run must never have been issued either
  });
});
