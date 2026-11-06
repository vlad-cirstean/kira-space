import type { PageSize } from '@shared/domain/tabs';
import { useTabsStore } from '../../../state/tabs';

// The seam that lets views/shared/keyvalue/ (state.ts, mutations.ts, KeyValuePane.vue)
// address either a real KeyValue tab or the browse split's preview pane through one shape, instead
// of reaching into state/tabs.ts's tab-only findKeyValueTab/patchKeyValueTabState directly. A real
// tab never registers explicitly — resolveTabHost below is keyValueHost's own fallback, so every
// existing KeyValue tab keeps behaving exactly as it did before this seam existed. Only a
// non-tab viewKey (BrowseView.vue's own `${tab.id}::preview`) registers one explicitly, backed by
// its own runtime fields (browse/state.ts).
export interface KeyValueHost {
  /** Tab whose reconnect gate and stale stamp govern this view: itself, or the owning browse tab. */
  gateTabId: string;
  connectionId: string | null;
  path: string;
  pageIndex: number;
  pageSize: PageSize;
  patch(p: { pageIndex?: number; pageSize?: PageSize }): void;
}

const registry = new Map<string, () => KeyValueHost | null>();

export function registerKeyValueHost(viewKey: string, host: () => KeyValueHost | null): void {
  registry.set(viewKey, host);
}

export function unregisterKeyValueHost(viewKey: string): void {
  registry.delete(viewKey);
}

// The tab-backed fallback: any viewKey that resolves to a real KeyValueTabRecord (viewKey === the
// tab's own id, the common case) is served from that tab's own state, with no explicit
// registration required — "registered once, centrally" means this function, not a
// per-tab call into registerKeyValueHost.
function resolveTabHost(viewKey: string): KeyValueHost | null {
  const tabsStore = useTabsStore();
  const tab = tabsStore.findKeyValueTab(viewKey);
  if (!tab) return null;
  return {
    gateTabId: viewKey,
    connectionId: tab.connectionId,
    path: tab.path,
    pageIndex: tab.state.pageIndex,
    pageSize: tab.state.pageSize,
    patch: (p) => tabsStore.patchKeyValueTabState(viewKey, p),
  };
}

export function keyValueHost(viewKey: string): KeyValueHost | null {
  return registry.get(viewKey)?.() ?? resolveTabHost(viewKey);
}
