import { t } from '@theme/i18n';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';

// Decision 3: asked when the server reports the edited row or document changed since it was
// loaded. True overwrites it; false keeps the user's edit untouched.
export function confirmOverwrite(detail: string): Promise<boolean> {
  return useConfirmDialogStore().confirm({
    title: t('studio.common.overwriteConfirm.title'),
    message: `${detail} ${t('studio.common.overwriteConfirm.body')}`,
    confirmLabel: t('space.ade.workflows.conflict.overwrite.label'),
    tone: 'danger',
  });
}
