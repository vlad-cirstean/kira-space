<script setup lang="ts">
import type { HttpUrlEncodedFieldState } from '@shared/domain/http';
import { t } from '@theme/i18n';
import type { VariableSupport } from '../../api/state/variableCompletion';
import { patchHttpRequestTabState } from '../../api/tabs';
import type { HttpRequestTabRecord } from '../../state/tabDomain';
import FieldRowsTable from '../shared/fields/FieldRowsTable.vue';

// C7: over C6's shared table, wired to state.urlEncoded — Go already serializes this mode (C1)
// and the body caption already knows its Content-Type (C5/D7).
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

function blankField(): HttpUrlEncodedFieldState {
  return { name: '', value: '', enabled: true, description: '' };
}

function onUpdateRows(urlEncoded: HttpUrlEncodedFieldState[]): void {
  patchHttpRequestTabState(props.tab.id, { urlEncoded });
}
</script>

<template>
  <FieldRowsTable
    :rows="tab.state.urlEncoded"
    :blank-row="blankField"
    show-enabled
    :name-placeholder="t('common.field.key.label')"
    :value-placeholder="t('common.field.value.label')"
    testid-prefix="http-urlencoded"
    container-testid="http-urlencoded-table"
    :value-variable-support="variables"
    :filter-query="filterQuery"
    :show-descriptions="showDescriptions"
    @update:rows="onUpdateRows"
  />
</template>
