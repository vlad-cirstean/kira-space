import { t } from '@theme/i18n';
import { isConflict, toCodedError } from '@workbench/bridge/codedError';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';

// Decision 3: asked when the server reports the edited row or document changed since it was
// loaded. True overwrites it; false keeps the user's edit untouched.
export function confirmOverwrite(detail: string): Promise<boolean> {
  return useConfirmDialogStore().confirm({
    title: t('studio.common.overwriteConfirm.title'),
    message: `${detail} ${t('studio.common.overwriteConfirm.body')}`,
    confirmLabel: t('common.action.overwrite.label'),
    tone: 'danger',
  });
}

/** An `E_CONFLICT` whose `details` carry `flag` true. Other conflicts (a name collision, an
 *  unrelated adapter refusal) must not reach an overwrite prompt: confirming would resend unguarded. */
export function isConflictWith(err: unknown, flag: string): boolean {
  const coded = toCodedError(err);
  const details = coded.details as Record<string, unknown> | undefined;
  return isConflict(err) && details?.[flag] === true;
}

/** Decision 3: the server reports the row, document or saved item changed since it was loaded. */
export function isChangedConflict(err: unknown): boolean {
  return isConflictWith(err, 'changed');
}
