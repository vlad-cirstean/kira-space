import type { TreeNode } from '@shared/domain/tree';
import { t } from '@theme/i18n';
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import { announceApp } from '@workbench/behaviour/liveRegion';
import { errorText } from '@workbench/bridge/codedError';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import type { MenuItem } from '@workbench/state/contextMenu';
import { copyNameItems, openItems } from '../../project/menu';
import { useConnectionsStore } from '../../state/connections';
import { useObjectStoreStore } from '../../state/objectStore';
import { useTabsStore } from '../../state/tabs';
import { nodeIcon } from '../../theme/icons';
import { useBrowseViewStore } from './state';

// The bodies of project/menus.ts's now-deleted namespaceMenu/prefixMenu (a container row
// — redis 'namespace' / s3 'prefix') and keyMenu/objectMenu (a leaf row — redis 'key' / s3
// 'object'), moved here verbatim once the tree stopped rendering any of these rows at all.
// `refresh(parent)`'s tree-row refresh becomes a local level reload — there is no longer a parent
// tree row to refresh, only this panel's own currently loaded level.

function containerRowMenu(
  tabId: string,
  connectionId: string,
  node: TreeNode,
  stale: boolean,
): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'refresh',
      label: t('common.action.refresh.label'),
      icon: 'refresh',
      run: () => void useBrowseViewStore().reload(tabId),
    },
    {
      type: 'item',
      id: 'copy-name',
      label: t('common.action.copyName.label'),
      icon: 'copy',
      shortcut: 'tree.copyName',
      run: () =>
        copyOrReportError(
          node.name,
          (m) => useBrowseViewStore().setActionError(tabId, m),
          () => announceApp(t('common.copy.copiedName.announce')),
        ),
    },
    // Gates itself on caps.fileTransfer/canInsert — a no-op list for redis's own 'namespace' rows
    // (P33 D3), the same as project/menus.ts's own namespaceMenu-vs-prefixMenu split used to be.
    ...(stale ? [] : useObjectStoreStore().uploadMenuItem(connectionId, node.path, tabId)),
  ];
}

// P9's key leaf, moved verbatim: minimal open/copy-name only (D14) — no download/delete rows
// regardless of caps.canDelete (redis's own is true), per the read-only-tree-row scope decision
// this phase does not revisit.
function keyRowMenu(connectionId: string, node: TreeNode): MenuItem[] {
  const row = { connectionId, path: node.path, name: node.name };
  return [
    ...openItems(row, {
      idBase: 'open',
      label: t('common.action.open.label'),
      icon: nodeIcon(node.kind),
      openTab: (cid, path, opts) => useTabsStore().openKeyValueTab(cid, path, opts),
    }),
    ...copyNameItems(row),
  ];
}

// P17's S3 object leaf, moved verbatim: open/open-in-new-tab/copy-name plus P33's Download and
// Delete, each gated on the connection's own caps/read-only state rather than shown permanently
// disabled.
function objectRowMenu(
  tabId: string,
  connectionId: string,
  node: TreeNode,
  stale: boolean,
): MenuItem[] {
  const connectionsStore = useConnectionsStore();
  const caps = connectionsStore.states[connectionId]?.caps;
  const record = connectionsStore.connectionRecord(connectionId);
  const row = { connectionId, path: node.path, name: node.name };
  const items: MenuItem[] = [
    ...openItems(row, {
      idBase: 'open',
      label: t('common.action.open.label'),
      icon: nodeIcon(node.kind),
      openTab: (cid, path, opts) => useTabsStore().openKeyValueTab(cid, path, opts),
    }),
    ...copyNameItems(row),
  ];

  if (caps?.fileTransfer) {
    items.push({ type: 'separator' });
    items.push({
      type: 'item',
      id: 'download-object',
      label: t('common.action.download.label'),
      icon: 'cloud-download',
      run: () => void useObjectStoreStore().downloadObject(connectionId, node.path, null),
    });
  }

  if (caps?.canDelete && !record?.readOnly) {
    items.push({ type: 'separator' });
    items.push({
      type: 'item',
      id: 'delete-object',
      label: t('common.action.delete.label'),
      icon: 'trash',
      danger: true,
      disabled: stale,
      ...(stale ? { hint: t('studio.common.connectionLost.tooltip') } : {}),
      shortcut: 'tree.delete',
      // P43 F6/D8: this runs inside contextMenu.ts's own `void item.run()` — an unhandled
      // rejection there is guaranteed, not merely possible, so the catch belongs here.
      run: async () => {
        if (
          !(await useConfirmDialogStore().confirm({
            title: t('studio.browse.deleteObjectConfirm.title'),
            message: [
              t('studio.browse.deleteObjectConfirm.body', { name: node.name }),
              t('common.confirm.irreversible.body'),
            ].join(' '),
            confirmLabel: t('common.action.delete.label'),
            tone: 'danger',
          }))
        )
          return;
        if (useBrowseViewStore().isStale(tabId)) {
          useBrowseViewStore().setActionError(tabId, t('studio.common.connectionLost.error'));
          return;
        }
        try {
          await useObjectStoreStore().deleteObject(connectionId, node.path, null);
          await useBrowseViewStore().reload(tabId);
          useBrowseViewStore().setActionError(tabId, null);
        } catch (err) {
          useBrowseViewStore().setActionError(tabId, errorText(err));
        }
      },
    });
  }

  return items;
}

export function menuForNode(
  tabId: string,
  connectionId: string,
  node: TreeNode,
  stale: boolean,
): MenuItem[] {
  if (node.hasChildren) return containerRowMenu(tabId, connectionId, node, stale);
  return node.kind === 'object'
    ? objectRowMenu(tabId, connectionId, node, stale)
    : keyRowMenu(connectionId, node);
}
