import type { CustomScript, ScriptKind } from '@shared/domain/scripts';
import { t } from '@theme/i18n';
import { moveToCollectionMenu } from '../lib/collectionMenu';
import type { MenuItem } from '../state/contextMenu';

// Pure descriptions of the panel's context menus; AutomationsPanel.vue injects what each item does.

export interface ScriptMenuActions {
  run(script: CustomScript): void | Promise<void>;
  runNow(script: CustomScript): void;
  editFile(script: CustomScript): void;
  openFile(script: CustomScript): void | Promise<void>;
  revealFile(script: CustomScript): void | Promise<void>;
  copyPath(script: CustomScript): void | Promise<void>;
  /** Kira Space: set the ADE task and branch of a recurring run. */
  editTarget?(script: CustomScript): void;
  /** Collection names, in panel order. */
  collections(): readonly string[];
  moveTo(script: CustomScript, collection: string): void | Promise<void>;
  moveToNew(script: CustomScript): void | Promise<void>;
  /** One place within the row's own set. */
  moveStep(script: CustomScript, step: -1 | 1): void;
  canMoveStep(script: CustomScript, step: -1 | 1): boolean;
  remove(script: CustomScript): void | Promise<void>;
}

export interface GroupMenuActions {
  create(kind: ScriptKind): void;
  rename(): void;
  remove(): void | Promise<void>;
  step(step: -1 | 1): void;
  can(step: -1 | 1): boolean;
}

export function groupMenu(actions: GroupMenuActions): MenuItem[] {
  const move = { step: actions.step, can: actions.can };
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
    {
      type: 'item',
      id: 'move-up',
      label: t('common.moveUp.tooltip'),
      icon: 'arrow-up',
      disabled: !move.can(-1),
      run: () => move.step(-1),
    },
    {
      type: 'item',
      id: 'move-down',
      label: t('common.moveDown.tooltip'),
      icon: 'arrow-down',
      disabled: !move.can(1),
      run: () => move.step(1),
    },
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
    {
      type: 'item',
      id: 'open-file',
      label: t('common.action.open.label'),
      icon: 'go-to-file',
      run: () => actions.openFile(script),
    },
    {
      type: 'item',
      id: 'reveal-file',
      label: t('common.action.reveal.label'),
      icon: 'folder-opened',
      run: () => actions.revealFile(script),
    },
    {
      type: 'item',
      id: 'copy-path',
      label: t('common.action.copyPath.label'),
      icon: 'copy',
      run: () => actions.copyPath(script),
    },
    { type: 'separator' },
    moveToCollectionMenu({
      collections: actions.collections().map((name) => ({ id: name, name })),
      current: script.collection === '' ? null : script.collection,
      allowNone: true,
      onMove: (name) => actions.moveTo(script, name ?? ''),
      onNew: () => actions.moveToNew(script),
    }),
    {
      type: 'item',
      id: 'move-up',
      label: t('common.moveUp.tooltip'),
      icon: 'arrow-up',
      shortcut: 'list.moveUp',
      disabled: !actions.canMoveStep(script, -1),
      run: () => actions.moveStep(script, -1),
    },
    {
      type: 'item',
      id: 'move-down',
      label: t('common.moveDown.tooltip'),
      icon: 'arrow-down',
      shortcut: 'list.moveDown',
      disabled: !actions.canMoveStep(script, 1),
      run: () => actions.moveStep(script, 1),
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'remove',
      label: t('common.action.remove.label'),
      icon: 'trash',
      danger: true,
      run: () => actions.remove(script),
    },
  ];
}
