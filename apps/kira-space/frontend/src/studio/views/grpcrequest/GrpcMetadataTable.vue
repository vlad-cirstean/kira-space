<script setup lang="ts">
import { metadataValueCompletions, WELL_KNOWN_REQUEST_METADATA } from '@kira/api-core';
import type { GrpcMetadataState } from '@shared/domain/grpc';
import { t } from '@theme/i18n';
import type { VariableSupport } from '../../api/state/variableCompletion';
import { patchGrpcRequestTabState } from '../../api/tabs';
import type { GrpcRequestTabRecord } from '../../state/tabDomain';
import FieldRowsTable from '../shared/fields/FieldRowsTable.vue';

// A thin wrapper over FieldRowsTable, same shape RequestHeadersTable.vue uses —
// replaces this package's own former MetadataTable.vue copy (the trade only held while
// views/grpcrequest/** could not import views/httprequest/**; FieldRowsTable now lives in
// views/shared/fields/, which both kinds may import).
const props = defineProps<{
  tab: GrpcRequestTabRecord;
  /** P16 D13: GrpcRequestView.vue's own #toolbar-2 filter box, forwarded to FieldRowsTable. */
  filterQuery?: string;
  /** P18 D10: GrpcRequestView.vue's own variableSupport(...), forwarded to the value cell. */
  variables?: VariableSupport;
  /** P22b D6/D7: GrpcRequestView.vue's own persisted description-column toggle. */
  showDescriptions?: boolean;
}>();

function blankMetadata(): GrpcMetadataState {
  return { name: '', value: '', enabled: true, description: '' };
}

function onUpdateRows(metadata: GrpcMetadataState[]): void {
  patchGrpcRequestTabState(props.tab.id, { metadata });
}
</script>

<template>
  <FieldRowsTable
    :rows="tab.state.metadata"
    :blank-row="blankMetadata"
    show-enabled
    :name-placeholder="t('studio.grpc.metadata.name.placeholder')"
    :value-placeholder="t('common.field.value.label')"
    testid-prefix="grpc-metadata"
    container-testid="grpc-metadata-table"
    :name-candidates="WELL_KNOWN_REQUEST_METADATA"
    :value-candidates-for="metadataValueCompletions"
    :value-variable-support="variables"
    :filter-query="filterQuery"
    :show-descriptions="showDescriptions"
    @update:rows="onUpdateRows"
  />
</template>
