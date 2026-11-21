import { moduleShortcut } from '@shared/domain/moduleShortcuts';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { type CollectionChoice, moveToCollectionMenu } from '@workbench/lib/collectionMenu';
import type { MenuItem } from '@workbench/state/contextMenu';
import type { CollectionRowVm } from './state/collections';

// The row and background context menus, built with the existing MenuItem type from
// state/contextMenu.ts. The collections are config files: each action here is an edit of the
// collection file.
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
  copyUrl(row: CollectionRowVm): void;
  importPostman(): void;
  /** The background menu's own "Import from curl…" item — opens the paste-a-curl-command
   *  dialog, always into a fresh tab (never row-scoped, unlike importPostman above). */
  importCurl(): void;
  exportCollection(row: CollectionRowVm): void;
  /** The collection row's own "Variables" item: the Variables tab on its default column. */
  variables(row: CollectionRowVm): void;
  /** The background menu's own "Variables" item. */
  environments(): void;
  /** The background menu's own "Dynamic values…" item — opens the read-only reference
   *  dialog. Not row-scoped, same as environments() above. */
  dynamicValues(): void;
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
    // Variables and Export are both collection-level actions: a collection is the unit of export
    // and owns the default column of the Variables tab.
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
        id: 'export',
        label: t('studio.api.collections.export.label'),
        icon: 'export',
        run: () => actions.exportCollection(row),
      },
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
    );
  }

  items.push({ type: 'separator' }, deleteItem(row, actions));
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
      shortcut: moduleShortcut('api', 'app.import'),
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
      label: t('common.module.variables.label'),
      icon: ICON.variables,
      shortcut: moduleShortcut('api', 'api.environments'),
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

// ---- the Variables tab's menus ----
// Built here, run by VariablesMatrix.vue: the printed chord and the keydown it binds reach the same
// item (runMenuShortcut).

export interface VariableCellMenuActions {
  edit(): void;
  copy(): void;
  paste(): void;
  clear(): void;
  history(): void;
}

/** One cell. `canCopy` is false for a secret (its value is not in the renderer). */
export function variableCellMenu(
  actions: VariableCellMenuActions,
  state: { editable: boolean; canCopy: boolean; canClear: boolean; canHistory: boolean },
): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'edit',
      label: t('common.action.edit.label'),
      icon: ICON.edit,
      disabled: !state.editable,
      shortcut: 'grid.edit',
      run: actions.edit,
    },
    {
      type: 'item',
      id: 'copy',
      label: t('common.action.copy.label'),
      icon: ICON.copy,
      disabled: !state.canCopy,
      shortcut: 'grid.copy',
      run: actions.copy,
    },
    {
      type: 'item',
      id: 'paste',
      label: t('common.action.paste.label'),
      icon: 'clippy',
      disabled: !state.editable,
      shortcut: 'grid.paste',
      run: actions.paste,
    },
    {
      type: 'item',
      id: 'clear',
      label: t('studio.api.variables.cell.clear.label'),
      icon: 'discard',
      disabled: !state.editable || !state.canClear,
      run: actions.clear,
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'history',
      label: t('common.module.history.label'),
      icon: ICON.history,
      disabled: !state.canHistory,
      run: actions.history,
    },
  ];
}

export interface VariableRowMenuActions {
  rename(): void;
  describe(): void;
  copyReference(): void;
  /** Shares or unshares the variable in one environment. */
  share(environmentId: string, on: boolean): void;
  history(): void;
  remove(): void;
}

export function variableRowMenu(
  actions: VariableRowMenuActions,
  state: {
    editable: boolean;
    canHistory: boolean;
    environments: readonly { id: string; name: string; shared: boolean }[];
  },
): MenuItem[] {
  const items: MenuItem[] = [
    {
      type: 'item',
      id: 'rename',
      label: t('common.action.rename.label'),
      icon: ICON.rename,
      disabled: !state.editable,
      shortcut: 'grid.rename',
      run: actions.rename,
    },
    {
      type: 'item',
      id: 'describe',
      label: t('studio.api.variables.row.describe.label'),
      icon: 'note',
      disabled: !state.editable,
      run: actions.describe,
    },
    {
      type: 'item',
      id: 'copy-reference',
      label: t('studio.api.variables.row.copyReference.label'),
      icon: ICON.copy,
      run: actions.copyReference,
    },
  ];
  if (state.environments.length > 0) {
    items.push({
      type: 'submenu',
      id: 'share',
      label: t('studio.api.variables.row.share.label'),
      icon: 'hubot',
      items: state.environments.map(
        (env): MenuItem => ({
          type: 'checkbox',
          id: `share-${env.id}`,
          label: env.name,
          checked: env.shared,
          run: () => actions.share(env.id, !env.shared),
        }),
      ),
    });
  }
  items.push(
    {
      type: 'item',
      id: 'history',
      label: t('common.module.history.label'),
      icon: ICON.history,
      disabled: !state.canHistory,
      run: actions.history,
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'delete',
      label: t('common.action.delete.label'),
      icon: ICON.delete,
      danger: true,
      disabled: !state.editable,
      shortcut: 'grid.deleteRows',
      run: actions.remove,
    },
  );
  return items;
}

export interface VariableColumnMenuActions {
  rename(): void;
  agentAccess(): void;
  dotenv(): void;
  yaml(): void;
  duplicate(): void;
  remove(): void;
}

/** An environment column's menu; the collection's default column passes `environment: false`. */
export function variableColumnMenu(
  actions: VariableColumnMenuActions,
  environment: boolean,
): MenuItem[] {
  const file: MenuItem[] = [
    {
      type: 'item',
      id: 'dotenv',
      label: t('studio.api.variablesDialog.bulk.toggle.label'),
      icon: 'list-flat',
      run: actions.dotenv,
    },
    {
      type: 'item',
      id: 'yaml',
      label: t('studio.api.variables.column.yaml.label'),
      icon: 'file-code',
      run: actions.yaml,
    },
  ];
  if (!environment) return file;
  return [
    {
      type: 'item',
      id: 'rename',
      label: t('studio.api.variables.column.rename.label'),
      icon: ICON.rename,
      run: actions.rename,
    },
    {
      type: 'item',
      id: 'agent',
      label: t('studio.api.variables.column.agent.label'),
      icon: 'hubot',
      run: actions.agentAccess,
    },
    ...file,
    { type: 'separator' },
    {
      type: 'item',
      id: 'duplicate',
      label: t('common.action.duplicate.label'),
      icon: ICON.duplicate,
      run: actions.duplicate,
    },
    {
      type: 'item',
      id: 'delete',
      label: t('common.action.delete.label'),
      icon: ICON.delete,
      danger: true,
      run: actions.remove,
    },
  ];
}
