import { defineStore } from 'pinia';
import { reactive } from 'vue';
import type { SelectedCell } from '../../../state/cellSelection';
import { useConnectionsStore } from '../../../state/connections';
import type { CellFormat } from './formats';

/** Adds 'no-primary-key' — a table with no primary key can't identify a row to write. Adds
 *  'value-truncated' — the buffer holds only the first 64 KB, so committing it verbatim would
 *  silently overwrite the full value with a truncated one; refused outright rather than
 *  half-supported. */
// Adds 'masked' — the grid's own preview toggle is showing a redaction, not the stored
// value, in this exact cell. Checked first (below): it is the one reason the user can fix
// immediately, by turning the preview off, unlike every other reason here.
// Adds 'generated-column' and 'pending-delete' — readOnlyReasonFor used to say
// nothing was wrong with either, so the dock looked editable while the grid's own onBeforeEditCell/
// stageEdit would refuse (silently, for a pending delete) the exact same write.
export type ReadOnlyReason =
  | 'masked'
  | 'connection-lost'
  | 'connection-read-only'
  | 'value-truncated'
  | 'generated-column'
  | 'pending-delete'
  | 'no-primary-key'
  | 'not-editable-yet';

export const useCellEditorFormatStore = defineStore('cellEditorFormat', () => {
  // Session-only, never persisted (D12): not tabs.state_json, not settings, not SQLite. A `\0`
  // separator because a connection id is a UUID but a path segment can legitimately contain `:`
  // and `/`.
  const overrides = reactive<Record<string, CellFormat>>({});

  function overrideKey(cell: SelectedCell): string {
    return `${cell.connectionId ?? ''}\0${cell.path}\0${cell.column.name}`;
  }

  function overrideFor(cell: SelectedCell): CellFormat | null {
    return overrides[overrideKey(cell)] ?? null;
  }

  function setOverride(cell: SelectedCell, format: CellFormat | null): void {
    const key = overrideKey(cell);
    if (format === null) delete overrides[key];
    else overrides[key] = format;
  }

  /** connection-read-only wins, so §8.6's forced case is always the visible one. `null` means the
   *  cell is genuinely editable in the grid (P5 D2) — the panel itself stays read-only regardless
   *  (D4), but no chip is shown for it. */
  function readOnlyReasonFor(cell: SelectedCell): ReadOnlyReason | null {
    if (cell.stale) return 'connection-lost';
    if (cell.masked) return 'masked';
    const record = useConnectionsStore().connectionRecord(cell.connectionId);
    if (record?.readOnly) return 'connection-read-only';
    // F3: same order the grid gate uses (onBeforeEditCell: !canEditTable() || isDeleted, then
    // generated, then truncated) — connection-read-only above is canEditTable()'s own veto.
    if (cell.pendingDelete) return 'pending-delete';
    if (cell.generated) return 'generated-column';
    if (cell.truncated) return 'value-truncated';
    if (!cell.hasPrimaryKey) return 'no-primary-key';
    return null;
  }

  return { overrideFor, setOverride, readOnlyReasonFor };
});
