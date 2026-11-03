import type { CustomScript } from '@shared/domain/scripts';
import { t } from '@theme/i18n';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import type { ScriptsSeam } from './module';

/** A run's file name in the runs folder. */
export function runFileName(id: string): string {
  return `${id}.yaml`;
}

// One copy of the confirm-then-remove flow for AutomationsPanel.vue's menu and Delete shortcut.
export function useRemoveScript(): (
  scripts: ScriptsSeam,
  script: CustomScript,
) => Promise<boolean> {
  const confirmDialogStore = useConfirmDialogStore();

  return async (scripts, script) => {
    const ok = await confirmDialogStore.confirm({
      title: t('automations.script.removeConfirm.title'),
      message: t('automations.script.removeConfirm.body', {
        name: script.name,
        module: t('common.module.automations.label'),
      }),
      confirmLabel: t('common.action.remove.label'),
      tone: 'danger',
    });
    if (!ok) return false;
    await scripts.remove(script.id);
    return true;
  };
}
