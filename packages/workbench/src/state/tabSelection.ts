import { defineStore } from 'pinia';
import { ref } from 'vue';

interface Selection {
  ids: string[];
  anchor: string | null;
}

const EMPTY: readonly string[] = [];

/** Which tabs of a strip are selected (P727), keyed by `TabStripHost.selectionScope`. Separate from
 *  the active tab: a Cmd/Ctrl-click toggles, a Shift-click selects a range, a plain click clears. */
export const useTabSelectionStore = defineStore('tabSelection', () => {
  const byScope = ref<Record<string, Selection>>({});

  function selected(scope: string): readonly string[] {
    return byScope.value[scope]?.ids ?? EMPTY;
  }

  function isSelected(scope: string, id: string): boolean {
    return selected(scope).includes(id);
  }

  /** The tab is in the set and the set holds two or more tabs. */
  function isMulti(scope: string, id: string): boolean {
    const ids = selected(scope);
    return ids.length >= 2 && ids.includes(id);
  }

  /** Toggles id. The first toggle on an empty set seeds it with the active tab too. */
  function toggle(scope: string, id: string, activeId: string | null): void {
    const current = selected(scope);
    let ids: string[];
    if (current.length === 0) {
      ids = activeId && activeId !== id ? [activeId, id] : [id];
    } else {
      ids = current.includes(id) ? current.filter((x) => x !== id) : [...current, id];
    }
    byScope.value[scope] = { ids, anchor: id };
  }

  /** Replaces the set with the strip-order range from the anchor (else the active tab) to id. */
  function range(
    scope: string,
    orderedIds: readonly string[],
    id: string,
    activeId: string | null,
  ): void {
    const anchor = byScope.value[scope]?.anchor ?? activeId ?? id;
    const from = orderedIds.indexOf(anchor);
    const to = orderedIds.indexOf(id);
    if (to < 0) return;
    const lo = Math.min(from < 0 ? to : from, to);
    const hi = Math.max(from < 0 ? to : from, to);
    byScope.value[scope] = { ids: orderedIds.slice(lo, hi + 1), anchor: anchor };
  }

  /** Empties the set; `anchor` is where the next Shift-click range starts. */
  function clear(scope: string, anchor: string | null = null): void {
    if (selected(scope).length === 0 && !anchor && !byScope.value[scope]?.anchor) return;
    byScope.value[scope] = { ids: [], anchor };
  }

  /** Drops ids no longer in the strip. */
  function prune(scope: string, liveIds: readonly string[]): void {
    const current = byScope.value[scope];
    if (!current) return;
    const live = new Set(liveIds);
    const ids = current.ids.filter((id) => live.has(id));
    if (ids.length === current.ids.length) return;
    byScope.value[scope] = {
      ids,
      anchor: current.anchor && live.has(current.anchor) ? current.anchor : null,
    };
  }

  return { selected, isSelected, isMulti, toggle, range, clear, prune };
});
