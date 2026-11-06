// P21 round 1 functional finding F10 (smaller items): applyLoadFailure's disconnected branch
// returned without touching `rt.status` for grid/documents/keyvalue/stream — only console's own
// onDisconnected callback additionally set it to 'idle'. Left at 'loading' behind the
// ReconnectGate, the invariant was maintained "by luck at a distance" (a later reconnect-then-load
// cycle reset it), not by construction. This pins that the shared branch now sets it directly.
//
// viewOp.ts imports bridge/control and state/tabs, both of which reach '/wails/runtime.js' at
// module scope, so this has to be a dynamic import() after @workbench/testing/unit/window's mock.module
// registration has run (the same pattern row-values-visible-span.spec.ts already uses).
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/studio/state/pinia';

setActivePinia(pinia);

const { applyLoadFailure } = await import('../../../frontend/src/studio/views/shared/viewOp');

function fakeRuntime() {
  return { status: 'loading', opId: 'op-1', error: null };
}

class FakeIpcError extends Error {
  constructor(readonly code: string) {
    super('boom');
  }
}

describe('applyLoadFailure — disconnected branch sets status (F10)', () => {
  test('a disconnect no longer leaves status stuck at loading', () => {
    const rt = fakeRuntime();
    applyLoadFailure(rt, 'op-1', new FakeIpcError('E_ENGINE_DOWN'), 'tab-1');
    expect(rt.status).not.toBe('loading');
    expect(rt.status).toBe('idle');
  });

  test('a superseded opId is still ignored entirely (unrelated to this fix)', () => {
    const rt = fakeRuntime();
    applyLoadFailure(rt, 'op-stale', new FakeIpcError('E_ENGINE_DOWN'), 'tab-1');
    expect(rt.status).toBe('loading'); // untouched — opId didn't match
  });

  test('a genuine error still sets status to error, not idle', () => {
    const rt = fakeRuntime();
    applyLoadFailure(rt, 'op-1', new FakeIpcError('E_QUERY'), 'tab-1');
    expect(rt.status).toBe('error');
  });
});
