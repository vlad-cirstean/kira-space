import { t } from '@theme/i18n';
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import { announceApp } from '@workbench/behaviour/liveRegion';
import type { MenuItem } from '@workbench/state/contextMenu';
import { useModeStore } from '../../state/mode';
import { useTabsStore } from '../../state/tabs';
import { dataQueryCommands } from '../../state/viewCommands';

// D9: the definition view's Columns section reuses the tree's former column-row menu items, but it
// already has the table's own path directly (`tab.path`) — no `pathParent()` needed the way the
// tree's former column rows required.
function targetTabForTable(connectionId: string, tablePath: string): string | null {
  const active = useModeStore().activeTab;
  // This menu lives only in the Definition view's Columns section, so `active` here is
  // always that table's *definition* tab, never its data tab — matching on connectionId/path
  // alone would reuse the definition tab itself and silently no-op the projection/sort patch
  // (patchDataTabState only writes to a 'data'-kind record).
  if (
    active &&
    active.kind === 'data' &&
    active.connectionId === connectionId &&
    active.path === tablePath
  ) {
    return active.id;
  }
  return useTabsStore().openDataTab(connectionId, tablePath)?.id ?? null;
}

// The pure half of "Add to projection". `currentProjection === null` means "every
// column shown" (ResolveProjection's own meaning for it) — the named column is already in it, so
// there is nothing to do; a `null` return here means exactly that ("no update needed"), never "set
// the projection to null" — the caller below never forwards it to setProjection. The old
// `current ? [...current, columnName] : [columnName]` treated a null projection the same as an
// empty one and replaced "everything" with just this one column, the exact opposite of what "Add"
// says and reading as data loss ("my grid lost all its columns").
export function nextProjectionAfterAddingColumn(
  currentProjection: string[] | null,
  columnName: string,
): string[] | null {
  if (currentProjection === null || currentProjection.includes(columnName)) return null;
  return [...currentProjection, columnName];
}

export function columnsSectionMenu(
  connectionId: string,
  tablePath: string,
  columnName: string,
  onCopyError: (message: string) => void,
): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'copy-name',
      label: t('common.action.copyName.label'),
      icon: 'copy',
      run: () =>
        copyOrReportError(columnName, onCopyError, () =>
          announceApp(t('common.copy.copiedName.announce')),
        ),
    },
    {
      type: 'item',
      id: 'add-to-projection',
      label: t('studio.definition.menu.addToProjection.label'),
      icon: 'list-selection',
      run: () => {
        const tabId = targetTabForTable(connectionId, tablePath);
        if (tabId === null) return;
        const tab = useTabsStore().findDataTab(tabId);
        const next = nextProjectionAfterAddingColumn(tab?.state.projection ?? null, columnName);
        if (next === null) return;
        void dataQueryCommands().setProjection(tabId, next);
      },
    },
    {
      type: 'item',
      id: 'sort-by',
      label: t('studio.definition.menu.sortBy.label'),
      icon: 'sort-precedence',
      run: () => {
        const tabId = targetTabForTable(connectionId, tablePath);
        if (tabId === null) return;
        void dataQueryCommands().setSort(tabId, {
          kind: 'structured',
          terms: [{ column: columnName, direction: 'asc' }],
        });
      },
    },
  ];
}
