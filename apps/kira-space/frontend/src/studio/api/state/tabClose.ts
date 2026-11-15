import {
  httpRequestTitle,
  isDirty,
  isGrpcDirty,
  sameGrpcRequest,
  sameRequest,
  toSavedGrpcRequest,
  toSavedRequest,
} from '@kira/api-core';
import type { GrpcSavedRequest, HttpSavedRequest } from '@shared/domain/collections';
import { defaultGrpcRequestTabState, grpcRequestTitle } from '@shared/domain/grpc';
import { defaultHttpRequestTabState } from '@shared/domain/http';
import { t } from '@theme/i18n';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { queryClient } from '@workbench/state/queryClient';
import { defineStore } from 'pinia';
import { onScopeDispose, type Ref, shallowReactive, watchEffect } from 'vue';
import {
  asGrpcRequestTab,
  asHttpRequestTab,
  type GrpcRequestTabRecord,
  type HttpRequestTabRecord,
} from '../../../state/tabDomain';
import { useTabsStore } from '../../../state/tabs';
import { useTabIncognitoStore } from '../../state/tabIncognito';
import {
  apiSavedGrpcRequestKey,
  apiSavedRequestKey,
  loadSavedGrpcRequest,
  loadSavedRequest,
} from './apiQueries';
import { useCollectionsStore } from './collections';
import { useSaveRequestDialogStore } from './saveRequestDialog';

// Guards closing an API tab (`http-request`, `grpc-request`). Dirty means a bound tab differs from
// its saved row, or a tab with no row holds more than a blank request. `unknown` is a bound tab
// whose row was never loaded (a restored background tab): its persisted state may hold edits.
type CloseState = 'clean' | 'dirty' | 'unknown';

export const useApiTabCloseStore = defineStore('apiTabClose', () => {
  // The saved request each mounted view last synced with (Decision 3's `seenSaved`).
  const seen = shallowReactive<Record<string, HttpSavedRequest | GrpcSavedRequest>>({});

  function publishSeen(tabId: string, value: HttpSavedRequest | GrpcSavedRequest): void {
    seen[tabId] = value;
  }

  function forgetSeen(tabId: string): void {
    delete seen[tabId];
  }

  function find(tabId: string) {
    const tab = useTabsStore().tabs.find((x) => x.id === tabId);
    return asHttpRequestTab(tab) ?? asGrpcRequestTab(tab);
  }

  function httpCloseState(tab: HttpRequestTabRecord): CloseState {
    const { itemId } = tab.state;
    const row =
      itemId === null
        ? null
        : queryClient.getQueryData<HttpSavedRequest | null>(apiSavedRequestKey(itemId));
    if (row === undefined) return 'unknown';
    if (row) return isDirty(tab.state, row) ? 'dirty' : 'clean';
    const blank = sameRequest(
      toSavedRequest(tab.state),
      toSavedRequest(defaultHttpRequestTabState()),
    );
    return blank ? 'clean' : 'dirty';
  }

  function grpcCloseState(tab: GrpcRequestTabRecord): CloseState {
    const { itemId } = tab.state;
    const row =
      itemId === null
        ? null
        : queryClient.getQueryData<GrpcSavedRequest | null>(apiSavedGrpcRequestKey(itemId));
    if (row === undefined) return 'unknown';
    if (row) return isGrpcDirty(tab.state, row) ? 'dirty' : 'clean';
    const blank = sameGrpcRequest(
      toSavedGrpcRequest(tab.state),
      toSavedGrpcRequest(defaultGrpcRequestTabState()),
    );
    return blank ? 'clean' : 'dirty';
  }

  function closeState(tabId: string): CloseState {
    const tab = find(tabId);
    if (!tab || useTabIncognitoStore().isIncognito(tabId)) return 'clean';
    return tab.kind === 'http-request' ? httpCloseState(tab) : grpcCloseState(tab);
  }

  /** Save, Discard or Cancel. True closes the tab: after Discard, or after Save wrote the row. */
  async function confirmClose(tabId: string): Promise<boolean> {
    let tab = find(tabId);
    if (!tab) return true;
    if (closeState(tabId) === 'unknown' && tab.state.itemId !== null) {
      if (tab.kind === 'http-request') await loadSavedRequest(tab.state.itemId);
      else await loadSavedGrpcRequest(tab.state.itemId);
      tab = find(tabId);
      if (!tab || closeState(tabId) === 'clean') return true;
    }

    const itemId = tab.state.itemId;
    const title =
      tab.kind === 'http-request' ? httpRequestTitle(tab.state) : grpcRequestTitle(tab.state);
    const modified =
      itemId !== null &&
      (tab.kind === 'http-request'
        ? !!queryClient.getQueryData(apiSavedRequestKey(itemId))
        : !!queryClient.getQueryData(apiSavedGrpcRequestKey(itemId)));
    const choice = await useConfirmDialogStore().choose({
      title: t('studio.api.tabClose.title', { name: title }),
      message: t(
        modified ? 'studio.api.tabClose.modified.body' : 'studio.api.tabClose.unsaved.body',
      ),
      confirmLabel: t('common.action.save.label'),
      secondaryLabel: t('common.action.discard.label'),
      tone: 'primary',
    });
    if (choice === 'cancel') return false;
    if (choice === 'secondary') return true;

    // The choice dialog is closed here, so an overwrite confirm inside the save can reuse it.
    const name = tab.state.name || title;
    const collectionsStore = useCollectionsStore();
    if (tab.kind === 'http-request') {
      const request = toSavedRequest(tab.state);
      if (itemId !== null && modified) {
        const against =
          seen[tabId] ?? queryClient.getQueryData<HttpSavedRequest>(apiSavedRequestKey(itemId));
        return collectionsStore.saveRequest(
          itemId,
          name,
          request,
          (against as HttpSavedRequest) ?? null,
        );
      }
      const dialog = useSaveRequestDialogStore();
      dialog.openSaveDialog(tabId, name, request);
      return dialog.waitForClose();
    }
    const request = toSavedGrpcRequest(tab.state);
    if (itemId !== null && modified) {
      const against =
        seen[tabId] ?? queryClient.getQueryData<GrpcSavedRequest>(apiSavedGrpcRequestKey(itemId));
      return collectionsStore.saveGrpcRequest(
        itemId,
        name,
        request,
        (against as GrpcSavedRequest) ?? null,
      );
    }
    const dialog = useSaveRequestDialogStore();
    dialog.openSaveGrpcDialog(tabId, name, request);
    return dialog.waitForClose();
  }

  return { seen, publishSeen, forgetSeen, closeState, confirmClose };
});

/** A request view publishes its `seenSaved` so the close path saves with the same guard. */
export function usePublishSeenSaved(
  tabId: () => string,
  seen: Ref<HttpSavedRequest | GrpcSavedRequest | null>,
): void {
  const store = useApiTabCloseStore();
  let published: string | null = null;
  watchEffect(() => {
    const id = tabId();
    const value = seen.value;
    if (value === null) return;
    store.publishSeen(id, value);
    published = id;
  });
  onScopeDispose(() => {
    if (published !== null) store.forgetSeen(published);
  });
}
