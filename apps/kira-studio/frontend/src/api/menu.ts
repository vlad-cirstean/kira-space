import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { type CollectionChoice, moveToCollectionMenu } from '@workbench/lib/collectionMenu';
import type { MenuItem } from '@workbench/state/contextMenu';
import type { CollectionRowVm } from './state/collections';

// The row and background context menus, built with the existing MenuItem type from
// state/contextMenu.ts. The collections are config files: each action here is an edit of the
// collection file, and Open file / Reveal show the file itself.
//
// The actions themselves are injected rather than imported, so this module stays free of both the
// store's mutation half and the request-view's open path — the menu is a description, and
// CollectionsTree.vue is the one place that knows how to perform any of it.
export interface CollectionMenuActions {
  open(row: CollectionRowVm): void;
  newRequest(row: CollectionRowVm): void;
  /** The row/background menu's own "New gRPC request" action. */
  newGrpcRequest(row: CollectionRowVm): void;
  newFolder(row: CollectionRowVm): void;
  newCollection(): void;
  rename(row: CollectionRowVm): void;
  duplicate(row: CollectionRowVm): void;
  /** Confirms, then deletes the rows with their subtrees. */
  remove(rows: readonly CollectionRowVm[]): void;
  /** Every collection, as the Move to collection submenu's choices. */
  collections(): readonly CollectionChoice[];
  moveTo(rows: readonly CollectionRowVm[], collectionId: string): void;
  moveToNewCollection(rows: readonly CollectionRowVm[]): void;
  /** One place within the row's own sibling set. */
  moveStep(row: CollectionRowVm, step: -1 | 1): Promise<void>;
  canMoveStep(row: CollectionRowVm, step: -1 | 1): boolean;
  copyUrl(row: CollectionRowVm): void;
  importPostman(): void;
  /** The background menu's own "Import from curl…" item — opens the paste-a-curl-command
   *  dialog, always into a fresh tab (never row-scoped, unlike importPostman above). */
  importCurl(): void;
  exportCollection(row: CollectionRowVm): void;
  /** The collection row's own "Variables…" item — opens (or activates) the variable-set tab
   *  scoped to it. */
  variables(row: CollectionRowVm): void;
  /** The collection row's own "Edit variables" item — the variables file dialog. */
  editVariables(row: CollectionRowVm): void;
  /** The background menu's own "Environments…" item. */
  environments(): void;
  /** The background menu's own "Dynamic values…" item — opens the read-only reference
   *  dialog. Not row-scoped, same as environments() above. */
  dynamicValues(): void;
  /** Opens the row's collection file in the system editor. */
  openFile(row: CollectionRowVm): void;
  /** Shows the row's collection file in the file manager. */
  revealFile(row: CollectionRowVm): void;
}

function fileItems(row: CollectionRowVm, actions: CollectionMenuActions): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'open-file',
      label: t('studio.api.config.openFile.label'),
      icon: 'go-to-file',
      run: () => actions.openFile(row),
    },
    {
      type: 'item',
      id: 'reveal-file',
      label: t('common.action.reveal.label'),
      icon: 'folder-opened',
      run: () => actions.revealFile(row),
    },
  ];
}

// Request and folder rows only: a move lands at a collection's root, so a collection is a valid
// target unless every row already sits at its root.
function moveMenu(rows: readonly CollectionRowVm[], actions: CollectionMenuActions): MenuItem {
  const home = rows.every((r) => r.collectionId === rows[0].collectionId)
    ? rows[0].collectionId
    : null;
  return moveToCollectionMenu({
    collections: actions.collections(),
    current: home,
    allowNone: false,
    canMoveTo: (id) => !rows.every((r) => r.collectionId === id && r.parentId === null),
    onMove: (id) => {
      if (id !== null) actions.moveTo(rows, id);
    },
    onNew: () => actions.moveToNewCollection(rows),
  });
}

function moveItems(row: CollectionRowVm, actions: CollectionMenuActions): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'move-up',
      label: t('common.moveUp.tooltip'),
      icon: 'arrow-up',
      shortcut: 'list.moveUp',
      disabled: !actions.canMoveStep(row, -1),
      run: () => actions.moveStep(row, -1),
    },
    {
      type: 'item',
      id: 'move-down',
      label: t('common.moveDown.tooltip'),
      icon: 'arrow-down',
      shortcut: 'list.moveDown',
      disabled: !actions.canMoveStep(row, 1),
      run: () => actions.moveStep(row, 1),
    },
  ];
}

/** The menu for two or more selected rows: the actions that apply to a set. `deleteCount` is the
 *  topmost rows a delete removes, so the label matches the confirm prompt. */
export function menuForSelection(
  rows: readonly CollectionRowVm[],
  deleteCount: number,
  actions: CollectionMenuActions,
): MenuItem[] {
  const items: MenuItem[] = [];
  if (rows.every((r) => r.kind !== 'collection')) {
    items.push(moveMenu(rows, actions), { type: 'separator' });
  }
  items.push({
    type: 'item',
    id: 'delete',
    label: t('studio.api.collections.deleteMany.label', { count: deleteCount }),
    icon: ICON.delete,
    danger: true,
    shortcut: 'tree.delete',
    run: () => actions.remove(rows),
  });
  return items;
}

function deleteItem(row: CollectionRowVm, actions: CollectionMenuActions): MenuItem {
  return {
    type: 'item',
    id: 'delete',
    label: t('common.action.delete.label'),
    icon: ICON.delete,
    danger: true,
    shortcut: 'tree.delete',
    run: () => actions.remove([row]),
  };
}

export function menuForRow(row: CollectionRowVm, actions: CollectionMenuActions): MenuItem[] {
  if (row.kind === 'request') {
    return [
      {
        type: 'item',
        id: 'open',
        label: t('common.action.open.label'),
        icon: 'go-to-file',
        shortcut: 'tree.open',
        run: () => actions.open(row),
      },
      { type: 'separator' },
      {
        type: 'item',
        id: 'rename',
        label: t('common.action.rename.label'),
        icon: ICON.rename,
        shortcut: 'tree.rename',
        run: () => actions.rename(row),
      },
      {
        type: 'item',
        id: 'duplicate',
        label: t('common.action.duplicate.label'),
        icon: ICON.duplicate,
        shortcut: 'tree.duplicate',
        run: () => actions.duplicate(row),
      },
      {
        type: 'item',
        id: 'copy-url',
        label: t('studio.common.copyUrl.label'),
        icon: 'link',
        disabled: !row.url,
        run: () => actions.copyUrl(row),
      },
      moveMenu([row], actions),
      ...moveItems(row, actions),
      { type: 'separator' },
      ...fileItems(row, actions),
      { type: 'separator' },
      deleteItem(row, actions),
    ];
  }

  const items: MenuItem[] = [
    {
      type: 'item',
      id: 'new-request',
      label: t('studio.api.common.newRequest.label'),
      icon: ICON.add,
      run: () => actions.newRequest(row),
    },
    {
      type: 'item',
      id: 'new-grpc-request',
      label: t('shared.grpc.newRequest.title'),
      icon: 'symbol-interface',
      run: () => actions.newGrpcRequest(row),
    },
    {
      type: 'item',
      id: 'new-folder',
      label: t('studio.api.collections.newFolder.label'),
      icon: 'new-folder',
      run: () => actions.newFolder(row),
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'rename',
      label: t('common.action.rename.label'),
      icon: ICON.rename,
      shortcut: 'tree.rename',
      run: () => actions.rename(row),
    },
  ];

  if (row.kind === 'collection') {
    // Variables… and Export are both collection-level actions: a collection is the unit of
    // export, and the only scope-owning row this menu ever sees (an environment's own variables
    // open through the environments tab instead).
    items.push(
      { type: 'separator' },
      {
        type: 'item',
        id: 'variables',
        label: t('common.module.variables.label'),
        icon: ICON.variables,
        run: () => actions.variables(row),
      },
      {
        type: 'item',
        id: 'edit-variables',
        label: t('studio.api.environments.editVariables.label'),
        icon: 'edit',
        run: () => actions.editVariables(row),
      },
      {
        type: 'item',
        id: 'export',
        label: t('studio.api.collections.export.label'),
        icon: 'export',
        run: () => actions.exportCollection(row),
      },
      ...moveItems(row, actions),
    );
  } else {
    items.push(
      {
        type: 'item',
        id: 'duplicate',
        label: t('common.action.duplicate.label'),
        icon: ICON.duplicate,
        shortcut: 'tree.duplicate',
        run: () => actions.duplicate(row),
      },
      moveMenu([row], actions),
      ...moveItems(row, actions),
    );
  }

  items.push(
    { type: 'separator' },
    ...fileItems(row, actions),
    { type: 'separator' },
    deleteItem(row, actions),
  );
  return items;
}

export function backgroundMenu(actions: CollectionMenuActions): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'new-collection',
      label: t('common.newCollection.label'),
      icon: ICON.newCollection,
      run: () => actions.newCollection(),
    },
    {
      type: 'item',
      id: 'import-collection',
      label: t('studio.api.common.importCollection.label'),
      icon: 'cloud-download',
      run: () => actions.importPostman(),
    },
    {
      type: 'item',
      id: 'import-curl',
      label: t('studio.api.common.importCurl.label'),
      icon: 'terminal',
      run: () => actions.importCurl(),
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'environments',
      label: t('studio.api.common.environments.label'),
      icon: ICON.environments,
      run: () => actions.environments(),
    },
    {
      type: 'item',
      id: 'dynamic-values',
      label: t('studio.api.dynamicValues.title'),
      icon: ICON.variables,
      run: () => actions.dynamicValues(),
    },
  ];
}
