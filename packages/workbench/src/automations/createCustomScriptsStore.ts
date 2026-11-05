import type {
  CustomScript,
  CustomScriptFields,
  RunFormResult,
  ScriptCollection,
  ScriptFileProblem,
  ScriptFileView,
  ScriptsSnapshot,
  SecretValues,
} from '@shared/domain/scripts';
import type { SecretStorageStatus } from '@shared/domain/secrets';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import type { FormEdit } from '../config/configForm';
import { hydrateThenSubscribe } from '../state/hydrateThenSubscribe';

// The script store both apps define from their own `control` calls (createKeepAwakeStore's
// precedent). The run files are the source: a snapshot broadcast replaces the state wholesale, so a
// file edited in an editor shows up in every window.
export interface CustomScriptsControl {
  customScriptsList(): Promise<ScriptsSnapshot>;
  customScriptsCreate(
    fields: CustomScriptFields,
    secretValues: SecretValues | null,
  ): Promise<CustomScript>;
  customScriptsDelete(id: string): Promise<void>;
  customScriptsFile(file: string): Promise<ScriptFileView>;
  customScriptsSaveFile(file: string, text: string, baseHash: string): Promise<ScriptFileView>;
  customScriptsForm(file: string, text: string, edits: FormEdit[]): Promise<RunFormResult>;
  customScriptsSetSecrets(id: string, values: SecretValues): Promise<CustomScript>;
  /** Kira Space only. */
  customScriptsSetScheduleTarget?(
    id: string,
    taskId: string,
    branchId: string,
  ): Promise<CustomScript>;
  /** collection '' is the ungrouped set. */
  customScriptsReorder(collection: string, ids: string[]): Promise<void>;
  customScriptsReorderCollections(names: string[]): Promise<void>;
  /** collection '' ungroups; `beforeId` null appends to the target set. */
  customScriptsMove(id: string, collection: string, beforeId: string | null): Promise<void>;
  customScriptsRenameCollection(name: string, newName: string): Promise<void>;
  customScriptsDeleteCollection(name: string): Promise<void>;
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
    dir: r.dir ?? '',
    collections: r.collections ?? [],
    problems: r.problems ?? [],
    scripts: (r.scripts ?? []).map((s) => ({ ...s, savedSecrets: s.savedSecrets ?? [] })),
    secretStorage: r.secretStorage ?? STORAGE_OFF,
  };
}

/** Puts the listed items in `ids` order at the positions the listed items already hold. */
function placeOrdered<T>(
  list: readonly T[],
  keys: readonly string[],
  keyOf: (x: T) => string,
): T[] {
  const byKey = new Map(list.map((x) => [keyOf(x), x]));
  const queue = keys.map((k) => byKey.get(k)).filter((x): x is T => x !== undefined);
  const listed = new Set(queue.map(keyOf));
  let next = 0;
  return list.map((x) => (listed.has(keyOf(x)) ? queue[next++] : x));
}

export function createCustomScriptsStore(control: CustomScriptsControl) {
  return defineStore('customScripts', () => {
    const state = reactive({
      dir: '',
      collections: [] as ScriptCollection[],
      records: [] as CustomScript[],
      problems: [] as ScriptFileProblem[],
      secretStorage: STORAGE_OFF,
    });

    let unsubscribe: (() => void) | null = null;

    function apply(snapshot: ScriptsSnapshot): void {
      state.dir = snapshot.dir;
      state.collections = snapshot.collections;
      state.records = snapshot.scripts;
      state.problems = snapshot.problems;
      state.secretStorage = snapshot.secretStorage;
    }

    async function hydrateCustomScripts(): Promise<void> {
      unsubscribe?.();
      unsubscribe = null;
      unsubscribe = await hydrateThenSubscribe({
        snapshot: () => control.customScriptsList(),
        subscribe: (cb) => control.onCustomScriptsChanged(cb),
        apply,
      });
    }

    function createCustomScript(fields: CustomScriptFields): Promise<CustomScript> {
      return control.customScriptsCreate(fields, null);
    }

    async function removeCustomScript(id: string): Promise<void> {
      await control.customScriptsDelete(id);
    }

    function readFile(file: string): Promise<ScriptFileView> {
      return control.customScriptsFile(file);
    }

    function saveFile(file: string, text: string, baseHash: string): Promise<ScriptFileView> {
      return control.customScriptsSaveFile(file, text, baseHash);
    }

    function form(file: string, text: string, edits: FormEdit[]): Promise<RunFormResult> {
      return control.customScriptsForm(file, text, edits);
    }

    async function setSecrets(id: string, values: SecretValues): Promise<void> {
      await control.customScriptsSetSecrets(id, values);
    }

    async function setScheduleTarget(id: string, taskId: string, branchId: string): Promise<void> {
      await control.customScriptsSetScheduleTarget?.(id, taskId, branchId);
    }

    async function resync(): Promise<void> {
      apply(await control.customScriptsList());
    }

    async function reorderScripts(collection: string, ids: string[]): Promise<void> {
      state.records = placeOrdered(state.records, ids, (s) => s.id);
      try {
        await control.customScriptsReorder(collection, ids);
      } catch (err) {
        await resync();
        throw err;
      }
    }

    async function reorderCollections(names: string[]): Promise<void> {
      state.collections = placeOrdered(state.collections, names, (c) => c.name);
      try {
        await control.customScriptsReorderCollections(names);
      } catch (err) {
        await resync();
        throw err;
      }
    }

    async function move(id: string, collection: string, beforeId: string | null): Promise<void> {
      await control.customScriptsMove(id, collection, beforeId);
    }

    async function renameCollection(name: string, newName: string): Promise<void> {
      await control.customScriptsRenameCollection(name, newName);
    }

    async function removeCollection(name: string): Promise<void> {
      await control.customScriptsDeleteCollection(name);
    }

    return {
      ...toRefs(state),
      hydrateCustomScripts,
      createCustomScript,
      removeCustomScript,
      readFile,
      saveFile,
      form,
      setSecrets,
      setScheduleTarget,
      reorderScripts,
      reorderCollections,
      move,
      renameCollection,
      removeCollection,
    };
  });
}
