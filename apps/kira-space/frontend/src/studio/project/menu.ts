import type { ConnectionSummary } from '@shared/domain/connection';
import { AWS_STYLE_KINDS, FILE_KINDS } from '@shared/domain/connection';
import { decodePath } from '@shared/domain/tree';
import { formatConnectionUri } from '@shared/domain/uri';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { copyAndAnnounce } from '@workbench/behaviour/clipboard';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import type { MenuItem } from '@workbench/state/contextMenu';
import { control } from '../bridge/control';
import { useConnectionDialogStore } from '../state/connectionDialogs';
import { useConnectionsStore } from '../state/connections';
import { useConsoleDefaultsStore } from '../state/consoleDefaults';
import { useObjectStoreStore } from '../state/objectStore';
import { schemaDialectFor, useSchemaDialogStore } from '../state/schemas';
import { useTabsStore } from '../state/tabs';
import { type CountableKind, countTab, dataQueryCommands } from '../state/viewCommands';
import { nodeIcon } from '../theme/icons';
import { revealConnectionSecret } from './state/connectionReveal';
import { useCredentialsDialogStore } from './state/credentialsDialog';
import {
  groupParentPath,
  rowKey,
  type TreeRowVm,
  useFiltersDialogStore,
  useTreeStore,
} from './state/tree';

// relationMenu/collectionMenu/streamNodeMenu/consumerGroupMenu (menus.ts) and
// keyRowMenu/objectRowMenu (views/browse/menu.ts) each hand-rolled the same open/open-in-new-tab
// pair, the same caps-gated "Open definition" row, the same copy-name(-qualified) pair and the
// same "open a tab, then count it" row — differing only in id/label/icon and which tab opener they
// call. `row` below is the minimal shape every row-like value (a project tree TreeRowVm, or a
// browse TreeNode plus its own connectionId) already has.

export interface MenuRowLike {
  connectionId: string;
  path: string;
  name: string;
}

const QUALIFIED_KINDS = new Set([
  'schema',
  'table',
  'view',
  'matview',
  'sequence',
  'function',
  'collection',
]);

// Produced locally from the path — never round-trips to the engine for a string join (§9b).
function qualifiedNameFor(row: MenuRowLike): string {
  const decoded = decodePath(row.connectionId, row.path);
  return decoded.segments
    .filter((s) => QUALIFIED_KINDS.has(s.kind))
    .map((s) => s.name)
    .join('.');
}

// The Open / Open in new tab pair. Display-only shortcut (P21 D5): Enter fires the tree/panel's
// own onOpen(row) directly (the same action double-click performs), not this run() via
// runMenuShortcut.
export function openItems(
  row: MenuRowLike,
  opts: {
    idBase: string;
    label: string;
    icon: string;
    openTab: (connectionId: string, path: string, tabOpts?: { newTab?: boolean }) => unknown;
  },
): MenuItem[] {
  return [
    {
      type: 'item',
      id: opts.idBase,
      label: opts.label,
      icon: opts.icon,
      shortcut: 'tree.open',
      run: () => {
        opts.openTab(row.connectionId, row.path);
      },
    },
    {
      type: 'item',
      id: `${opts.idBase}-new-tab`,
      label: t('common.action.openInNewTab.label'),
      icon: opts.icon,
      run: () => {
        opts.openTab(row.connectionId, row.path, { newTab: true });
      },
    },
  ];
}

// D5: offered only when the connection's caps say so — never a permanently disabled row.
function definitionItem(row: MenuRowLike): MenuItem[] {
  if (useConnectionsStore().states[row.connectionId]?.caps?.definition !== true) return [];
  return [
    {
      type: 'item',
      id: 'open-definition',
      label: t('studio.project.menu.openDefinition.label'),
      icon: 'file-code',
      run: () => {
        useTabsStore().openDefinitionTab(row.connectionId, row.path);
      },
    },
  ];
}

export function copyNameItems(row: MenuRowLike, qualified = false): MenuItem[] {
  const items: MenuItem[] = [
    {
      type: 'item',
      id: 'copy-name',
      label: t('common.action.copyName.label'),
      icon: 'copy',
      shortcut: 'tree.copyName',
      run: () => void copyAndAnnounce(row.name, t('common.copy.copiedName.announce')),
    },
  ];
  if (qualified) {
    items.push({
      type: 'item',
      id: 'copy-qualified-name',
      label: t('studio.project.menu.copyQualifiedName.label'),
      icon: 'copy',
      run: () =>
        void copyAndAnnounce(
          qualifiedNameFor(row),
          t('studio.project.menu.copyQualifiedName.announce'),
        ),
    });
  }
  return items;
}

// Only relationMenu/collectionMenu share this exact refreshObject call — every other menu's own
// Refresh row reaches a different store method and stays where it is.
function refreshItem(row: MenuRowLike): MenuItem {
  return {
    type: 'item',
    id: 'refresh',
    label: t('common.action.refresh.label'),
    icon: 'refresh',
    run: () => useTreeStore().refreshObject(row.connectionId, row.path),
  };
}

// Opens (or reuses) the row's own tab and runs Σ on it — never a bare count with nowhere to show
// the answer.
function countItem(
  row: MenuRowLike,
  opts: {
    id: string;
    label: string;
    kind: CountableKind;
    openTab: (connectionId: string, path: string) => { id: string } | null;
  },
): MenuItem {
  return {
    type: 'item',
    id: opts.id,
    label: opts.label,
    icon: 'symbol-numeric',
    run: () => {
      const opened = opts.openTab(row.connectionId, row.path);
      if (opened) countTab(opts.kind, opened.id);
    },
  };
}

// Wraps every item's run so a rejection (clipboard denied, delete on a busy DB) lands on the row's
// error popover instead of an unhandled rejection with no trace.
function reportingErrors(row: TreeRowVm, items: MenuItem[]): MenuItem[] {
  const path = row.kind === 'group' ? groupParentPath(row.path) : row.path;
  return items.map((item): MenuItem => {
    if (item.type === 'submenu') return { ...item, items: reportingErrors(row, item.items) };
    if (item.type !== 'item') return item;
    return {
      ...item,
      run: async () => {
        try {
          await item.run();
        } catch (err) {
          useTreeStore().reportError(row.connectionId, path, err);
        }
      },
    };
  });
}

export function menuForRow(row: TreeRowVm): MenuItem[] {
  return reportingErrors(row, rawMenuForRow(row));
}

function rawMenuForRow(row: TreeRowVm): MenuItem[] {
  switch (row.kind) {
    case 'group':
      return groupMenu(row);
    case 'connection':
      return connectionMenu(row);
    case 'database':
    case 'schema':
      return containerMenu(row);
    case 'table':
    case 'view':
    case 'matview':
      return relationMenu(row);
    case 'collection':
      return collectionMenu(row);
    case 'bucket':
      return bucketMenu(row);
    case 'topic':
    case 'queue':
      return streamNodeMenu(row);
    case 'consumerGroup':
      return consumerGroupMenu(row);
    case 'sequence':
    case 'function':
      return copyNameItems(row, true);
    default:
      return [];
  }
}

function isKibana(row: MenuRowLike): boolean {
  return useConnectionsStore().connectionRecord(row.connectionId)?.kind === 'kibana';
}

// D5: offered only when the connection's caps say so, same discipline as "Open definition" — shared by
// all three menu builders below rather than repeated per-row-kind gating logic.
function consoleMenuItem(row: TreeRowVm): MenuItem[] {
  if (useConnectionsStore().states[row.connectionId]?.caps?.sql !== true) return [];
  // A kibana connection's console is Discover, opened by its own items.
  if (isKibana(row)) return [];
  return [
    {
      type: 'item',
      id: 'open-console',
      label: t('studio.project.menu.openConsole.label'),
      icon: 'terminal',
      run: () => {
        useTabsStore().openConsoleTab(row.connectionId, row.path);
      },
    },
  ];
}

// Network kinds in fields mode only: URI mode keeps its URI encrypted, file and AWS kinds have no
// username/password pair to paste.
function canUpdateCredentials(record: ConnectionSummary | undefined): boolean {
  return (
    record !== undefined &&
    record.mode === 'fields' &&
    !FILE_KINDS.has(record.kind) &&
    !AWS_STYLE_KINDS.has(record.kind)
  );
}

// Disabled at the ends and while a search filters the list: the visible order is then partial.
function moveItems(connectionId: string): MenuItem[] {
  const connections = useConnectionsStore();
  const at = connections.records.findIndex((r) => r.id === connectionId);
  const searching = useTreeStore().search.trim() !== '';
  return [
    {
      type: 'item',
      id: 'move-up',
      label: t('common.moveUp.tooltip'),
      icon: 'arrow-up',
      shortcut: 'list.moveUp',
      disabled: searching || at <= 0,
      run: () => connections.moveConnection(connectionId, -1),
    },
    {
      type: 'item',
      id: 'move-down',
      label: t('common.moveDown.tooltip'),
      icon: 'arrow-down',
      shortcut: 'list.moveDown',
      disabled: searching || at < 0 || at >= connections.records.length - 1,
      run: () => connections.moveConnection(connectionId, 1),
    },
  ];
}

function connectionMenu(row: TreeRowVm): MenuItem[] {
  const status = useConnectionsStore().states[row.connectionId]?.status ?? 'disconnected';
  const record = useConnectionsStore().connectionRecord(row.connectionId);
  const isLive = status === 'connected' || status === 'connecting';

  const items: MenuItem[] = [
    isLive
      ? {
          type: 'item',
          id: 'disconnect',
          label: t('studio.project.menu.disconnect.label'),
          icon: 'debug-disconnect',
          run: () => useConnectionsStore().disconnectConnection(row.connectionId),
        }
      : {
          type: 'item',
          id: 'connect',
          label: t('common.action.connect.label'),
          icon: 'plug',
          run: () => useConnectionsStore().connectConnection(row.connectionId),
        },
    {
      type: 'item',
      id: 'refresh',
      label: t('common.action.refresh.label'),
      icon: ICON.refresh,
      run: () => useTreeStore().refreshConnection(row.connectionId),
    },
    {
      type: 'item',
      id: 'edit',
      label: t('studio.project.menu.editFile.label'),
      icon: ICON.edit,
      disabled: !record?.file,
      run: () => useConnectionDialogStore().openFile(record?.file ?? ''),
    },
    {
      type: 'item',
      id: 'open-file',
      label: t('studio.project.menu.openInEditor.label'),
      icon: 'go-to-file',
      disabled: !record?.file,
      run: () => control.configFilesOpen({ folder: 'connections', file: record?.file ?? '' }),
    },
    {
      type: 'item',
      id: 'reveal-file',
      label: t('studio.project.menu.revealFile.label'),
      icon: 'folder-opened',
      disabled: !record?.file,
      run: () => control.configFilesReveal({ folder: 'connections', file: record?.file ?? '' }),
    },
    ...(canUpdateCredentials(record)
      ? [
          {
            type: 'item' as const,
            id: 'update-credentials',
            label: t('studio.project.menu.updateCredentials.label'),
            icon: 'key',
            run: () => useCredentialsDialogStore().openFor(row.connectionId),
          },
        ]
      : []),
    {
      type: 'item',
      id: 'duplicate',
      label: t('common.action.duplicate.label'),
      icon: ICON.duplicate,
      shortcut: 'tree.duplicate',
      run: async () => {
        await useConnectionsStore().duplicateConnection(row.connectionId);
      },
    },
    ...moveItems(row.connectionId),
    ...copyNameItems(row),
    {
      type: 'item',
      id: 'copy-uri',
      label: t('studio.project.menu.copyUri.label'),
      icon: 'link',
      shortcut: 'tree.copyUri',
      // Fields mode has no stored URI: one is synthesised from the fields, passwordless. A URI-mode
      // URI is ciphertext and may hold secrets, so copying it is a gated reveal (P7, P181).
      run: async () => {
        if (!record) return;
        if (record.mode !== 'uri')
          return void copyAndAnnounce(
            formatConnectionUri({ ...record, password: null }),
            t('studio.project.menu.copyUri.announce'),
          );
        const result = await revealConnectionSecret(record.id, record.name, 'uri', () => true);
        if (!result) return;
        if (result.outcome === 'error') throw new Error(result.error);
        if (result.uri === null) throw new Error(t('studio.project.menu.noStoredUri.error'));
        return void copyAndAnnounce(result.uri, t('studio.project.menu.copyUri.announce'));
      },
    },
    {
      type: 'item',
      id: 'filters',
      label: t('studio.project.menu.filters.label'),
      icon: ICON.filter,
      run: () => useFiltersDialogStore().openFiltersDialog(row.connectionId, row.path),
    },
    // P18 (v1.1) D3: a SQL-only surface, same gate the console's own SQL behaviours use.
    ...(schemaDialectFor(record?.kind) !== undefined
      ? [
          {
            type: 'item' as const,
            id: 'schema',
            label: t('studio.project.menu.editSchema.label'),
            icon: 'symbol-structure',
            run: () => useSchemaDialogStore().openSchemaDialog(row.connectionId),
          },
        ]
      : []),
    ...consoleMenuItem(row),
    ...newDiscoverItem(row),
    { type: 'separator' },
    {
      type: 'item',
      id: 'delete',
      label: t('common.action.delete.label'),
      icon: ICON.delete,
      danger: true,
      shortcut: 'tree.delete',
      run: async () => {
        if (
          !(await useConfirmDialogStore().confirm({
            title: t('studio.project.deleteConnectionConfirm.title'),
            message: [
              t('studio.project.deleteConnectionConfirm.body', { name: row.name }),
              t('common.confirm.irreversible.body'),
            ].join(' '),
            confirmLabel: t('common.action.delete.label'),
            tone: 'danger',
          }))
        )
          return;
        await useConnectionsStore().deleteConnection(row.connectionId);
      },
    },
  ];
  return items;
}

// D9: Postgres-only — MariaDB's console can already switch database with its own `USE db;` as
// the console's first statement, so there is nothing for this item to do there.
function setAsDefaultMenuItem(row: TreeRowVm): MenuItem[] {
  const record = useConnectionsStore().connectionRecord(row.connectionId);
  if (record?.kind !== 'postgres') return [];
  return [
    {
      type: 'item',
      id: 'set-as-default',
      label: t('studio.project.menu.setDefault.label'),
      icon: 'star',
      checked: useConsoleDefaultsStore().consoleDefaultFor(row.connectionId) === row.path,
      run: () => useConsoleDefaultsStore().setConsoleDefault(row.connectionId, row.path),
    },
  ];
}

// P41 D17: first position, ahead of Refresh — a redis `database` / s3 `bucket` row's primary
// action once its key space is unbounded (caps.keyBrowser). `shortcut: 'tree.open'` is display-only
// (ProjectTree.vue's onOpen(row) is what Enter actually dispatches, same as relationMenu's own).
function browseMenuItem(row: TreeRowVm): MenuItem[] {
  if (row.kind !== 'database' && row.kind !== 'bucket') return [];
  if (useConnectionsStore().states[row.connectionId]?.caps?.keyBrowser !== true) return [];
  return [
    {
      type: 'item',
      id: 'browse',
      label:
        row.kind === 'bucket'
          ? t('studio.project.menu.browseObjects.label')
          : t('studio.project.menu.browseKeys.label'),
      icon: 'list-tree',
      shortcut: 'tree.open',
      run: () => {
        useTabsStore().openBrowseTab(row.connectionId, row.path);
      },
    },
  ];
}

function containerMenu(row: TreeRowVm): MenuItem[] {
  return [
    ...browseMenuItem(row),
    {
      type: 'item',
      id: 'refresh',
      label: t('common.action.refresh.label'),
      icon: ICON.refresh,
      run: () => useTreeStore().refresh(row.connectionId, row.path),
    },
    ...copyNameItems(row),
    {
      type: 'item',
      id: 'filters',
      label: t('studio.project.menu.filters.label'),
      icon: ICON.filter,
      run: () => useFiltersDialogStore().openFiltersDialog(row.connectionId, row.path),
    },
    ...consoleMenuItem(row),
    ...setAsDefaultMenuItem(row),
  ];
}

// The S3 bucket row: containerMenu (same as a SQL database/schema) plus the Upload — the
// empty-bucket case (no object row to open one from) needs a tree entry point of its own.
// uploadMenuItem now lives in state/objectStore.ts, shared with views/browse/menu.ts's own
// container-row menu.
function bucketMenu(row: TreeRowVm): MenuItem[] {
  return [
    ...containerMenu(row),
    ...useObjectStoreStore().uploadMenuItem(row.connectionId, row.path),
  ];
}

// §8.10's own ordering: Open data / Open data in new tab come first, before Refresh.
function relationMenu(row: TreeRowVm): MenuItem[] {
  return [
    ...openItems(row, {
      idBase: 'open-data',
      label: t('studio.project.menu.openData.label'),
      icon: 'table',
      openTab: (connectionId, path, opts) => useTabsStore().openDataTab(connectionId, path, opts),
    }),
    ...definitionItem(row),
    ...consoleMenuItem(row),
    refreshItem(row),
    ...copyNameItems(row, true),
    countItem(row, {
      id: 'count-rows',
      label: t('studio.project.menu.countRows.label'),
      kind: 'data',
      openTab: (connectionId, path) => useTabsStore().openDataTab(connectionId, path),
    }),
    {
      type: 'submenu',
      id: 'saved-filters',
      label: t('studio.project.menu.savedFilters.label'),
      icon: 'bookmark',
      items: savedFiltersSubmenu(row),
    },
  ];
}

// The collection row: near-copy of relationMenu, opening a 'document' tab instead of 'data' —
// gains "Open definition" (Caps.definition === true for mongo). No saved-filters submenu (a saved
// query body is a SQL WHERE/ORDER BY shape that has no Mongo-filter analog yet).
function collectionMenu(row: TreeRowVm): MenuItem[] {
  if (isKibana(row)) return discoverDataViewMenu(row);
  return [
    ...openItems(row, {
      idBase: 'open-document',
      label: t('common.action.open.label'),
      icon: 'json',
      openTab: (connectionId, path, opts) =>
        useTabsStore().openDocumentTab(connectionId, path, opts),
    }),
    ...definitionItem(row),
    ...consoleMenuItem(row),
    refreshItem(row),
    ...copyNameItems(row, true),
    countItem(row, {
      id: 'count-documents',
      label: t('studio.project.menu.countDocuments.label'),
      kind: 'document',
      openTab: (connectionId, path) => useTabsStore().openDocumentTab(connectionId, path),
    }),
  ];
}

// The connection root: a Discover tab with no data view chosen yet.
function newDiscoverItem(row: TreeRowVm): MenuItem[] {
  if (!isKibana(row)) return [];
  return [
    {
      type: 'item',
      id: 'new-discover',
      label: t('studio.kibana.menu.newDiscover.label'),
      icon: 'search',
      run: () => {
        useTabsStore().openDiscoverTab(row.connectionId, '', { newTab: true });
      },
    },
  ];
}

// A kibana data view: Discover replaces the document, definition, console and count items.
function discoverDataViewMenu(row: TreeRowVm): MenuItem[] {
  return [
    ...openItems(row, {
      idBase: 'open-discover',
      label: t('studio.kibana.menu.openDiscover.label'),
      icon: 'search',
      openTab: (connectionId, path, opts) =>
        useTabsStore().openDiscoverTab(connectionId, path, { ...opts, title: row.name }),
    }),
    refreshItem(row),
    ...copyNameItems(row),
  ];
}

// The synthetic folder row: nothing that needs a real node behind it — Refresh reloads the
// *parent* (the folder is a view over that listing, so this is the only way to refresh what
// it shows), Collapse all is the same global action every other row's menu omits but the tree
// background menu offers, included here because a folder is the row most likely to be right-
// clicked specifically to declutter a crowded schema.
function groupMenu(row: TreeRowVm): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'refresh',
      label: t('common.action.refresh.label'),
      icon: ICON.refresh,
      run: () => useTreeStore().refresh(row.connectionId, groupParentPath(row.path)),
    },
    {
      type: 'item',
      id: 'collapse-all',
      label: t('common.action.collapseAll.label'),
      icon: 'collapse-all',
      run: () => useTreeStore().collapseAll(),
    },
  ];
}

// namespaceMenu/prefixMenu (redis 'namespace' / s3 'prefix' — a container row) and
// keyMenu/objectMenu (redis 'key' / s3 'object' — a leaf row) are gone: neither row exists in the
// tree any more — both moved to views/browse/menu.ts's containerRowMenu/keyRowMenu/
// objectRowMenu, addressed by TreeNode instead of by tree row, with `refresh(parent)`'s tree-row
// refresh replaced by the Browse panel's own local level reload.

// The topic/queue leaf: minimal open/copy-name only, same discipline as keyMenu — no
// edit/delete rows anywhere, per the read-only scope decision. "Open definition" is gated on
// caps.definition the same way relationMenu/collectionMenu gate it — a topic's partitions and
// config, a queue's attributes, live there now that the tree no longer expands either.
function streamNodeMenu(row: TreeRowVm): MenuItem[] {
  return [
    ...openItems(row, {
      idBase: 'open-stream',
      label: t('common.action.open.label'),
      icon: nodeIcon(row.kind),
      openTab: (connectionId, path, opts) => useTabsStore().openStreamTab(connectionId, path, opts),
    }),
    ...definitionItem(row),
    ...copyNameItems(row),
  ];
}

// Kept separate from the sequence/function case (menuForRow's own `copyNameItems(row,
// true)`) rather than sharing definitionItem's gating — those rows' caps.definition is
// true but the adapter still throws E_UNSUPPORTED for those paths, so gating on caps there
// would offer a row that always errors.
function consumerGroupMenu(row: TreeRowVm): MenuItem[] {
  return [...definitionItem(row), ...copyNameItems(row, true)];
}

// Reads from the on-demand cache state/tree.ts populates just before the menu opens
// (ProjectTree.vue's onContextMenu) — building this synchronously is what keeps menuForRow()
// itself synchronous.
function savedFiltersSubmenu(row: TreeRowVm): MenuItem[] {
  const saved = useTreeStore().savedQueries[rowKey(row.connectionId, row.path)] ?? [];
  if (saved.length === 0) {
    return [
      {
        type: 'item',
        id: 'saved-filters-empty',
        label: t('studio.project.menu.savedFilters.empty'),
        disabled: true,
        run: () => {},
      },
    ];
  }
  return saved.map((entry) => ({
    type: 'item' as const,
    id: `saved-filter-${entry.id}`,
    label: entry.name,
    run: async () => {
      const opened = useTabsStore().openDataTab(row.connectionId, row.path);
      if (!opened) return;
      const tabId = opened.id;
      await dataQueryCommands().setFilter(tabId, entry.body.where);
      await dataQueryCommands().setSort(tabId, entry.body.orderBy);
      await control.queriesTouch(entry.id);
    },
  }));
}

export function emptyBackgroundMenu(): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'new-connection',
      label: t('studio.project.connectionDialog.newConnection.title'),
      icon: ICON.add,
      shortcut: 'app.newConnection',
      run: () => useConnectionDialogStore().openNew(),
    },
    {
      type: 'item',
      id: 'refresh-all',
      label: t('common.refreshAll.label'),
      icon: ICON.refresh,
      run: () => useTreeStore().refreshAllConnections(),
    },
    {
      type: 'item',
      id: 'collapse-all',
      label: t('common.action.collapseAll.label'),
      icon: 'collapse-all',
      run: () => useTreeStore().collapseAll(),
    },
  ];
}
