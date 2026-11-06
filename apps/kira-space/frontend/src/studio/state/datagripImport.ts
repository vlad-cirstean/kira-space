import type { DataGripPreview, DataGripPreviewRow, DataGripReport } from '@shared/domain/datagrip';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { control } from '../bridge/control';
import { useConnectionsStore } from './connections';

/** D10: "looks like it's already imported" — a live comparison against connectionsState.records,
 *  matching on name+host+port+database. Nothing is persisted to detect this (§0.3). */
export function looksAlreadyImported(row: DataGripPreviewRow): boolean {
  return useConnectionsStore().records.some(
    (r) =>
      r.name === row.name &&
      (r.host ?? null) === (row.host ?? null) &&
      (r.port ?? null) === (row.port ?? null) &&
      (r.database ?? null) === (row.database ?? null),
  );
}

export const useDatagripImportStore = defineStore('datagripImport', () => {
  const state = reactive({
    open: false,
    projectPath: '',
    preview: null as DataGripPreview | null,
    selected: new Set<string>(),
    error: null as string | null,
    report: null as DataGripReport | null,
  });

  // D10: everything that maps starts checked, except a probable re-import, which starts unchecked
  // so the user has to opt back in to a likely duplicate.
  function initialSelection(preview: DataGripPreview): Set<string> {
    const selected = new Set<string>();
    for (const row of preview.rows) {
      if (row.importable && !looksAlreadyImported(row)) selected.add(row.uuid);
    }
    return selected;
  }

  function resetDialogState(): void {
    state.open = false;
    state.projectPath = '';
    state.preview = null;
    state.selected = new Set();
    state.error = null;
    state.report = null;
  }

  function failToDialog(err: unknown): false {
    state.error = errorText(err);
    state.preview = null;
    state.report = null;
    state.open = true;
    return false;
  }

  /** Opens the native folder picker and scans the chosen project. Returns false when the
   *  picker was cancelled or failed or the scan failed. A failure opens the dialog on its error
   *  alert (a closed dialog would make it look like a cancelled picker). Callers own the pending
   *  state (`useAsyncAction`), which also stops a double click opening two pickers. */
  async function pickAndScanDataGripProject(): Promise<boolean> {
    state.error = null;
    let path: string;
    try {
      const chosen = await control.filesChooseFolder(t('studio.start.importDataGrip.label'));
      if (chosen.canceled || !chosen.path) return false;
      path = chosen.path;
    } catch (err) {
      return failToDialog(err);
    }
    return scanDataGripProject(path);
  }

  async function scanDataGripProject(path: string): Promise<boolean> {
    state.error = null;
    try {
      const preview = await control.datagripScan(path);
      state.projectPath = path;
      state.preview = preview;
      state.selected = initialSelection(preview);
      state.report = null;
      state.open = true;
      return true;
    } catch (err) {
      return failToDialog(err);
    }
  }

  function toggleDataGripRow(uuid: string): void {
    if (state.selected.has(uuid)) state.selected.delete(uuid);
    else state.selected.add(uuid);
  }

  function closeDataGripImportDialog(): void {
    resetDialogState();
  }

  /** The dialog's own confirm action. Nothing is written before this runs — the
   *  preview is a real review step, not decoration. The created connections themselves reach
   *  connectionsState.records through the existing connectionsChanged broadcast
   *  (connections.Service.emitListChanged), so nothing here has to re-fetch the list by hand. */
  async function confirmDataGripImport(): Promise<DataGripReport | null> {
    if (!state.preview || state.selected.size === 0) return null;
    const uuids = [...state.selected];
    state.error = null;
    try {
      const report = await control.datagripImport(state.projectPath, uuids);
      state.report = report;
      return report;
    } catch (err) {
      state.error = errorText(err);
      return null;
    }
  }

  return {
    ...toRefs(state),
    pickAndScanDataGripProject,
    toggleDataGripRow,
    closeDataGripImportDialog,
    confirmDataGripImport,
  };
});
