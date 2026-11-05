import type { ForeignKeyMeta } from '@shared/domain/tree';
import { newUuid } from '@shared/ids';
import { cellText, isNull, isTruncated, type TypeClass } from '@shared/protocol/page';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { data } from '../../bridge/data';

// P67 §4.1: the popover's own fetch + decode + state shape — no component logic here, this module
// is what FkPreviewPopover.vue renders.

interface PreviewColumn {
  name: string;
  dataType: string;
  typeClass: TypeClass;
  /** Part of the FK edge's referencedColumns — badged in the popover. */
  isTarget: boolean;
  isPrimaryKey: boolean;
}

interface PreviewRow {
  values: { text: string; isNull: boolean; truncated: boolean }[];
}

export type FkPreviewState =
  | { status: 'loading' }
  | { status: 'error'; message: string }
  | { status: 'ready'; columns: PreviewColumn[]; rows: PreviewRow[]; hasMore: boolean };

/** The popover's own in-flight bookkeeping: `cancelled` is a belt-and-braces drop for a response
 *  that lands after close (server-side cancel is best-effort), `opId` is filled in synchronously
 *  by `fetchReferencedRow` below (before the request resolves) so the popover can call
 *  `control.opsCancel(opId)` on close while the read is still outstanding. */
export interface PreviewSignal {
  cancelled: boolean;
  opId: string | null;
}

const PREVIEW_ROW_LIMIT = 1;

/** One tab-free `data.read` against the FK edge's target, scoped to `filter`
 *  (`foreignKeyValueFilter`'s own output — the caller builds it, this never re-derives it).
 *  `tabId: sourceTabId` is attribution only (the op shows in the *source* tab's own op-log) — this
 *  never touches `runtime[sourceTabId]` itself (no `beginOp`, no status/stop-button write). */
export async function fetchReferencedRow(
  connectionId: string,
  sourceTabId: string,
  entry: ForeignKeyMeta,
  filter: string,
  signal: PreviewSignal,
): Promise<FkPreviewState> {
  const opId = newUuid();
  signal.opId = opId;
  try {
    const response = await data.read({
      opId,
      tabId: sourceTabId,
      connectionId,
      path: entry.referencedPath,
      projection: null,
      filter,
      sort: null,
      // P68 7a proposed pageSize 2; not applied. ReadRequestWire.pageSize is 10 | 100 | 1000 |
      // 10000 and Go ReadRequestWire.Validate rejects any other value with E_QUERY. 10 is the floor.
      pageSize: 10,
      cursor: { mode: 'offset', offset: 0 },
    });
    if (signal.cancelled) return { status: 'loading' };
    const page = response.page;
    if (page.kind !== 'tabular') {
      return {
        status: 'error',
        message: t('studio.grid.fkPreview.unexpectedPage.error', { kind: page.kind }),
      };
    }
    const targetColumns = new Set(entry.referencedColumns);
    const columns: PreviewColumn[] = page.columns.map((c) => ({
      name: c.name,
      dataType: c.dataType,
      typeClass: c.typeClass,
      isTarget: targetColumns.has(c.name),
      isPrimaryKey: c.isPrimaryKey,
    }));
    const decoder = new TextDecoder();
    const rowLimit = Math.min(page.rowCount, PREVIEW_ROW_LIMIT);
    const rows: PreviewRow[] = [];
    for (let row = 0; row < rowLimit; row++) {
      rows.push({
        values: page.chunks.map((chunk) => ({
          text: isNull(chunk, row) ? '' : cellText(chunk, row, decoder),
          isNull: isNull(chunk, row),
          truncated: isTruncated(chunk, row),
        })),
      });
    }
    return { status: 'ready', columns, rows, hasMore: page.rowCount > PREVIEW_ROW_LIMIT };
  } catch (err) {
    if (signal.cancelled) return { status: 'loading' };
    return { status: 'error', message: errorText(err) };
  }
}
