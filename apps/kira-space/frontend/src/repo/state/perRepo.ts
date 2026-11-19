import { reactive } from 'vue';

/**
 * "One entry per open repo workspace", get-or-create, the same reactive-`Map` pattern
 * `worktrees.ts` and the task Files tab's search state each implemented on their own: the Map itself is
 * wrapped in `reactive()`, not just each value, so a computed/template that reads `stateFor(id)`
 * (or `all.get(id)`) before an entry exists still reruns once one is later created — a plain Map
 * makes that first `.get()` untracked (Vue 3's native Map/Set reactivity needs the Map itself to
 * be the reactive object).
 *
 * Not a Pinia store itself — CLAUDE.md's "one store, one concern" still holds. Each of the three
 * stores above composes this helper for its own per-repo state shape rather than sharing one
 * grab-bag store, and keeps its own `drop*`/`collapse*` wrapper where dropping an entry needs
 * more than the Map deletion (`search.ts`'s own `repoBySearchId` cleanup, `worktrees.ts`'s own
 * `release`).
 */
export function createPerRepoState<T extends object>(
  init: () => T,
): {
  stateFor(repoId: string): T;
  drop(repoId: string): void;
  readonly all: ReadonlyMap<string, T>;
} {
  // Vue's UnwrapNestedRefs doesn't resolve cleanly through a generic T (unlike the three concrete
  // callers this was hoisted from) — cast once here rather than at every call site.
  const byRepo = reactive(new Map<string, T>()) as Map<string, T>;

  return {
    stateFor(repoId: string): T {
      let state = byRepo.get(repoId);
      if (!state) {
        state = reactive(init()) as T;
        byRepo.set(repoId, state);
      }
      return state;
    },
    drop(repoId: string): void {
      byRepo.delete(repoId);
    },
    get all(): ReadonlyMap<string, T> {
      return byRepo;
    },
  };
}
