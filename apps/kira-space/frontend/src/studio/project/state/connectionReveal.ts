import { t } from '@theme/i18n';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { control } from '../../bridge/control';

export type ConnectionRevealResult =
  | { outcome: 'revealed'; password: string | null; uri: string | null }
  | { outcome: 'error'; error: string };

// The backend decides the outcome, this renders it. Recurses once for the
// confirmation-required -> user confirms -> re-ask-with-confirmed:true path. Resolves undefined
// when the user cancelled or declined, or when isCurrent turned false across an await (the caller
// moved on; nothing may be written or shown for it).
export async function revealConnectionSecret(
  id: string,
  name: string,
  mode: 'fields' | 'uri',
  isCurrent: () => boolean,
  confirmed = false,
): Promise<ConnectionRevealResult | undefined> {
  const result = await control.connectionsReveal(id, confirmed);
  if (!isCurrent()) return undefined;
  switch (result.outcome) {
    case 'revealed':
      return { outcome: 'revealed', password: result.password, uri: result.uri ?? null };
    case 'cancelled':
      // D11: the user cancelled the OS prompt on purpose — nothing to show for it.
      return undefined;
    case 'confirmation-required': {
      const ok = await useConfirmDialogStore().confirm({
        title: t(
          mode === 'uri'
            ? 'studio.project.uriRevealConfirm.title'
            : 'studio.project.passwordRevealConfirm.title',
        ),
        message: t(
          mode === 'uri'
            ? 'studio.project.uriRevealConfirm.body'
            : 'studio.project.passwordRevealConfirm.body',
          { name },
        ),
        confirmLabel: t('common.action.reveal.label'),
        tone: 'primary',
      });
      if (!isCurrent() || !ok) return undefined;
      return revealConnectionSecret(id, name, mode, isCurrent, true);
    }
    default:
      return {
        outcome: 'error',
        error:
          result.error ??
          t(
            mode === 'uri'
              ? 'studio.project.uriRevealFailed.error'
              : 'studio.project.passwordRevealFailed.error',
          ),
      };
  }
}
