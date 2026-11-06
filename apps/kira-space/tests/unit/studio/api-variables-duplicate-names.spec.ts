// Three separate code paths each resolve a duplicate variable name within one scope, and disagreed
// on which one wins. The documented rule is first-wins by sort_order — isDuplicateName
// (VariablesDialog.vue's own chip) and findSecretVariableId (curl.ts) already matched it;
// mergedValuesAndSecrets (this file) used to overwrite on every iteration (last-wins). This pins
// the fix: given two same-named variables, the one with the lower sort_order — the one
// control.variablesList returns first — is the value mergeVariableRows reports, exactly what a live
// send or a copied curl command uses.
//
// mergeVariableRows is a pure function over two already-loaded row arrays (no store, no
// cache) — this spec calls it directly rather than going through ensureVariablesLoaded/a store.
import { describe, expect, test } from 'bun:test';
import type { ApiVariable } from '@shared/domain/variables';
import { mergeVariableRows } from '../../../frontend/src/studio/api/state/variables';

function variable(overrides: Partial<ApiVariable>): ApiVariable {
  return {
    id: 'id',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'token',
    value: 'value',
    isSecret: false,
    sortOrder: 0,
    description: '',
    ...overrides,
  };
}

describe('mergeVariableRows duplicate-name resolution (D12)', () => {
  test('a duplicate name within one scope resolves first-wins by sort_order', () => {
    const collectionRows = [
      variable({
        id: 'v1',
        ownerId: 'col-dup-1',
        name: 'token',
        value: 'first-value',
        sortOrder: 0,
      }),
      variable({
        id: 'v2',
        ownerId: 'col-dup-1',
        name: 'token',
        value: 'second-value',
        sortOrder: 1,
      }),
    ];

    const { values } = mergeVariableRows(collectionRows, []);

    expect(values.token).toBe('first-value');
  });

  test('environment still overrides collection for the same name, despite within-scope first-wins', () => {
    const collectionRows = [
      variable({ id: 'c1', ownerId: 'col-dup-2', name: 'token', value: 'collection-value' }),
    ];
    const environmentRows = [
      variable({
        id: 'e1',
        scope: 'environment',
        ownerId: 'env-dup-2',
        name: 'token',
        value: 'env-value',
      }),
    ];

    const { values } = mergeVariableRows(collectionRows, environmentRows);

    expect(values.token).toBe('env-value');
  });
});
