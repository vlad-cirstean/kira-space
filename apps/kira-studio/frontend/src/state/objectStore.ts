import {
  OBJECT_CONTENT_TYPE_SENTINEL,
  OBJECT_FILE_SENTINEL,
  OBJECT_KEY_SENTINEL,
} from '@shared/domain/objectStore';
import { decodePath, encodePath, pathTail } from '@shared/domain/tree';
import { t } from '@theme/i18n';
import type { MenuItem } from '@workbench/state/contextMenu';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { control } from '../bridge/control';
import { data } from '../bridge/data';
import { useConnectionsStore } from './connections';

// P33 D17: lives in state/ (not views/keyvalue/) because project/menus.ts must be able to open
// the upload dialog without importing a views/ module sideways (§11's dependency rule, F17) — the
// tree's own bucket/prefix rows are a second entry point alongside the object tab's own toolbar.
export const useObjectStoreStore = defineStore('objectStore', () => {
  const state = reactive({
    open: false,
    connectionId: null as string | null,
    /** The bucket or prefix path a successful upload lands the new object under. */
    containerPath: '',
    /** Tab the dialog was opened from, when its stale state should gate the upload; null from the tree. */
    tabId: null as string | null,
  });

  function openUploadDialog(connectionId: string, containerPath: string, tabId?: string): void {
    state.connectionId = connectionId;
    state.containerPath = containerPath;
    state.tabId = tabId ?? null;
    state.open = true;
  }

  // P33 D3/P41 D10: offered only when the connection's caps say so (fileTransfer + canInsert) and
  // it isn't read-only, never a permanently disabled row — shared by the project tree's bucket/
  // prefix rows (project/menus.ts) and the Browse panel's own container rows (views/browse/menu.ts),
  // which sit on opposite sides of the project/ -> views/ layering rule, so the item itself lives
  // here rather than in either caller.
  function uploadMenuItem(connectionId: string, containerPath: string, tabId?: string): MenuItem[] {
    const connectionsStore = useConnectionsStore();
    const caps = connectionsStore.states[connectionId]?.caps;
    const record = connectionsStore.connectionRecord(connectionId);
    if (!caps?.fileTransfer || !caps.canInsert || record?.readOnly) return [];
    return [
      {
        type: 'item',
        id: 'upload-file',
        label: t('studio.browse.upload.label'),
        icon: 'cloud-upload',
        run: () => openUploadDialog(connectionId, containerPath, tabId),
      },
    ];
  }

  function closeUploadDialog(): void {
    state.open = false;
  }

  /** Save dialog → data.objectDownload. Returns the chosen path, or null when cancelled. */
  async function downloadObject(
    connectionId: string,
    path: string,
    tabId: string | null,
  ): Promise<string | null> {
    const tail = pathTail(path);
    if (tail?.kind !== 'object') return null;
    const chosen = await control.filesChooseSave(tail.name);
    if (chosen.canceled || !chosen.filePath) return null;
    await data.objectDownload({
      tabId,
      connectionId,
      path,
      destPath: chosen.filePath,
    });
    return chosen.filePath;
  }

  /** Confirm → data.mutate delete. Callers reload/refresh; this module never touches a view's runtime. */
  async function deleteObject(
    connectionId: string,
    path: string,
    tabId: string | null,
  ): Promise<boolean> {
    const tail = pathTail(path);
    if (tail?.kind !== 'object') return false;
    await data.mutate({
      tabId,
      connectionId,
      path,
      ops: [{ kind: 'delete', key: { [OBJECT_KEY_SENTINEL]: tail.name } }],
    });
    return true;
  }

  /** data.mutate insert with the `$file` sentinel; resolves to the new object's encoded path. */
  async function uploadObject(args: {
    connectionId: string;
    containerPath: string;
    key: string;
    sourcePath: string;
    contentType: string;
    tabId: string | null;
  }): Promise<string> {
    await data.mutate({
      tabId: args.tabId,
      connectionId: args.connectionId,
      path: args.containerPath,
      ops: [
        {
          kind: 'insert',
          values: {
            [OBJECT_KEY_SENTINEL]: args.key,
            [OBJECT_FILE_SENTINEL]: args.sourcePath,
            [OBJECT_CONTENT_TYPE_SENTINEL]: args.contentType,
          },
        },
      ],
    });
    const containerSegments = decodePath(args.connectionId, args.containerPath).segments;
    return encodePath([...containerSegments, { kind: 'object', name: args.key }]);
  }

  return {
    ...toRefs(state),
    openUploadDialog,
    uploadMenuItem,
    closeUploadDialog,
    downloadObject,
    deleteObject,
    uploadObject,
  };
});
