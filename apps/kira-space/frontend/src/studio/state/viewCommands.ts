import type { SortSpec } from '@shared/domain/queries';
import { useTabsStore } from './tabs';

// The leaf-registry inversion state/tabRuntime.ts already uses, applied to the six
// project/ -> views/ edges (ProjectTree.vue's four reload imports, menus.ts's runCount/
// runDocumentCount/setFilter/setProjection/setSort). Registration happens at module scope in each
// view's own state.ts, and every one of those modules is reached by a chain of static imports from
// main.ts (main.ts -> App.vue -> WorkbenchShell.vue -> MainView.vue -> each view's *View.vue ->
// ./state), so the registry is populated before the first render — unlike shortcuts/commands.ts's
// deliberate no-op (mount-scoped, legitimately empty), an unregistered kind here can only mean that
// static import chain broke, so the accessors below throw rather than silently doing nothing.
export type CommandTabKind = 'data' | 'document' | 'keyvalue' | 'stream' | 'browse';

const reloaders = new Map<CommandTabKind, (tabId: string) => Promise<void>>();

export function registerTabReload(
  kind: CommandTabKind,
  fn: (tabId: string) => Promise<void>,
): void {
  reloaders.set(kind, fn);
}

/** Fire-and-forget, matching today's `void reload*Tab(id)` call sites exactly. */
export function reloadTab(kind: CommandTabKind, tabId: string): void {
  const fn = reloaders.get(kind);
  if (!fn) throw new Error(`viewCommands: no reload registered for tab kind "${kind}"`);
  void fn(tabId);
}

export type CountableKind = 'data' | 'document';

const counters = new Map<CountableKind, (tabId: string) => Promise<void>>();

// Only the data and document views have a row/document count reachable from the tree menu.
export function registerTabCount(kind: CountableKind, fn: (tabId: string) => Promise<void>): void {
  counters.set(kind, fn);
}

export function countTab(kind: CountableKind, tabId: string): void {
  const fn = counters.get(kind);
  if (!fn) throw new Error(`viewCommands: no count registered for tab kind "${kind}"`);
  void fn(tabId);
}

// The three views/grid/state.ts operations reached from outside the grid module: project/menus.ts's
// saved-filter items, and the column-menu's Add-to-projection / Sort-by items.
export interface DataQueryCommands {
  setFilter(tabId: string, filter: string | null): Promise<void>;
  setSort(tabId: string, sort: SortSpec | null): Promise<void>;
  setProjection(tabId: string, projection: string[] | null): Promise<void>;
}

let dataQueryCmds: DataQueryCommands | null = null;

export function registerDataQueryCommands(cmds: DataQueryCommands): void {
  dataQueryCmds = cmds;
}

export function dataQueryCommands(): DataQueryCommands {
  if (!dataQueryCmds) throw new Error('viewCommands: data query commands were never registered');
  return dataQueryCmds;
}

// An S3 upload can land in a container the project tree no longer renders (a bucket's own
// prefix level, once redis/s3 stop expanding) — UploadObjectDialog.vue can no longer reach
// for project/state/tree.ts's refresh() (project/ must not import views/), so it calls this
// instead. The Browse view owns that level's cache now.
let browseInvalidateFn: ((connectionId: string, path: string) => Promise<void>) | null = null;

export function registerBrowseInvalidate(
  fn: (connectionId: string, path: string) => Promise<void>,
): void {
  browseInvalidateFn = fn;
}

/** Fire-and-forget, matching UploadObjectDialog.vue's own prior `await refresh(...)` call site —
 *  a no-op (not a throw) when no Browse tab has ever mounted, unlike reloadTab/countTab above,
 *  since a Browse tab visiting the uploaded-into container is not guaranteed the way every tab
 *  kind's own state.ts module is guaranteed to load. */
export function browseInvalidate(connectionId: string, path: string): void {
  void browseInvalidateFn?.(connectionId, path);
}

// The "L2 is invalidated by any local mutation on the same target" is kept by the engine
// (engine/data.ts's cache.invalidateAfterMutation), but the renderer's own per-tab page
// stores are not — a second tab open on the same (connectionId, path) kept rendering rows a
// sibling tab had already deleted or edited, with no user action able to notice. Skips the tab
// that performed the mutation (its own caller already reloads it, with the pages-only invalidate
// scope — a second reload here would double it) and every tab still behind the
// reconnect gate (it has no page to correct, and will load fresh when pressed). Only the
// four kinds a mutation ever targets — a Browse tab's path is a container, never a mutation's own
// (connectionId, path).
export function reloadTabsForTarget(connectionId: string, path: string, exceptTabId: string): void {
  const tabsStore = useTabsStore();
  for (const tab of tabsStore.tabs) {
    if (tab.id === exceptTabId) continue;
    if (tab.connectionId !== connectionId || tab.path !== path) continue;
    if (!tabsStore.isHydrated(tab.id)) continue;
    switch (tab.kind) {
      case 'data':
      case 'document':
      case 'keyvalue':
      case 'stream':
        reloadTab(tab.kind, tab.id);
        break;
      default:
        break;
    }
  }
}
