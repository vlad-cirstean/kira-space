import type { CustomScript, ScriptKind } from '@shared/domain/scripts';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { moveToCollectionMenu } from '../lib/collectionMenu';
import type { MenuItem } from '../state/contextMenu';

// Pure descriptions of the panel's context menus; AutomationsPanel.vue injects what each item does.

export interface ScriptMenuActions {
  run(script: CustomScript): void | Promise<void>;
  runNow(script: CustomScript): void;
  editFile(script: CustomScript): void;
  /** Kira Space: set the ADE task and branch of a recurring run. */
  editTarget?(script: CustomScript): void;
  /** Collection names, in panel order. */
  collections(): readonly string[];
  moveTo(script: CustomScript, collection: string): void | Promise<void>;
  moveToNew(script: CustomScript): void | Promise<void>;
  remove(script: CustomScript): void | Promise<void>;
}

export interface GroupMenuActions {
  create(kind: ScriptKind): void;
  rename(): void;
  remove(): void | Promise<void>;
}

export function groupMenu(actions: GroupMenuActions): MenuItem[] {
  return [
    ...newScriptItems(actions.create),
    {
      type: 'item',
      id: 'rename',
      label: t('common.action.rename.label'),
      icon: 'edit',
      shortcut: 'tree.rename',
      run: () => actions.rename(),
    },
    { type: 'separator' },
    { type: 'separator' },
    {
      type: 'item',
      id: 'delete',
      label: t('common.action.delete.label'),
      icon: 'trash',
      danger: true,
      run: () => actions.remove(),
    },
  ];
}

export function newScriptItems(create: (kind: ScriptKind) => void): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'new-script',
      label: t('automations.script.new.label'),
      icon: 'add',
      run: () => create('script'),
    },
    {
      type: 'item',
      id: 'new-smart-script',
      label: t('automations.script.newSmart.label'),
      icon: 'sparkle',
      run: () => create('smart'),
    },
  ];
}

export function scriptMenu(script: CustomScript, actions: ScriptMenuActions): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'run',
      label: t('common.action.run.label'),
      icon: 'play',
      run: () => actions.run(script),
    },
    ...(script.schedule
      ? ([
          {
            type: 'item',
            id: 'run-now',
            label: t('automations.script.runNow.label'),
            icon: 'debug-start',
            run: () => actions.runNow(script),
          },
          ...(actions.editTarget
            ? ([
                {
                  type: 'item',
                  id: 'edit-target',
                  label: t('automations.script.editTarget.label'),
                  icon: 'target',
                  run: () => actions.editTarget?.(script),
                },
              ] satisfies MenuItem[])
            : []),
        ] satisfies MenuItem[])
      : []),
    {
      type: 'item',
      id: 'edit-file',
      label: t('automations.script.editFile.label'),
      icon: 'edit',
      run: () => actions.editFile(script),
    },
    { type: 'separator' },
    moveToCollectionMenu({
      collections: actions.collections().map((name) => ({ id: name, name })),
      current: script.collection === '' ? null : script.collection,
      allowNone: true,
      onMove: (name) => actions.moveTo(script, name ?? ''),
      onNew: () => actions.moveToNew(script),
    }),
    { type: 'separator' },
    {
      type: 'item',
      id: 'remove',
      label: t('common.action.remove.label'),
      icon: ICON.remove,
      danger: true,
      run: () => actions.remove(script),
    },
  ];
}
