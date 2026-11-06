import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed, ref, watch } from 'vue';
import { beautifyShellText, toShellText } from '../shared/document/ejson';
import { usePublishEditDirty } from '../shared/editDirty';
import { useEditBuffer } from '../shared/useEditBuffer';
import { saveDocumentEdit, saveNewDocument } from './mutations';
import { documentRow, getPage, pageVersion } from './page';
import { useDocumentViewStore } from './state';

const NEW_DOCUMENT_TEMPLATE = '{\n  \n}';

export function useDocumentEdit(
  tabId: () => string,
  isEditable: (row: number) => boolean,
  isStale: () => boolean,
) {
  const documentViewStore = useDocumentViewStore();

  const creatingNew = ref(false);
  const newBuffer = useEditBuffer({
    original: () => NEW_DOCUMENT_TEMPLATE,
    beautifier: () => beautifyShellText,
  });
  const editingId = ref<string | null>(null);
  const editingRow = ref<number | null>(null);
  const editOriginal = ref('');
  const editBuffer = useEditBuffer({
    original: () => editOriginal.value,
    beautifier: () => beautifyShellText,
  });

  // A second Save while the first awaits the server would insert a second document (no `_id` yet).
  const createAction = useAsyncAction(() => saveNewDocument(tabId(), newBuffer.doc.value));
  const editAction = useAsyncAction((id: string) =>
    saveDocumentEdit(tabId(), id, editBuffer.doc.value),
  );
  const saving = computed(() => createAction.pending.value || editAction.pending.value);

  usePublishEditDirty(
    tabId,
    () =>
      (creatingNew.value && newBuffer.isDirty.value) ||
      (editingRow.value !== null && editBuffer.isDirty.value),
  );

  function onAddDocument(): void {
    creatingNew.value = true;
    newBuffer.reseed();
  }

  function cancelCreate(): void {
    creatingNew.value = false;
  }

  async function commitCreate(): Promise<void> {
    if (saving.value) return;
    if (isStale()) {
      documentViewStore.setActionError(tabId(), t('studio.common.connectionLost.error'));
      return;
    }
    if (await createAction.run()) {
      documentViewStore.setActionError(tabId(), null);
      creatingNew.value = false;
      newBuffer.reseed();
    } else {
      documentViewStore.setActionError(tabId(), createAction.error.value);
    }
  }

  function startEdit(row: number, id: string, body: string): void {
    if (!isEditable(row)) return;
    editingRow.value = row;
    editingId.value = id;
    editOriginal.value = toShellText(body);
    editBuffer.reseed();
  }

  function cancelEdit(): void {
    editingRow.value = null;
    editingId.value = null;
  }

  async function commitEdit(): Promise<void> {
    const id = editingId.value;
    if (id === null || saving.value) return;
    if (isStale()) {
      documentViewStore.setActionError(tabId(), t('studio.common.connectionLost.error'));
      return;
    }
    if (await editAction.run(id)) {
      documentViewStore.setActionError(tabId(), null);
      editingRow.value = null;
      editingId.value = null;
    } else {
      documentViewStore.setActionError(tabId(), editAction.error.value);
    }
  }

  // A new page (paging, a delete's reload, filter) moves or removes the edited document: re-find it
  // by id so the editor stays under its own document, and drop the edit when it left the page.
  const currentPage = computed(() => {
    void pageVersion.n;
    return getPage(tabId());
  });
  watch(currentPage, (page) => {
    const id = editingId.value;
    if (id === null || !page) return;
    for (let i = 0; i < page.rowCount; i++) {
      if (documentRow(tabId(), i)?.id === id) {
        editingRow.value = i;
        return;
      }
    }
    cancelEdit();
  });

  return {
    creatingNew,
    newBuffer,
    editingRow,
    editBuffer,
    saving,
    onAddDocument,
    cancelCreate,
    commitCreate,
    startEdit,
    cancelEdit,
    commitEdit,
  };
}
