import { t } from '@theme/i18n';
import { formatBytes } from '@workbench/behaviour/format';
import type { ReadOnlyReason } from './state';

export function readOnlyChipText(reason: ReadOnlyReason | null): string {
  switch (reason) {
    case 'connection-lost':
      return t('studio.cellEditor.readOnly.connectionLost.label');
    case 'masked':
      return t('studio.cellEditor.readOnly.masked.label');
    case 'connection-read-only':
      return t('studio.common.connectionReadOnly.tooltip');
    case 'value-truncated':
      return t('studio.cellEditor.readOnly.truncated.label');
    case 'generated-column':
      return t('studio.cellEditor.readOnly.generated.label');
    case 'pending-delete':
      return t('studio.cellEditor.readOnly.pendingDelete.label');
    case 'no-primary-key':
      return t('studio.cellEditor.readOnly.noPrimaryKey.label');
    default:
      return '';
  }
}

export function readOnlyChipTitle(reason: ReadOnlyReason | null): string | undefined {
  switch (reason) {
    case 'connection-lost':
      return t('studio.common.connectionLost.tooltip');
    case 'masked':
      return t('studio.cellEditor.readOnly.masked.hint');
    case 'value-truncated':
      return t('studio.cellEditor.readOnly.truncated.hint', { size: formatBytes(64 * 1024) });
    case 'generated-column':
      return t('studio.cellEditor.readOnly.generated.hint');
    case 'pending-delete':
      return t('studio.cellEditor.readOnly.pendingDelete.hint');
    case 'no-primary-key':
      return t('studio.grid.noPrimaryKey.tooltip');
    default:
      return undefined;
  }
}
