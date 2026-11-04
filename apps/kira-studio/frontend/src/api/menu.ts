import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import type { MenuItem } from '@workbench/state/contextMenu';
import type { CollectionRowVm } from './state/collections';

// The row and background context menus, built with the existing MenuItem type from
// state/contextMenu.ts. The collections are config files now (P473): the menu browses, creates,
// imports and exports; renaming, moving and deleting are done in the file.
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

export function menuForRow(row: CollectionRowVm, actions: CollectionMenuActions): MenuItem[] {
  if (row.kind === 'request') {
    return [
      {
        type: 'item',
        id: 'open',
        label: t('common.action.open.label'),
        icon: 'go-to-file',
        run: () => actions.open(row),
      },
      {
        type: 'item',
        id: 'copy-url',
        label: t('studio.common.copyUrl.label'),
        icon: 'link',
        disabled: !row.url,
        run: () => actions.copyUrl(row),
      },
      { type: 'separator' },
      ...fileItems(row, actions),
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
    );
  }

  items.push({ type: 'separator' }, ...fileItems(row, actions));
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
