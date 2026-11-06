// api-variables-reveal-grace-expiry.spec.ts pinned that `revealedValues` re-masks itself at the
// grace window. Two sibling maps did not:
// `revealedHistoryValues` (a prior secret value revealed from the history popover) and
// `copyAsCurlDialogState.revealedSecretValues` (the fully-substituted Copy-as-curl command, the
// more serious of the two — the dialog can sit open on screen indefinitely with a real credential
// inline). Both were cleared only by their own dialog/popover's close path, never by expiry. This
// test fails against the pre-fix code (neither map ever schedules a timer) and passes once both
// share the same createRevealExpiry discipline `revealedValues` already had.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/state/pinia';

setActivePinia(pinia);

const { control } = await import('../../../frontend/src/studio/bridge/control');
restoreAfterEach(control);
const { useVariableSetStore } = await import('../../../frontend/src/studio/api/state/variables');
const { useCopyAsCurlStore } = await import('../../../frontend/src/studio/api/state/curl');
const { loadVariableRows } = await import('../../../frontend/src/studio/api/state/apiQueries');

const variableSetStore = useVariableSetStore();
const copyAsCurlStore = useCopyAsCurlStore();

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

describe('revealHistoryEntry grace-window expiry (round 2 finding 5)', () => {
  test('a revealed history value is scheduled for re-masking at the auth grace, not only on popover close', async () => {
    variableSetStore.closeHistoryMenu();
    (
      control as unknown as { variablesRevealHistory: typeof control.variablesRevealHistory }
    ).variablesRevealHistory = async () => ({
      outcome: 'revealed',
      value: 'old_sk_live_secret',
      error: null,
    });
    (control as unknown as { variablesHistory: typeof control.variablesHistory }).variablesHistory =
      async () => [];

    await variableSetStore.openHistoryMenu('tab-1', 'environment', 'env-1', 'var-1');

    await withCapturedTimeout(async (fire) => {
      const value = await variableSetStore.revealHistoryEntry('hist-1');
      expect(value).toBe('old_sk_live_secret');
      expect(variableSetStore.revealedHistoryValues['hist-1']).toBe('old_sk_live_secret');

      fire();

      expect(variableSetStore.revealedHistoryValues['hist-1']).toBeUndefined();
    });
  });
});

describe('Copy-as-curl revealed secret grace-window expiry (round 2 finding 5)', () => {
  test('the rendered command re-masks itself once the reveal grace elapses, without the dialog closing', async () => {
    (control as unknown as { variablesList: typeof control.variablesList }).variablesList =
      async () => [
        {
          id: 'secret-var-1',
          scope: 'environment',
          ownerId: 'env-curl-1',
          name: 'apiToken',
          value: '',
          isSecret: true,
          sortOrder: 0,
          description: '',
        },
      ];
    (control as unknown as { variablesReveal: typeof control.variablesReveal }).variablesReveal =
      async () => ({ outcome: 'revealed', value: 'sk_live_curl_secret', error: null });

    // P112: ensureVariablesLoaded is gone — loadVariableRows (cache-first) seeds the same row.
    await loadVariableRows('environment', 'env-curl-1');

    copyAsCurlStore.openCopyAsCurlDialog(
      'GET',
      {
        url: 'https://api.example.com/{{apiToken}}',
        headers: [],
        body: {
          mode: 'none',
          raw: '',
          code: '',
          codeLanguage: 'json',
          urlEncoded: [],
          formData: [],
          file: '',
        },
        refs: [],
      },
      ['apiToken'],
      '',
      '',
      'env-curl-1',
    );

    await withCapturedTimeout(async (fire) => {
      await copyAsCurlStore.revealSecretValues();
      expect(copyAsCurlStore.revealedSecretValues.apiToken).toBe('sk_live_curl_secret');
      expect(copyAsCurlStore.currentCurlCommand()).toContain('sk_live_curl_secret');

      fire();

      expect(copyAsCurlStore.revealedSecretValues.apiToken).toBeUndefined();
      expect(copyAsCurlStore.currentCurlCommand()).not.toContain('sk_live_curl_secret');
      expect(copyAsCurlStore.currentCurlCommand()).toContain('{{apiToken}}');
    });
  });
});
