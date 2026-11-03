import type { CustomScript, ScriptKind } from '@shared/domain/scripts';
import { t } from '@theme/i18n';
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
  /** One place within the row's own set. */
  moveStep(script: CustomScript, step: -1 | 1): void;
  canMoveStep(script: CustomScript, step: -1 | 1): boolean;
  remove(script: CustomScript): void | Promise<void>;
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
