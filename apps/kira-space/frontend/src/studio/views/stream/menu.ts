import { t } from '@theme/i18n';
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import { announceApp } from '@workbench/behaviour/liveRegion';
import type { MenuItem } from '@workbench/state/contextMenu';
import type { StreamRow } from './page';

// Read-only view (P10's D13) — copy-only per-row menu, no edit/delete rows anywhere. Mirrors
// keyvalue/menu.ts; key/body only (no body for a tombstone) (headers/attrs/timestamp are visible inline but rarely
// what someone wants to paste elsewhere).
export function rowMenu(row: StreamRow | null, onError: (message: string) => void): MenuItem[] {
  const key = row?.key ?? null;
  const body = row?.body ?? null;
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
  if (row !== null) {
    items.push({
      type: 'item',
      id: 'copy-row',
      label: t('studio.stream.copyRow.label'),
      icon: 'copy',
      run: () =>
        copyOrReportError(rowAsJson(row), onError, () =>
          announceApp(t('studio.stream.copiedRow.announce')),
        ),
    });
  }
  return items;
}

// A text cell is its string; a binary cell (base64 of non-UTF-8 bytes) keeps its encoding instead of
// passing as text. A tombstone body is JSON null, distinct from the empty string.
function cellJson(value: string | null, binary: boolean): string | { base64: string } | null {
  return value !== null && binary ? { base64: value } : value;
}

function parseCell(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

// Headers and attributes cells hold JSON text; parsing keeps their order and repeated names (a
// repeat arrives as an array).
function rowAsJson(row: StreamRow): string {
  return JSON.stringify(
    {
      key: cellJson(row.key, row.keyBinary),
      headers: parseCell(row.headers),
      attributes: parseCell(row.attrs),
      timestamp: row.timestamp,
      body: cellJson(row.body, row.bodyBinary),
    },
    null,
    2,
  );
}
