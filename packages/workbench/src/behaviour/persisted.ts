import { StorageSerializers } from '@vueuse/core';
import { effect, effectScope, type Ref, ref } from 'vue';

/** Structural zod schema: workbench takes no zod dependency. */
export interface Schema<T> {
  safeParse(value: unknown): { success: true; data: T } | { success: false };
}

// One shared ref per key: two components binding the same key see the same value. The write effect
// lives in a detached scope so the first component to unmount does not stop it for the others.
// VueUse's storage composables are not used: they build a runtime-core `watch`, which captures the
// calling component instance and pins it for the app's lifetime. `effect` captures none.
interface Entry {
  state: Ref<unknown>;
  read(raw: string): unknown;
  defaultValue: unknown;
}
const cache = new Map<string, Entry>();
const scope = effectScope(true);
let listening = false;

function serializerFor<T>(defaultValue: T, schema: Schema<T> | undefined) {
  const type =
    defaultValue === null || typeof defaultValue === 'object'
      ? 'object'
      : (typeof defaultValue as 'boolean' | 'number' | 'string');
  const base = StorageSerializers[type] as {
    read(raw: string): unknown;
    write(value: unknown): string;
  };
  return {
    read(raw: string): T {
      let value: unknown;
      try {
        value = base.read(raw);
      } catch {
        return defaultValue;
      }
      if (!schema) return value as T;
      const parsed = schema.safeParse(value);
      return parsed.success ? parsed.data : defaultValue;
    },
    write: (value: T): string => base.write(value),
  };
}

function readStored<T>(key: string, read: (raw: string) => T, defaultValue: T): T {
  try {
    const raw = localStorage.getItem(key);
    return raw === null ? defaultValue : read(raw);
  } catch {
    return defaultValue;
  }
}

function writeStored(key: string, value: unknown, write: (v: never) => string): void {
  try {
    if (value === null || value === undefined) {
      localStorage.removeItem(key);
      return;
    }
    const raw = write(value as never);
    if (localStorage.getItem(key) !== raw) localStorage.setItem(key, raw);
  } catch {
    // Storage blocked or full: the in-memory value stands.
  }
}

function listenForRemoteChanges(): void {
  if (listening || typeof window === 'undefined') return;
  listening = true;
  window.addEventListener('storage', (e) => {
    if (e.storageArea !== localStorage || e.key === null) return;
    const entry = cache.get(e.key);
    if (!entry) return;
    // The echo write is skipped in `writeStored`: storage already holds this value.
    entry.state.value = e.newValue === null ? entry.defaultValue : entry.read(e.newValue);
  });
}

/**
 * Per-device UI state in `localStorage` under `kira.<module>.<name>` (each app and the phone UI is
 * its own origin, so no app segment). A stored value that fails `schema` reads as the default.
 * Geometry that follows the user
 * across windows belongs in the Go layout store and per-tab view state in the tab's `state_json`,
 * not here.
 */
export function usePersistedState<T>(
  key: string,
  defaultValue: T,
  opts: { schema?: Schema<T> } = {},
): Ref<T> {
  const cached = cache.get(key);
  if (cached) return cached.state as Ref<T>;
  const serializer = serializerFor(defaultValue, opts.schema);
  const state = ref(readStored(key, serializer.read, defaultValue)) as Ref<T>;
  const entry: Entry = { state, read: serializer.read, defaultValue };
  cache.set(key, entry);
  scope.run(() => {
    let queued = false;
    // The initial run only tracks: defaults are never persisted.
    const runner = effect(() => serializer.write(state.value as T), {
      scheduler: () => {
        if (queued) return;
        queued = true;
        queueMicrotask(() => {
          queued = false;
          // Re-run to re-track nested properties, then write the fresh value.
          runner();
          writeStored(key, state.value, serializer.write);
        });
      },
    });
  });
  listenForRemoteChanges();
  return state;
}
