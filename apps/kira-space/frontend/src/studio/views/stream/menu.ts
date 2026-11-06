import { t } from '@theme/i18n';
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import { announceApp } from '@workbench/behaviour/liveRegion';
import type { MenuItem } from '@workbench/state/contextMenu';

// Read-only view (P10's D13) — copy-only per-row menu, no edit/delete rows anywhere. Mirrors
// keyvalue/menu.ts; key/body only (no body for a tombstone) (headers/attrs/timestamp are visible inline but rarely
// what someone wants to paste elsewhere).
export function rowMenu(
  key: string | null,
  body: string | null,
  onError: (message: string) => void,
): MenuItem[] {
  const items: MenuItem[] = [];
  if (key !== null) {
    items.push({
      type: 'item',
      id: 'copy-key',
      label: t('studio.stream.copyKey.label'),
      icon: 'copy',
      run: () =>
        copyOrReportError(key, onError, () => announceApp(t('studio.stream.copiedKey.announce'))),
    });
  }
  if (body !== null) {
    items.push({
      type: 'item',
      id: 'copy-body',
      label: t('studio.stream.copyBody.label'),
      icon: 'copy',
      run: () =>
        copyOrReportError(body, onError, () => announceApp(t('studio.stream.copiedBody.announce'))),
    });
  }
  return items;
}
