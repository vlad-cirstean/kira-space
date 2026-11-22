import type { PaletteColor } from '@shared/domain/color';
import type { ApiVariable } from '@shared/domain/variables';
import { matchesQuery } from '@workbench/behaviour/match';
import { computed } from 'vue';
import { useApiAgentAccess, useApiConfig } from './apiConfig';
import { useVariableMatrix } from './apiQueries';
import { useCollectionsStore } from './collections';
import { useVariablesStore } from './variables';
import { useVariablesMatrixStore } from './variablesMatrix';

export interface MatrixOwner {
  scope: 'collection' | 'environment';
  id: string;
  name: string;
  file: string;
  color: PaletteColor;
  isActive: boolean;
  /** The file has a problem: its cells are read-only until it is fixed. */
  broken: boolean;
}

/** A row's lock: every holder a secret, none, or a mix (a hand-edited file). */
export type RowLock = 'none' | 'all' | 'mixed';

/** The Variables tab's data: one row per name, one column per owner file. Derived, never stored. */
export function useMatrixModel() {
  const collectionsStore = useCollectionsStore();
  const variablesStore = useVariablesStore();
  const matrix = useVariablesMatrixStore();
  const config = useApiConfig();
  const access = useApiAgentAccess();

  const collections = computed(() => collectionsStore.collections);
  const collection = computed(
    () =>
      collections.value.find((c) => c.id === matrix.collectionId) ?? collections.value[0] ?? null,
  );
  const environments = computed(() => variablesStore.environments);
  const rows = useVariableMatrix(
    () => collection.value?.id ?? '',
    () => environments.value.map((e) => e.id),
  );

  const brokenFiles = computed(
    () => new Set((config.data.value?.files ?? []).filter((f) => f.problem).map((f) => f.file)),
  );

  const defaultOwner = computed<MatrixOwner | null>(() => {
    const c = collection.value;
    return c
      ? {
          scope: 'collection',
          id: c.id,
          name: c.name,
          file: c.file,
          color: 'none',
          isActive: false,
          broken: brokenFiles.value.has(c.file),
        }
      : null;
  });
  const envOwners = computed<MatrixOwner[]>(() =>
    environments.value.map((e) => ({
      scope: 'environment',
      id: e.id,
      name: e.name,
      file: e.file,
      color: e.color,
      isActive: e.isActive,
      broken: brokenFiles.value.has(e.file),
    })),
  );
  const owners = computed(() => [
    ...(defaultOwner.value ? [defaultOwner.value] : []),
    ...envOwners.value,
  ]);
  /** Narrow mode's one column: the picked owner, else the active environment, else the first. */
  const shownOwner = computed<MatrixOwner | null>(
    () =>
      owners.value.find((o) => o.id === matrix.shownOwnerId) ??
      envOwners.value.find((o) => o.isActive) ??
      owners.value[0] ??
      null,
  );

  // First row of a name wins inside one file, matching how a send resolves a duplicate.
  const byOwner = computed(() => {
    const out = new Map<string, Map<string, ApiVariable>>();
    const index = (ownerId: string, list: readonly ApiVariable[]): void => {
      const m = new Map<string, ApiVariable>();
      for (const v of list) if (!m.has(v.name)) m.set(v.name, v);
      out.set(ownerId, m);
    };
    if (defaultOwner.value) index(defaultOwner.value.id, rows.collection.value);
    for (const o of envOwners.value) index(o.id, rows.environments.value[o.id] ?? []);
    return out;
  });

  function variableOf(ownerId: string, name: string): ApiVariable | undefined {
    return byOwner.value.get(ownerId)?.get(name);
  }

  const allNames = computed(() => {
    const seen = new Set<string>();
    for (const o of owners.value)
      for (const name of byOwner.value.get(o.id)?.keys() ?? []) seen.add(name);
    return [...seen];
  });
  const names = computed(() => allNames.value.filter((n) => matchesQuery(n, matrix.filter)));

  /** The owners whose file holds this name. */
  function holders(name: string): MatrixOwner[] {
    return owners.value.filter((o) => variableOf(o.id, name));
  }

  function lockOf(name: string): RowLock {
    const held = holders(name).map((o) => variableOf(o.id, name)?.isSecret === true);
    if (held.length === 0 || held.every((s) => !s)) return 'none';
    return held.every(Boolean) ? 'all' : 'mixed';
  }

  function descriptionOf(name: string): string {
    for (const o of [...envOwners.value, ...(defaultOwner.value ? [defaultOwner.value] : [])]) {
      const d = variableOf(o.id, name)?.description;
      if (d) return d;
    }
    return '';
  }

  const sharedIn = computed(() => {
    const out = new Set<string>();
    for (const env of access.data.value ?? []) {
      for (const v of env.variables) out.add(`${env.id}\u0000${v.name}`);
    }
    return out;
  });
  function isShared(envId: string, name: string): boolean {
    return sharedIn.value.has(`${envId}\u0000${name}`);
  }

  /** Every environment overrides this default. */
  function overriddenEverywhere(name: string): boolean {
    return (
      envOwners.value.length > 0 &&
      !!defaultOwner.value &&
      !!variableOf(defaultOwner.value.id, name) &&
      envOwners.value.every((o) => variableOf(o.id, name))
    );
  }

  /** Cells nobody can resolve: an environment with neither a value nor a default, and a secret
   *  with no saved value. */
  const missingCount = computed(() => {
    let n = 0;
    for (const name of allNames.value) {
      const lock = lockOf(name);
      const base = defaultOwner.value ? variableOf(defaultOwner.value.id, name) : undefined;
      for (const o of envOwners.value) {
        const v = variableOf(o.id, name);
        if (v) {
          if (v.isSecret && !v.secretSet) n++;
        } else if (lock === 'all' || !base) n++;
      }
    }
    return n;
  });

  return {
    collections,
    collection,
    defaultOwner,
    envOwners,
    owners,
    shownOwner,
    names,
    allNames,
    variableOf,
    holders,
    lockOf,
    descriptionOf,
    isShared,
    overriddenEverywhere,
    missingCount,
  };
}

export type MatrixModel = ReturnType<typeof useMatrixModel>;
