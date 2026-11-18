import { defineStore } from 'pinia';
import { reactive } from 'vue';
import { registerTabRuntimeCleanup } from './tabRuntime';

export const VIEW_STATE_MAX = 400;

export interface KeptScroll {
  top: number;
  left: number;
}

export function tabViewKey(tabId: string, surface: string): string {
  return `tab:${tabId}:${surface}`;
}

// View state of unmounted surfaces (P660): survives a module or tab switch, never a restart.
export const useViewStateStore = defineStore('viewState', () => {
  const values = reactive<Record<string, unknown>>({});
  // Non-reactive: nothing renders from scroll, a scroll event writes two numbers.
  const scrolls = new Map<string, KeptScroll>();
  const lru = new Map<string, true>();

  function touch(key: string): void {
    lru.delete(key);
    lru.set(key, true);
    if (lru.size <= VIEW_STATE_MAX) return;
    const oldest = lru.keys().next().value;
    if (oldest === undefined) return;
    lru.delete(oldest);
    delete values[oldest];
    scrolls.delete(oldest);
  }

  function read<T>(key: string, fallback: T): T {
    return key in values ? (values[key] as T) : fallback;
  }

  function write(key: string, value: unknown): void {
    values[key] = value;
    touch(key);
  }

  function scroll(key: string): KeptScroll | undefined {
    return scrolls.get(key);
  }

  function setScroll(key: string, pos: KeptScroll): void {
    scrolls.set(key, pos);
    touch(key);
  }

  function drop(prefix: string): void {
    for (const key of Object.keys(values)) if (key.startsWith(prefix)) delete values[key];
    for (const key of [...scrolls.keys()]) if (key.startsWith(prefix)) scrolls.delete(key);
    for (const key of [...lru.keys()]) if (key.startsWith(prefix)) lru.delete(key);
  }

  return { read, write, scroll, setScroll, drop };
});

registerTabRuntimeCleanup((tabId) => useViewStateStore().drop(tabViewKey(tabId, '')));
