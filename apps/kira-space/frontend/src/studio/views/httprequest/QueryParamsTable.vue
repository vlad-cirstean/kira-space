<script setup lang="ts">
import {
  buildQuery,
  parseQuery,
  type QueryPair,
  reconcileParamDescriptions,
  splitUrl,
} from '@kira/api-core';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import type { VariableSupport } from '../../api/state/variableCompletion';
import { patchHttpRequestTabState } from '../../api/tabs';
import type { HttpRequestTabRecord } from '../../state/tabDomain';
import FieldRowsTable from '../shared/fields/FieldRowsTable.vue';

// A derived two-way editor over the URL, never a stored `params` array — `pairs`
// is a pure computed off `tab.state.url`, so typing in the URL field re-renders this table without
// this component ever writing back; only an edit made *here* calls onUpdateRows, which rewrites
// the URL.
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

// A query param row has nowhere of its own to carry a description (there is no `params` array;
// the URL is the single source of truth for the query string). `description` is annotation
// merged in from the tab's own side-car map, keyed by name — never input to buildQuery below, so
// the two can never disagree about what is sent.
interface ParamRow extends QueryPair {
  description: string;
}

const pairs = computed<ParamRow[]>(() =>
  parseQuery(splitUrl(props.tab.state.url).query).map((p) => ({
    ...p,
    description: props.tab.state.paramDescriptions[p.name] ?? '',
  })),
);

function blankParam(): ParamRow {
  return { name: '', value: '', description: '' };
}

function onUpdateRows(next: ParamRow[]): void {
  const { base, hash } = splitUrl(props.tab.state.url);
  const realRows = next.filter((p) => p.name !== '' || p.value !== '');
  const query = buildQuery(realRows);
  const url = base + (query ? `?${query}` : '') + (hash ? `#${hash}` : '');
  const paramDescriptions = reconcileParamDescriptions(realRows);
  patchHttpRequestTabState(props.tab.id, { url, paramDescriptions });
}
</script>

<template>
  <FieldRowsTable
    :rows="pairs"
    :blank-row="blankParam"
    :name-placeholder="t('common.field.key.label')"
    :value-placeholder="t('common.field.value.label')"
    testid-prefix="http-param"
    container-testid="http-params-table"
    :value-variable-support="variables"
    :filter-query="filterQuery"
    :show-descriptions="showDescriptions"
    @update:rows="onUpdateRows"
  />
</template>
