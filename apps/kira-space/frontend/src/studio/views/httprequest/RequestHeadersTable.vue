<script setup lang="ts">
import { headerValueCompletions, WELL_KNOWN_REQUEST_HEADERS } from '@kira/api-core';
import type { HttpHeaderState } from '@shared/domain/http';
import { t } from '@theme/i18n';
import type { VariableSupport } from '../../api/state/variableCompletion';
import { patchHttpRequestTabState } from '../../api/tabs';
import type { HttpRequestTabRecord } from '../../state/tabDomain';
import FieldRowsTable from '../shared/fields/FieldRowsTable.vue';

// D15/C6: a thin wrapper over FieldRowsTable — headers write `tab.state.headers` directly (D6:
// the one persisted array with real write-through, unlike Params which is derived from the URL).
const props = defineProps<{
  tab: HttpRequestTabRecord;
  /** P15b D4: HttpRequestView.vue's own variableSupport(...) — forwarded to the value cell. */
  variables?: VariableSupport;
  /** P16 D13: HttpRequestView.vue's own #toolbar-2 filter box, forwarded to FieldRowsTable. */
  filterQuery?: string;
  /** P22b D7: HttpRequestView.vue's own persisted description-column toggle, forwarded to
   *  FieldRowsTable. */
  showDescriptions?: boolean;
}>();

function blankHeader(): HttpHeaderState {
  return { name: '', value: '', enabled: true, description: '' };
}

function onUpdateRows(headers: HttpHeaderState[]): void {
  patchHttpRequestTabState(props.tab.id, { headers });
}
</script>

<template>
  <FieldRowsTable
    :rows="tab.state.headers"
    :blank-row="blankHeader"
    show-enabled
    :name-placeholder="t('studio.http.headers.name.placeholder')"
    :value-placeholder="t('common.field.value.label')"
    testid-prefix="http-header"
    container-testid="http-headers-table"
    :name-candidates="WELL_KNOWN_REQUEST_HEADERS"
    :value-candidates-for="headerValueCompletions"
    :value-variable-support="variables"
    :filter-query="filterQuery"
    :show-descriptions="showDescriptions"
    @update:rows="onUpdateRows"
  />
</template>
