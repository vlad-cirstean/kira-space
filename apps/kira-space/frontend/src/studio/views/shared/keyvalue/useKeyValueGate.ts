import { pathTail } from '@shared/domain/tree';
import { OBJECT_BODY_EDIT_BYTES } from '@shared/protocol/page';
import { t } from '@theme/i18n';
import { formatBytes } from '@workbench/behaviour/format';
import { computed } from 'vue';
import { useConnectionsStore } from '../../../state/connections';
import { writeDisabledReason } from '../writeGate';
import { keyValueHost } from './host';
import { getPage, keyValueRow, pageVersion } from './page';
import { useKeyValueViewStore } from './state';

interface ObjectEditGate {
  editable: boolean;
  reason: string;
}

/** Capability, read-only and object-size gates for one key/object view, shared by its toolbar and pane. */
export function useKeyValueGate(viewKey: () => string) {
  const connectionsStore = useConnectionsStore();
  const keyValueViewStore = useKeyValueViewStore();

  const host = computed(() => keyValueHost(viewKey()));
  const rt = computed(() => keyValueViewStore.runtime[viewKey()]);
  const page = computed(() => {
    void pageVersion.n;
    return getPage(viewKey());
  });
  const connRecord = computed(() =>
    connectionsStore.connectionRecord(host.value?.connectionId ?? null),
  );
  const caps = computed(() =>
    host.value?.connectionId
      ? (connectionsStore.states[host.value.connectionId]?.caps ?? null)
      : null,
  );

  // The path's 'key' segment carries the full namespace-joined redis key name verbatim.
  const targetTail = computed(() => pathTail(host.value?.path ?? ''));
  const keyName = computed(() => targetTail.value?.name ?? '');

  // Shown page outlived its connection: kept, read-only, behind StaleDataStrip.
  const stale = computed(() => keyValueViewStore.isStale(viewKey()));

  const canUpdate = computed(
    () => !!caps.value?.canUpdate && !connRecord.value?.readOnly && !stale.value,
  );
  const canDelete = computed(
    () => !!caps.value?.canDelete && !connRecord.value?.readOnly && !stale.value,
  );
  const canInsert = computed(
    () => !!caps.value?.canInsert && !connRecord.value?.readOnly && !stale.value,
  );
  const canDownload = computed(() => !!caps.value?.fileTransfer);

  // An S3 object page lists exactly what read.ts pushed; there is nothing to paginate.
  const isSingleObjectPage = computed(() => page.value?.redisType === 'object');
  // Only string keys edit: other types need per-element mutations, not a whole-key replace.
  const editableType = computed(() => page.value?.redisType === 'string');

  function rowAt(i: number) {
    void pageVersion.n;
    return keyValueRow(viewKey(), i);
  }

  // The Body row exists only for objects at or under SINGLE_ROW_MAX_BYTES; absence means too large.
  const objectBodyRow = computed(() => {
    const p = page.value;
    if (p?.redisType !== 'object') return null;
    for (let i = 0; i < p.rowCount; i++) {
      const row = rowAt(i);
      if (row?.field === 'Body')
        return { index: i, value: row.value, isTruncated: row.isTruncated };
    }
    return null;
  });

  const objectEditGate = computed<ObjectEditGate>(() => {
    if (stale.value) {
      return { editable: false, reason: t('studio.common.connectionLost.tooltip') };
    }
    if (!canUpdate.value) {
      return { editable: false, reason: writeDisabledReason(caps.value?.canUpdate, stale.value) };
    }
    const bodyRow = objectBodyRow.value;
    if (bodyRow === null) {
      return { editable: false, reason: t('studio.keyValue.objectGate.tooLarge.hint') };
    }
    if (bodyRow.isTruncated) {
      return { editable: false, reason: t('studio.keyValue.objectGate.partial.hint') };
    }
    if (bodyRow.value.includes('�')) {
      return { editable: false, reason: t('studio.keyValue.objectGate.invalidText.hint') };
    }
    const size = page.value?.memoryBytes ?? null;
    if (size !== null && size > OBJECT_BODY_EDIT_BYTES) {
      return {
        editable: false,
        reason: t('studio.keyValue.objectGate.sizeLimit.hint', {
          size: formatBytes(size),
          limit: formatBytes(OBJECT_BODY_EDIT_BYTES),
        }),
      };
    }
    return { editable: true, reason: t('studio.keyValue.editValue.label') };
  });

  const editTitle = computed(() => {
    if (isSingleObjectPage.value) return objectEditGate.value.reason;
    if (!canUpdate.value) return writeDisabledReason(caps.value?.canUpdate, stale.value);
    if (!editableType.value) return t('studio.keyValue.editTypeUnsupported.tooltip');
    return t('studio.keyValue.editValue.label');
  });
  const editDisabled = computed(() =>
    isSingleObjectPage.value
      ? !objectEditGate.value.editable
      : !canUpdate.value || !editableType.value,
  );
  const addTitle = computed(() => {
    if (!canInsert.value) return writeDisabledReason(caps.value?.canInsert, stale.value);
    return t(
      isSingleObjectPage.value ? 'studio.keyValue.upload.tooltip' : 'studio.keyValue.add.tooltip',
    );
  });
  const deleteTitle = computed(() => {
    if (!canDelete.value) return writeDisabledReason(caps.value?.canDelete, stale.value);
    return t(
      isSingleObjectPage.value
        ? 'studio.keyValue.deleteObject.tooltip'
        : 'studio.keyValue.deleteKey.tooltip',
    );
  });
  const downloadTitle = computed(() =>
    canDownload.value
      ? t('studio.keyValue.downloadObject.tooltip')
      : t('studio.common.writeUnsupported.tooltip'),
  );

  return {
    host,
    rt,
    page,
    stale,
    connRecord,
    targetTail,
    keyName,
    canUpdate,
    canDelete,
    canInsert,
    canDownload,
    isSingleObjectPage,
    editableType,
    objectBodyRow,
    objectEditGate,
    editTitle,
    editDisabled,
    addTitle,
    deleteTitle,
    downloadTitle,
    rowAt,
  };
}
