import type { DataGripPreview, DataGripPreviewRow, DataGripReport } from '@shared/domain/datagrip';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { queryClient } from '@workbench/state/queryClient';
import { defineStore } from 'pinia';
import { computed, reactive, toRefs } from 'vue';
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
    state.report = null;
    pickAction.reset();
    confirmAction.reset();
  }

  // A failure opens the dialog on its error alert (a closed dialog would make it look like a
  // cancelled picker). The two actions keep their own pending for their own buttons.
  function openOnFailure(): void {
    state.preview = null;
    state.report = null;
    state.open = true;
  }

  // queryClient is explicit: this store is created before any component mounts.
  const pickAction = useAsyncAction(pickAndScan, { queryClient });
  const confirmAction = useAsyncAction(confirmImport, { queryClient });
  const error = computed(() => pickAction.error.value ?? confirmAction.error.value);

  // Each action runs on its own observer: starting one drops the other's stale failure.
  let scanned = false;
  async function pickAndScan(): Promise<void> {
    scanned = false;
    confirmAction.reset();
    try {
      const chosen = await control.filesChooseFolder(t('studio.start.importDataGrip.label'));
      if (chosen.canceled || !chosen.path) return;
      const preview = await control.datagripScan(chosen.path);
      state.projectPath = chosen.path;
      state.preview = preview;
      state.selected = initialSelection(preview);
      state.report = null;
      state.open = true;
      scanned = true;
    } catch (err) {
      openOnFailure();
      throw err;
    }
  }

  /** Opens the native folder picker and scans the chosen project. Returns false when the
   *  picker was cancelled or failed or the scan failed; the failure shows as `error`. Its `picking`
   *  state also stops a double click opening two pickers. */
  async function pickAndScanDataGripProject(): Promise<boolean> {
    return (await pickAction.run()) && scanned;
  }

  function toggleDataGripRow(uuid: string): void {
    if (state.selected.has(uuid)) state.selected.delete(uuid);
    else state.selected.add(uuid);
  }

  function closeDataGripImportDialog(): void {
    resetDialogState();
  }

  let imported: DataGripReport | null = null;
  async function confirmImport(): Promise<void> {
    imported = null;
    pickAction.reset();
    if (!state.preview || state.selected.size === 0) return;
    const uuids = [...state.selected];
    imported = await control.datagripImport(state.projectPath, uuids);
    state.report = imported;
  }

  /** The dialog's own confirm action. Nothing is written before this runs — the
   *  preview is a real review step, not decoration. The created connections themselves reach
   *  connectionsState.records through the existing connectionsChanged broadcast
   *  (connections.Service.emitListChanged), so nothing here has to re-fetch the list by hand. */
  async function confirmDataGripImport(): Promise<DataGripReport | null> {
    await confirmAction.run();
    return imported;
  }

  return {
    ...toRefs(state),
    error,
    picking: pickAction.pending,
    confirming: confirmAction.pending,
    pickAndScanDataGripProject,
    toggleDataGripRow,
    closeDataGripImportDialog,
    confirmDataGripImport,
  };
});
