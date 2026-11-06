// F7 (P108 Part 10): goNext/goPrev used to have no in-flight guard — a second Next click landing
// before the first click's load resolved reused the first click's still-stale nextToken while
// pageIndex had already been bumped twice, so the pager showed a page number two ahead of the
// rows actually on screen. rt.opId !== null now makes a stray second click a no-op until the
// first load lands or fails.
//
// navigation.ts -> viewOp.ts -> bridge/control.ts reaches '/wails/runtime.js' at module scope,
// hence the window mock — same pattern grid-paste-row-cell-kind-target.spec.ts already uses.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { PageCursor } from '@shared/protocol/dataOps';

const { createPageNavigation } = await import(
  '../../../frontend/src/studio/views/shared/page/navigation'
);

interface FakeRuntime {
  opId: string | null;
  nextToken: string | null;
  prevToken: string | null;
  count: { value: number } | null;
}

function fakeHost(pageSize = 10) {
  const tabState = { pageIndex: 0, pageSize };
  const rt: FakeRuntime = { opId: null, nextToken: 'tok-1', prevToken: null, count: null };
  const loadCalls: { cursor: PageCursor; revertPageIndexOnFailure?: number }[] = [];
  let resolveLoad: (() => void) | null = null;

  const host = {
    tab: () => tabState,
    patch: (_id: string, patch: { pageIndex: number }) => {
      tabState.pageIndex = patch.pageIndex;
    },
    runtime: () => rt,
    ensureRuntime: () => rt,
    load: (_id: string, cursor: PageCursor, revertPageIndexOnFailure?: number) => {
      loadCalls.push({ cursor, revertPageIndexOnFailure });
      rt.opId = `op-${loadCalls.length}`;
      return new Promise<void>((resolve) => {
        resolveLoad = () => {
          rt.opId = null;
          resolve();
        };
      });
    },
  };

  return { host, tabState, rt, loadCalls, finishLoad: () => resolveLoad?.() };
}

describe('createPageNavigation in-flight guard (F7, P108 Part 10)', () => {
  test('a second goNext call while a load is in flight is a no-op, not a second optimistic advance', async () => {
    const { host, tabState, loadCalls, finishLoad } = fakeHost();
    const nav = createPageNavigation(host);

    const first = nav.goNext('t1');
    expect(tabState.pageIndex).toBe(1); // first click's optimistic advance did happen

    const second = nav.goNext('t1'); // stray click while op 1 is still in flight
    await second;
    expect(tabState.pageIndex).toBe(1); // never stacked to 2
    expect(loadCalls.length).toBe(1); // load() called once, not twice

    finishLoad();
    await first;
  });

  test('goNext works normally again once the in-flight load has resolved', async () => {
    const { host, tabState, loadCalls, finishLoad } = fakeHost();
    const nav = createPageNavigation(host);

    const first = nav.goNext('t1');
    finishLoad();
    await first;
    expect(tabState.pageIndex).toBe(1);

    const second = nav.goNext('t1');
    finishLoad();
    await second;
    expect(tabState.pageIndex).toBe(2);
    expect(loadCalls.length).toBe(2);
  });

  test('a second goPrev call while a load is in flight is a no-op', async () => {
    const { host, tabState, loadCalls, finishLoad } = fakeHost();
    tabState.pageIndex = 5;
    const nav = createPageNavigation(host);

    const first = nav.goPrev('t1');
    expect(tabState.pageIndex).toBe(4);

    const second = nav.goPrev('t1');
    await second;
    expect(tabState.pageIndex).toBe(4);
    expect(loadCalls.length).toBe(1);

    finishLoad();
    await first;
  });
});
