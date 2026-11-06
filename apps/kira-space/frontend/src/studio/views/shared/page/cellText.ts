import { t } from '@theme/i18n';
import { formatBytes } from '@workbench/behaviour/format';

// Resolved once on first use: grid cell formatters run per cell, and neither string varies.
let nullText: string | undefined;
let truncatedTip: string | undefined;

export function nullCellText(): string {
  nullText ??= t('studio.common.nullValue.label');
  return nullText;
}

export function truncatedCellTip(): string {
  truncatedTip ??= t('studio.common.cellTruncated.tooltip', { size: formatBytes(64 * 1024) });
  return truncatedTip;
}
