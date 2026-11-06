// P168 Part 10 F1: closing or reopening the Copy-as-curl dialog must stop an in-flight reveal loop.
import '@workbench/testing/unit/window';

import { expect, test } from 'bun:test';
import { deferred } from '@workbench/testing/unit/async';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/state/pinia';

setActivePinia(pinia);

const { control } = await import('../../../frontend/src/studio/bridge/control');
restoreAfterEach(control);
const { useCopyAsCurlStore } = await import('../../../frontend/src/studio/api/state/curl');
const { useVariableSetStore } = await import('../../../frontend/src/studio/api/state/variables');

const store = useCopyAsCurlStore();

const resolved = {
  url: 'https://api.example.com/{{A}}/{{B}}',
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
} as unknown as Parameters<typeof store.openCopyAsCurlDialog>[1];

test('close during a pending reveal stops the loop and leaves no plaintext behind', async () => {
  const secret = (name: string) => ({
    id: `v${name}`,
    scope: 'environment' as const,
    ownerId: 'env-cancel',
    name,
    value: '',
    isSecret: true,
    sortOrder: 0,
    description: '',
  });
  (control as unknown as { variablesList: typeof control.variablesList }).variablesList =
    async () => [secret('A'), secret('B')];
  const first = deferred<{ outcome: 'revealed'; value: string; error: null }>();
  const calls: string[] = [];
  (control as unknown as { variablesReveal: typeof control.variablesReveal }).variablesReveal = (
    id: string,
  ) => {
    calls.push(id);
    return first.promise;
  };

  store.openCopyAsCurlDialog('GET', resolved, ['A', 'B'], '', '', 'env-cancel');
  const loop = store.revealSecretValues();
  await new Promise((r) => setTimeout(r, 0));
  expect(calls).toEqual(['vA']);

  store.closeCopyAsCurlDialog();
  first.resolve({ outcome: 'revealed', value: 'plain-A', error: null });
  await loop;

  expect(calls).toEqual(['vA']);
  expect(store.revealedSecretValues).toEqual({});
  expect(useVariableSetStore().revealedValues).toEqual({});
});
