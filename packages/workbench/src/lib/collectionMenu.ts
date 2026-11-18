import { t } from '@theme/i18n';
import type { MenuItem, MenuRadioOption } from '../state/contextMenu';

export interface CollectionChoice {
  id: string;
  name: string;
}

/** The "Move to collection" submenu both the script and API context menus share. */
export function moveToCollectionMenu(opts: {
  collections: readonly CollectionChoice[];
  current: string | null;
  allowNone: boolean;
  /** Default: any id other than the current one. */
  canMoveTo?: (id: string | null) => boolean;
  onMove(id: string | null): void | Promise<void>;
  onNew(): void | Promise<void>;
}): MenuItem {
  const canMoveTo = opts.canMoveTo ?? ((id: string | null) => id !== opts.current);
  const options: MenuRadioOption[] = [];
  if (opts.allowNone) {
    options.push({
      type: 'option',
      id: 'move-to-none',
      value: null,
      label: t('automations.dialog.script.noCollection.option'),
      disabled: !canMoveTo(null),
      rerun: true,
      run: () => opts.onMove(null),
    });
  }
  for (const c of opts.collections) {
    options.push({
      type: 'option',
      id: `move-to-${c.id}`,
      value: c.id,
      label: c.name,
      disabled: !canMoveTo(c.id),
      rerun: true,
      run: () => opts.onMove(c.id),
    });
  }
  const items: MenuItem[] = [
    { type: 'radio', id: 'move-to', value: opts.current, items: options },
    { type: 'separator' },
    {
      type: 'item',
      id: 'move-to-new',
      label: t('common.newCollection.label'),
      icon: 'new-folder',
      run: () => opts.onNew(),
    },
  ];
  return {
    type: 'submenu',
    id: 'move-to-collection',
    label: t('workbench.collectionMenu.move.label'),
    icon: 'folder-library',
    items,
  };
}
