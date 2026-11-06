import { pathParent } from '@shared/domain/tree';
import type { ColumnDescriptor } from '@shared/protocol/page';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { errorText } from '@workbench/bridge/codedError';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { ref, watch } from 'vue';
import { type SelectedCell, useCellSelectionStore } from '../../../state/cellSelection';
import { useObjectStoreStore } from '../../../state/objectStore';
import { browseInvalidate } from '../../../state/viewCommands';
import { deleteKey, saveValueEdit } from './mutations';
import { useKeyValueViewStore } from './state';
import type { useKeyValueGate } from './useKeyValueGate';

/** Row preview, S3 body draft, delete and download for one key/object view. */
export function useKeyValueActions(
  viewKey: () => string,
  gate: ReturnType<typeof useKeyValueGate>,
) {
  const cellSelectionStore = useCellSelectionStore();
  const confirmDialogStore = useConfirmDialogStore();
  const objectStoreStore = useObjectStoreStore();
  const keyValueViewStore = useKeyValueViewStore();

  // S3 PutObject is an immediate write with no undo: the cell editor only stages a draft here
  // and the explicit Save is the sole caller of saveValueEdit for the object body.
  const objectDraft = ref<string | null>(null);
  const objectSave = useAsyncAction((value: string) =>
    saveValueEdit(viewKey(), gate.keyName.value, value),
  );

  // A reload leaves a staged draft and the published cell stale: a later Save could overwrite newer data.
  let selectedRow: number | null = null;
  watch(gate.page, () => {
    selectedRow = null;
    objectDraft.value = null;
    objectSave.error.value = null;
    cellSelectionStore.clearSelectedCellFor(viewKey());
  });

  async function saveObjectEdit(): Promise<void> {
    if (objectDraft.value === null || !gate.keyName.value) return;
    if (gate.stale.value) {
      objectSave.error.value = t('studio.common.connectionLost.error');
      return;
    }
    if (await objectSave.run(objectDraft.value)) objectDraft.value = null;
  }

  // The cell editor panel previews any clicked row read-only; only the S3 Body row can stage an edit.
  function selectRow(i: number): void {
    const row = gate.rowAt(i);
    const p = gate.page.value;
    const h = gate.host.value;
    if (!row || !p) return;
    selectedRow = i;
    const column: ColumnDescriptor = {
      name: p.redisType === 'string' ? 'value' : row.field,
      dataType: p.redisType === 'object' ? 's3 object field' : `redis ${p.redisType}`,
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    };
    const isEditableBodyRow =
      p.redisType === 'object' && row.field === 'Body' && gate.objectEditGate.value.editable;
    const selected: SelectedCell = {
      tabId: viewKey(),
      connectionId: h?.connectionId ?? null,
      path: h?.path ?? '',
      columnIndex: 1, // the page's `values` column; `fields` is 0
      column,
      row: i,
      value: row.value,
      truncated: row.isTruncated,
      hasPrimaryKey: true,
      stale: gate.stale.value,
      ...(isEditableBodyRow
        ? {
            onEdit: (newValue: string) => {
              objectDraft.value = newValue;
            },
            onRevert: () => {
              objectDraft.value = null;
            },
          }
        : {}),
    };
    cellSelectionStore.publishSelectedCell(selected);
  }

  // Republish the selected row at a drop or reconnect so the dock's Save follows the stale state.
  watch(gate.stale, () => {
    if (selectedRow !== null) selectRow(selectedRow);
  });

  function editObjectBody(): void {
    if (!gate.objectEditGate.value.editable) return;
    const index = gate.objectBodyRow.value?.index;
    if (index !== undefined) selectRow(index);
  }

  // S3 delete rides objectStore.deleteObject: it needs the object's whole path, not just the key name.
  async function onDeleteKey(): Promise<void> {
    if (!gate.canDelete.value || !gate.keyName.value) return;
    const isObject = gate.isSingleObjectPage.value;
    const name = gate.keyName.value;
    const confirmed = await confirmDialogStore.confirm({
      title: t(
        isObject
          ? 'studio.browse.deleteObjectConfirm.title'
          : 'studio.keyValue.deleteKeyConfirm.title',
      ),
      message: [
        t(
          isObject
            ? 'studio.browse.deleteObjectConfirm.body'
            : 'studio.keyValue.deleteKeyConfirm.body',
          { name },
        ),
        t('common.confirm.irreversible.body'),
      ].join(' '),
      confirmLabel: t('common.action.delete.label'),
      tone: 'danger',
    });
    if (!confirmed) return;
    if (gate.stale.value) {
      keyValueViewStore.setActionError(viewKey(), t('studio.common.connectionLost.error'));
      return;
    }
    const h = gate.host.value;
    if (!h?.connectionId) return;
    try {
      if (isObject) {
        await objectStoreStore.deleteObject(h.connectionId, h.path, viewKey());
        await keyValueViewStore.reload(viewKey());
        browseInvalidate(h.connectionId, pathParent(h.path) ?? '');
      } else {
        await deleteKey(viewKey(), name);
      }
      keyValueViewStore.setActionError(viewKey(), null);
    } catch (err) {
      keyValueViewStore.setActionError(viewKey(), errorText(err));
    }
  }

  // A read: never blocked by read-only, gated only on caps.fileTransfer.
  async function onDownload(): Promise<void> {
    const h = gate.host.value;
    if (!gate.canDownload.value || !h?.connectionId) return;
    await objectStoreStore.downloadObject(h.connectionId, h.path, viewKey());
  }

  function onUpload(): void {
    const h = gate.host.value;
    if (h?.connectionId)
      objectStoreStore.openUploadDialog(h.connectionId, pathParent(h.path) ?? '', h.gateTabId);
  }

  return {
    objectDraft,
    objectSaving: objectSave.pending,
    objectSaveError: objectSave.error,
    saveObjectEdit,
    selectRow,
    editObjectBody,
    onDeleteKey,
    onDownload,
    onUpload,
  };
}
