import type { GrpcSavedRequest, HttpSavedRequest } from '@shared/domain/collections';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';

// Every sibling dialog (useImportCurlStore, useCopyAsCurlStore, edit-raw, dynamic values)
// already owns its own state; this was the one dialog still living inside the tree store.
// `submitSaveDialog` stays in
// useCollectionsStore (it reaches deep into the tree's own load/reveal/cache machinery), reading
// tabId/payload from here and calling closeSaveDialog() here on completion.

type SaveDialogPayload =
  | { protocol: 'http'; request: HttpSavedRequest }
  | { protocol: 'grpc'; request: GrpcSavedRequest };

interface SaveDialogState {
  open: boolean;
  /** The tab being saved. */
  tabId: string | null;
  suggestedName: string;
  payload: SaveDialogPayload | null;
}

export const useSaveRequestDialogStore = defineStore('saveRequestDialog', () => {
  const state = reactive<SaveDialogState>({
    open: false,
    tabId: null,
    suggestedName: '',
    payload: null,
  });

  /** Save as… — the request view opens this without importing the dialog component. */
  function openSaveDialog(tabId: string, suggestedName: string, request: HttpSavedRequest): void {
    state.tabId = tabId;
    state.suggestedName = suggestedName;
    state.payload = { protocol: 'http', request };
    state.open = true;
  }

  /** openSaveDialog's own gRPC sibling (P11 D12). */
  function openSaveGrpcDialog(
    tabId: string,
    suggestedName: string,
    request: GrpcSavedRequest,
  ): void {
    state.tabId = tabId;
    state.suggestedName = suggestedName;
    state.payload = { protocol: 'grpc', request };
    state.open = true;
  }

  function closeSaveDialog(): void {
    state.open = false;
    state.payload = null;
    state.tabId = null;
  }

  return {
    ...toRefs(state),
    openSaveDialog,
    openSaveGrpcDialog,
    closeSaveDialog,
  };
});
