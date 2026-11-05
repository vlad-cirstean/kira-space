<script setup lang="ts">
import type { PaletteColor } from '@shared/domain/color';
import type { ApiVariable } from '@shared/domain/variables';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { tableHeadClass, tableHeadRowClass } from '@theme/components/ui/table';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { cn } from '@theme/lib/utils';
import RunState from '@theme/RunState.vue';
import { matchesQuery } from '@workbench/behaviour/match';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import ConfigFileBar from '@workbench/config/ConfigFileBar.vue';
import { computed } from 'vue';
import { control } from '../bridge/control';
import { useConnectionsStore } from '../state/connections';
import { useRunState } from '../state/runState';
import type { VariableSetTabRecord } from '../state/tabDomain';
import { useApiConfig } from './state/apiConfig';
import { useVariableRows } from './state/apiQueries';
import { useApiVariablesDialogStore } from './state/apiVariablesDialog';
import { useCollectionsStore } from './state/collections';
import { useVariableSetStore, useVariablesStore } from './state/variables';
import VariableRow from './VariableRow.vue';

// A variable set is a config file (P473): this tab shows what the app reads from it, with the
// file's path and validity on top. Edit the file to change a variable; a secret's value is set in
// the environments tab's Secrets section.
//
// `data-testid="variables-dialog"`/`"variables-filter"`/`"variables-error"` keep the names the
// dialog this tab replaced used.
const props = defineProps<{ tab: VariableSetTabRecord }>();

const collectionsStore = useCollectionsStore();
const variablesStore = useVariablesStore();
const variableSetStore = useVariableSetStore();
const connectionsStore = useConnectionsStore();
const config = useApiConfig();
const variablesDialogStore = useApiVariablesDialogStore();

const scope = computed(() => props.tab.state.scope);
const ownerId = computed(() => props.tab.state.ownerId);

// A restored tab whose owner no longer exists renders an EmptyState rather than an empty table,
// gated on the owner list actually having loaded so a fresh tab doesn't flash "no longer exists".
const ownersLoaded = computed(() =>
  scope.value === 'collection' ? collectionsStore.loaded : variablesStore.loaded,
);
const owningCollection = computed(() =>
  scope.value === 'collection' ? collectionsStore.collectionRecord(ownerId.value) : undefined,
);
const owningEnvironment = computed(() =>
  scope.value === 'environment'
    ? variablesStore.environments.find((e) => e.id === ownerId.value)
    : undefined,
);
const ownerExists = computed(() =>
  scope.value === 'collection' ? !!owningCollection.value : !!owningEnvironment.value,
);

const fileName = computed(
  () => owningCollection.value?.file ?? owningEnvironment.value?.file ?? '',
);
const fileInfo = computed(() => config.data.value?.files.find((f) => f.file === fileName.value));

const connRecord = computed(() => connectionsStore.connectionRecord(props.tab.connectionId));
const railColor = computed<PaletteColor | null | undefined>(() => {
  if (scope.value === 'environment') return owningEnvironment.value?.color ?? 'none';
  return connRecord.value ? (connRecord.value.color ?? null) : undefined;
});
const runState = useRunState(() => props.tab.id);

const rowsQuery = useVariableRows(scope.value, ownerId);
const rows = computed<ApiVariable[]>(() => rowsQuery.data.value ?? []);
const error = computed(() => variableSetStore.variableSetError(props.tab.id));

const headCell = cn(tableHeadClass, 'px-0');
const filterQuery = computed({
  get: () => variableSetStore.filterQueryFor(props.tab.id),
  set: (v: string) => variableSetStore.setFilterQuery(props.tab.id, v),
});
const isFiltered = computed(() => filterQuery.value.trim() !== '');
const displayRows = computed<ApiVariable[]>(() =>
  rows.value.filter((row) => matchesQuery(row.name, filterQuery.value)),
);

function onRevealError(message: string): void {
  variableSetStore.setVariableSetError(props.tab.id, message);
}

function onReveal(id: string): void {
  void variableSetStore.revealVariable(id, onRevealError);
}

function onHistoryClickFor(row: ApiVariable): void {
  void variableSetStore.openHistoryMenu(props.tab.id, scope.value, ownerId.value, row.id);
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="variables-dialog" :data-scope="scope">
    <ViewToolbar>
      <span v-if="railColor !== undefined" :class="colorMarkClass('dot', railColor)" />
      <span class="size-4 flex items-center justify-center shrink-0">
        <CodiconIcon
          :name="scope === 'environment' ? ICON.environments : ICON.variables"
          :size="13"
        />
      </span>
      <span class="text-kira-md text-fg truncate" data-testid="variable-set-target">{{
        tab.state.name || t('common.module.variables.label')
      }}</span>
      <span class="ml-auto flex items-center gap-1" />
    </ViewToolbar>
    <div :class="colorMarkClass('band', railColor)" />
    <ViewToolbar v-if="ownerExists && fileName">
      <ConfigFileBar
        :path="fileInfo?.path ?? fileName"
        :error="fileInfo?.problem ?? null"
        :open="() => control.configFilesOpen({ folder: 'api', file: fileName })"
        :reveal="() => control.configFilesReveal({ folder: 'api', file: fileName })"
        :about="() => control.configFilesOpen({ folder: 'api', file: 'AGENTS.md' })"
      />
    </ViewToolbar>
    <ViewToolbar>
      <div class="w-64"
        ><SearchField
          v-model="filterQuery"
          :placeholder="t('studio.api.common.filterByName.placeholder')"
          data-testid="variables-filter"
        /></div
      >
      <span class="ml-auto" />
      <RunState :state="runState" />
      <Button
        v-if="ownerExists && fileName"
        variant="toolbar"
        size="kira"
        data-testid="variables-edit"
        @click="variablesDialogStore.open(fileName)"
      >
        {{ t('common.action.edit.label') }}
      </Button>
    </ViewToolbar>

    <EmptyState
      v-if="ownersLoaded && !ownerExists"
      data-testid="variable-set-orphan"
      icon="warning"
      :title="t('studio.api.variableSet.orphan.empty')"
    />
    <div
      v-else
      class="flex flex-col gap-0.5 p-1 flex-1 min-h-0 overflow-y-auto"
      data-testid="variables-list"
    >
      <Alert v-if="error" variant="error" data-testid="variables-error">
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
      <Alert
        v-else-if="connectionsStore.secretStorage && !connectionsStore.secretStorage.available"
        variant="warn"
        data-testid="variables-secrets-unavailable"
      >
        <AlertDescription>{{ connectionsStore.secretStorage.reason }}</AlertDescription>
      </Alert>
      <div v-if="fileInfo?.lastGood" class="px-1.5 text-kira-sm text-subtle">
        {{ t('studio.api.config.lastGood.hint') }}
      </div>

      <div :class="cn(tableHeadRowClass, 'grid gap-1 px-1.5 grid-cols-[1.2fr_2fr_1.5fr_auto]')">
        <span :class="headCell">{{ t('common.field.name.label') }}</span>
        <span :class="headCell">{{ t('common.field.value.label') }}</span>
        <span :class="headCell">{{ t('common.field.description.label') }}</span>
        <span></span>
      </div>
      <VariableRow
        v-for="row in displayRows"
        :key="row.id"
        :row="row"
        :revealed="variableSetStore.revealedValues[row.id]"
        @reveal="onReveal(row.id)"
        @history="onHistoryClickFor(row)"
      />
      <EmptyState
        v-if="rows.length === 0 && !rowsQuery.isPending.value"
        data-testid="variables-empty"
        :icon="ICON.variables"
        :title="t('studio.api.config.variables.empty')"
      />
      <EmptyState
        v-else-if="isFiltered && displayRows.length === 0"
        data-testid="variables-filter-empty"
        :icon="ICON.search"
        :title="t('common.state.noMatches.empty')"
      />
    </div>
  </div>
</template>
