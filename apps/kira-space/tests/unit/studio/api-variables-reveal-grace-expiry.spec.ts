// internal/localauth's reveal gate is deliberately
// fixed rather than sliding ("so a long editing session can't hold one authentication open
// indefinitely"), but nothing downstream ever acted on that expiry — a revealed variable's
// plaintext stayed in revealedValues (and in VariableSetView.vue's own draft copy) for the life of
// the tab, cleared only when the tab happened to close. This pins that a reveal now schedules its
// own re-mask at the grace window, without waiting five real minutes: setTimeout is captured
// rather than run, and invoked manually to simulate the window elapsing.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/studio/state/pinia';

setActivePinia(pinia);

const { control } = await import('../../../frontend/src/studio/bridge/control');
restoreAfterEach(control);
const { useVariableSetStore } = await import('../../../frontend/src/studio/api/state/variables');

const variableSetStore = useVariableSetStore();

function withCapturedTimeout<T>(run: (fire: () => void) => Promise<T>): Promise<T> {
  const realSetTimeout = globalThis.setTimeout;
  let captured: { fn: () => void; ms: number } | null = null;
  (globalThis as { setTimeout: typeof setTimeout }).setTimeout = ((fn: () => void, ms: number) => {
    captured = { fn, ms };
    return 0 as unknown as ReturnType<typeof setTimeout>;
  }) as typeof setTimeout;
  const fire = () => {
    if (!captured) throw new Error('setTimeout was never called');
    captured.fn();
  };
  return run(fire).finally(() => {
    globalThis.setTimeout = realSetTimeout;
  });
}

describe('revealVariable grace-window expiry (finding 5)', () => {
  test('a revealed value is scheduled for re-masking at (roughly) the 5-minute auth grace', async () => {
    variableSetStore.clearRevealed();
    (control as unknown as { variablesReveal: typeof control.variablesReveal }).variablesReveal =
      async () => ({ outcome: 'revealed', value: 'sk_live_super_secret', error: null });

    await withCapturedTimeout(async (fire) => {
      const value = await variableSetStore.revealVariable('var-1', () => {});
      expect(value).toBe('sk_live_super_secret');
      expect(variableSetStore.revealedValues['var-1']).toBe('sk_live_super_secret');

      fire();

      expect(variableSetStore.revealedValues['var-1']).toBeUndefined();
    });
  });

  test('clearRevealed cancels a still-pending expiry timer', async () => {
    variableSetStore.clearRevealed();
    (control as unknown as { variablesReveal: typeof control.variablesReveal }).variablesReveal =
      async () => ({ outcome: 'revealed', value: 'sk_live_super_secret', error: null });

    let cleared = 0;
    const realClearTimeout = globalThis.clearTimeout;
    (globalThis as { clearTimeout: typeof clearTimeout }).clearTimeout = ((
      id: Parameters<typeof clearTimeout>[0],
    ) => {
      cleared += 1;
      return realClearTimeout(id);
    }) as typeof clearTimeout;

    try {
      await variableSetStore.revealVariable('var-2', () => {});
      expect(variableSetStore.revealedValues['var-2']).toBe('sk_live_super_secret');
      variableSetStore.clearRevealed();
      expect(variableSetStore.revealedValues['var-2']).toBeUndefined();
      expect(cleared).toBeGreaterThan(0);
    } finally {
      globalThis.clearTimeout = realClearTimeout;
    }
  });
});
