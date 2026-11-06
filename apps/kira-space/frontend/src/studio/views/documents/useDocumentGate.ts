import { t } from '@theme/i18n';
import { formatBytes } from '@workbench/behaviour/format';
import { computed } from 'vue';
import { useConnectionsStore } from '../../state/connections';
import type { DocumentTabRecord } from '../../state/tabDomain';
import { writeDisabledReason } from '../shared/writeGate';
import { documentRow, getPage, pageVersion } from './page';
import { useDocumentViewStore } from './state';

/** Capability, read-only, projection, truncation and stale-connection gates for one document tab. */
export function useDocumentGate(tab: () => DocumentTabRecord) {
  const connectionsStore = useConnectionsStore();
  const documentViewStore = useDocumentViewStore();

  const caps = computed(() => {
    const connectionId = tab().connectionId;
    return connectionId ? (connectionsStore.states[connectionId]?.caps ?? null) : null;
  });

  const connRecord = computed(() => connectionsStore.connectionRecord(tab().connectionId));
  // Shown page outlived its connection: kept, read-only, behind StaleDataStrip.
  const stale = computed(() => documentViewStore.isStale(tab().id));
  const hasPage = computed(() => {
    void pageVersion.n;
    return !!getPage(tab().id);
  });
  const saveDisabledReason = computed(() =>
    stale.value ? t('studio.common.connectionLost.tooltip') : undefined,
  );

  const canInsert = computed(
    () => !!caps.value?.canInsert && !connRecord.value?.readOnly && !stale.value,
  );
  const canDelete = computed(
    () => !!caps.value?.canDelete && !connRecord.value?.readOnly && !stale.value,
  );
  const insertTitle = computed(() =>
    canInsert.value
      ? t('studio.documents.add.label')
      : writeDisabledReason(caps.value?.canInsert, stale.value),
  );
  const deleteTitle = computed(() =>
    canDelete.value
      ? t('common.action.delete.label')
      : writeDisabledReason(caps.value?.canDelete, stale.value),
  );

  // Task: mutate.ts's update op is a whole-document replaceOne({_id}, body) — when a projection is
  // active the fetched (and therefore edited) body holds only the projected fields, so committing
  // it would silently drop every field the projection hid. Editing is refused outright rather than
  // attempting a partial merge, since "which fields the user meant to keep" isn't recoverable from
  // a projected body alone; the fix is clearing the projection first.
  // A truncated body is a prefix of the document: a hand-closed buffer would replace the whole
  // document with it, so truncated rows are never editable.
  function editGateFor(row: number): { editable: boolean; label: string } {
    if (stale.value) {
      return { editable: false, label: t('studio.common.connectionLost.tooltip') };
    }
    if (!caps.value?.canUpdate) {
      return { editable: false, label: t('studio.documents.gate.noUpdate.tooltip') };
    }
    if (connRecord.value?.readOnly) {
      return { editable: false, label: t('studio.common.connectionReadOnly.tooltip') };
    }
    void pageVersion.n;
    if (documentRow(tab().id, row)?.isTruncated) {
      return {
        editable: false,
        label: t('studio.documents.gate.truncated.tooltip', { size: formatBytes(64 * 1024) }),
      };
    }
    if (tab().state.projection !== null) {
      return {
        editable: false,
        label: t('studio.documents.gate.projection.tooltip'),
      };
    }
    return { editable: true, label: t('common.action.edit.label') };
  }

  return {
    caps,
    connRecord,
    stale,
    hasPage,
    saveDisabledReason,
    canInsert,
    canDelete,
    insertTitle,
    deleteTitle,
    editGateFor,
  };
}
