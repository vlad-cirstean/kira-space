import { t } from '@theme/i18n';

/** Why a write control is disabled. "Connection is read-only" explains it only when the
 *  connection's own toggle is the cause; an adapter that cannot write at all gets the structural
 *  reason, since that toggle could never turn the control back on. `stale` (shown data outlived its
 *  connection) wins over both. */
export function writeDisabledReason(capFlag: boolean | undefined, stale = false): string {
  if (stale) return t('studio.common.connectionLost.tooltip');
  return t(
    capFlag === false
      ? 'studio.common.writeUnsupported.tooltip'
      : 'studio.common.connectionReadOnly.tooltip',
  );
}
