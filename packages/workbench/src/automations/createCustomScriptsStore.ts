import type {
  CustomScript,
  CustomScriptFields,
  ScriptCollection,
  ScriptsSnapshot,
  SecretValues,
} from '@shared/domain/scripts';
import type { SecretStorageStatus } from '@shared/domain/secrets';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { hydrateThenSubscribe } from '../state/hydrateThenSubscribe';

// The script store both apps define from their own `control` calls (createKeepAwakeStore's
// precedent). A snapshot broadcast replaces `collections` and `records` wholesale, so a command added in one
// window shows up in every other.
export interface CustomScriptsControl {
  customScriptsList(): Promise<ScriptsSnapshot>;
  customScriptsCreate(
    fields: CustomScriptFields,
    secretValues: SecretValues | null,
  ): Promise<CustomScript>;
  customScriptsUpdate(
    id: string,
    fields: CustomScriptFields,
    secretValues: SecretValues | null,
  ): Promise<CustomScript>;
  customScriptsDelete(id: string): Promise<void>;
  customScriptsCreateCollection(name: string): Promise<ScriptCollection>;
  customScriptsRenameCollection(id: string, name: string): Promise<void>;
  customScriptsDeleteCollection(id: string): Promise<void>;
  customScriptsMove(
    id: string,
    collectionId: string | null,
    beforeId?: string | null,
  ): Promise<void>;
  /** collectionId null is the ungrouped set. */
  customScriptsReorder(collectionId: string | null, ids: string[]): Promise<void>;
  customScriptsReorderCollections(ids: string[]): Promise<void>;
  onCustomScriptsChanged(cb: (snapshot: ScriptsSnapshot) => void): () => void;
}

const STORAGE_OFF: SecretStorageStatus = {
  available: false,
  backend: 'unavailable',
  insecureFallback: false,
  reason: null,
};

/** A Go nil slice crosses the bridge as null; coerce the arrays so the store always holds arrays. */
export function scriptsSnapshotOf(raw: unknown): ScriptsSnapshot {
  const r = (raw ?? {}) as Partial<ScriptsSnapshot>;
  return {
    collections: r.collections ?? [],
    scripts: (r.scripts ?? []).map((s) => ({ ...s, savedSecrets: s.savedSecrets ?? [] })),
    secretStorage: r.secretStorage ?? STORAGE_OFF,
  };
}

/** Puts the listed items in `ids` order at the positions the listed items already hold. */
function placeOrdered<T extends { id: string }>(list: readonly T[], ids: readonly string[]): T[] {
  const byId = new Map(list.map((x) => [x.id, x]));
  const queue = ids.map((id) => byId.get(id)).filter((x): x is T => x !== undefined);
  const listed = new Set(queue.map((x) => x.id));
  let next = 0;
  return list.map((x) => (listed.has(x.id) ? queue[next++] : x));
}

export function createCustomScriptsStore(control: CustomScriptsControl) {
  return defineStore('customScripts', () => {
    const state = reactive({
      collections: [] as ScriptCollection[],
      records: [] as CustomScript[],
      secretStorage: STORAGE_OFF,
    });

    let unsubscribe: (() => void) | null = null;

    async function hydrateCustomScripts(): Promise<void> {
      unsubscribe?.();
      unsubscribe = null;
      unsubscribe = await hydrateThenSubscribe({
        snapshot: () => control.customScriptsList(),
        subscribe: (cb) => control.onCustomScriptsChanged(cb),
        apply: (snapshot) => {
          state.collections = snapshot.collections;
          state.records = snapshot.scripts;
          state.secretStorage = snapshot.secretStorage;
        },
      });
    }

    async function createCustomScript(
      fields: CustomScriptFields,
      secretValues: SecretValues | null = null,
    ): Promise<void> {
      await control.customScriptsCreate(fields, secretValues);
    }

    async function updateCustomScript(
      id: string,
      fields: CustomScriptFields,
      secretValues: SecretValues | null = null,
    ): Promise<void> {
      await control.customScriptsUpdate(id, fields, secretValues);
    }

    async function removeCustomScript(id: string): Promise<void> {
      await control.customScriptsDelete(id);
    }

    function createCollection(name: string): Promise<ScriptCollection> {
      return control.customScriptsCreateCollection(name);
    }

    async function renameCollection(id: string, name: string): Promise<void> {
      await control.customScriptsRenameCollection(id, name);
    }

    async function removeCollection(id: string): Promise<void> {
      await control.customScriptsDeleteCollection(id);
    }

    async function moveScript(
      id: string,
      collectionId: string | null,
      beforeId: string | null = null,
    ): Promise<void> {
      await control.customScriptsMove(id, collectionId, beforeId);
    }

    async function resync(): Promise<void> {
      const snapshot = await control.customScriptsList();
      state.collections = snapshot.collections;
      state.records = snapshot.scripts;
      state.secretStorage = snapshot.secretStorage;
    }

    async function reorderScripts(collectionId: string | null, ids: string[]): Promise<void> {
      state.records = placeOrdered(state.records, ids);
      try {
        await control.customScriptsReorder(collectionId, ids);
      } catch (err) {
        await resync();
        throw err;
      }
    }

    async function reorderCollections(ids: string[]): Promise<void> {
      state.collections = placeOrdered(state.collections, ids);
      try {
        await control.customScriptsReorderCollections(ids);
      } catch (err) {
        await resync();
        throw err;
      }
    }

    return {
      ...toRefs(state),
      hydrateCustomScripts,
      createCustomScript,
      updateCustomScript,
      removeCustomScript,
      createCollection,
      renameCollection,
      removeCollection,
      moveScript,
      reorderScripts,
      reorderCollections,
    };
  });
}
