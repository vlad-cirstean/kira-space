<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import PickerTrigger from '@theme/components/PickerTrigger.vue';
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useElementSize } from '@vueuse/core';
import { openActivePanelSearch } from '@workbench/behaviour/panelSearch';
import MenuItems from '@workbench/components/MenuItems.vue';
import ViewHead from '@workbench/components/ViewHead.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed, nextTick, ref, useTemplateRef } from 'vue';
import { useConnectionsStore } from '../state/connections';
import ApiConfigProblems from './ApiConfigProblems.vue';
import InlineValueInput from './InlineValueInput.vue';
import { useCollectionsStore } from './state/collections';
import { useMatrixModel } from './state/matrixModel';
import { useVariableSetStore, useVariablesStore } from './state/variables';
import { useVariablesMatrixStore } from './state/variablesMatrix';
import VariablesMatrix from './VariablesMatrix.vue';

// The Variables tab: every variable of the module in one grid, a row per name, a column per file
// (the collection's defaults, then each environment). Narrower than NARROW_PX it shows one column,
// picked in the toolbar. Environment values override the collection default.
const NARROW_PX = 760;

const matrix = useVariablesMatrixStore();
const variablesStore = useVariablesStore();
const variableSetStore = useVariableSetStore();
const collectionsStore = useCollectionsStore();
const connectionsStore = useConnectionsStore();
const model = useMatrixModel();

const root = useTemplateRef<HTMLElement>('root');
const { width } = useElementSize(root);
const narrow = computed(() => width.value > 0 && width.value < NARROW_PX);

const error = computed(() => matrix.error ?? variableSetStore.error ?? variablesStore.error);
function dismissError(): void {
  matrix.dismissError();
  variableSetStore.setVariableSetError(null);
  variablesStore.dismissError();
}

const hasOwners = computed(() => model.owners.value.length > 0);
// The empty state gives way to the grid as soon as Add variable is chosen or an environment exists:
// its column menu is how an empty environment is renamed, shared or deleted.
const startedAdding = ref(false);
const isEmpty = computed(
  () =>
    model.allNames.value.length === 0 &&
    model.envOwners.value.length === 0 &&
    matrix.filter.trim() === '' &&
    !startedAdding.value,
);

// ---- toolbar pickers ----

const collectionItems = computed<MenuItem[]>(() => [
  {
    type: 'radio',
    id: 'collection',
    value: model.collection.value?.id ?? null,
    items: model.collections.value.map((c) => ({
      type: 'option' as const,
      id: `collection-${c.id}`,
      testId: 'matrix-collection-option',
      value: c.id,
      label: c.name,
      run: () => matrix.focusCollection(c.id),
    })),
  },
]);

const ownerItems = computed<MenuItem[]>(() => [
  {
    type: 'radio',
    id: 'owner',
    value: model.shownOwner.value?.id ?? null,
    items: model.owners.value.map((o) => ({
      type: 'option' as const,
      id: `owner-${o.id}`,
      testId: 'matrix-env-option',
      value: o.id,
      swatch: o.scope === 'environment' ? o.color : undefined,
      label: o.name,
      run: () => {
        matrix.shownOwnerId = o.id;
      },
    })),
  },
]);

// ---- new environment: a name typed into the toolbar, Enter creates it ----

async function createEnvironment(name: string): Promise<void> {
  matrix.addingEnvironment = false;
  const created = await variablesStore.createEnvironment(name);
  if (created) matrix.shownOwnerId = created.id;
}

async function focusNewRow(): Promise<void> {
  startedAdding.value = true;
  await nextTick();
  matrix.newRowFocus++;
}

function findUsages(): void {
  if (!matrix.renamed) return;
  collectionsStore.search = `{{${matrix.renamed.from}}}`;
  matrix.renamed = null;
  openActivePanelSearch();
}
</script>

<template>
  <div ref="root" class="flex h-full min-h-0 flex-col" data-testid="variables-matrix">
    <ViewHead
      :icon="ICON.variables"
      :title="t('common.module.variables.label')"
      title-testid="variables-target"
    />
    <ViewToolbar>
      <div class="w-56"
        ><SearchField
          v-model="matrix.filter"
          :placeholder="t('studio.api.common.filterByName.placeholder')"
          data-testid="variables-filter"
        /></div
      >
      <DropdownMenu v-if="model.collections.value.length > 1" :modal="false">
        <DropdownMenuTrigger as-child>
          <PickerTrigger size="kira" data-testid="matrix-collection-picker">
            <template #leading><CodiconIcon name="library" :size="13" /></template>
            {{ model.collection.value?.name }}
          </PickerTrigger>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start"
          ><MenuItems :items="collectionItems" /></DropdownMenuContent
        >
      </DropdownMenu>
      <DropdownMenu v-if="narrow && model.owners.value.length > 0" :modal="false">
        <DropdownMenuTrigger as-child>
          <PickerTrigger size="kira" data-testid="matrix-env-picker">
            <template #leading>
              <span
                v-if="model.shownOwner.value?.scope === 'environment'"
                :class="colorMarkClass('dot', model.shownOwner.value.color)"
              />
              <CodiconIcon v-else name="library" :size="13" />
            </template>
            {{ model.shownOwner.value?.name }}
          </PickerTrigger>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start"><MenuItems :items="ownerItems" /></DropdownMenuContent>
      </DropdownMenu>
      <span class="ml-auto" />
      <InlineValueInput
        v-if="matrix.addingEnvironment"
        class="w-48"
        value=""
        :placeholder="t('common.environmentName.aria')"
        :label="t('studio.api.environments.new.label')"
        data-testid="matrix-new-env-name"
        @commit="createEnvironment"
        @cancel="matrix.addingEnvironment = false"
      />
      <Button
        v-else
        variant="toolbar"
        size="kira"
        data-testid="matrix-new-env"
        @click="matrix.addingEnvironment = true"
        ><CodiconIcon :name="ICON.environments" :size="13" />
        {{ t('studio.api.environments.new.label') }}</Button
      >
      <Button
        v-if="hasOwners"
        variant="toolbar"
        size="kira"
        data-testid="matrix-add"
        @click="focusNewRow"
        ><CodiconIcon :name="ICON.add" :size="13" />
        {{ t('common.action.addVariable.label') }}</Button
      >
    </ViewToolbar>

    <Alert v-if="error" variant="error" strip data-testid="variables-error">
      <AlertDescription>{{ error }}</AlertDescription>
      <AlertAction>
        <TooltipIconButton
          :icon="ICON.close"
          :label="t('common.action.dismiss.label')"
          data-testid="variables-error-dismiss"
          @click="dismissError"
        />
      </AlertAction>
    </Alert>
    <Alert v-else-if="matrix.renamed" variant="info" strip data-testid="variables-renamed">
      <AlertDescription>{{
        t('studio.api.variables.rename.notice.description', {
          from: matrix.renamed.from,
          to: matrix.renamed.to,
        })
      }}</AlertDescription>
      <AlertAction>
        <Button
          variant="toolbar"
          size="kira"
          data-testid="variables-find-usages"
          @click="findUsages"
          ><CodiconIcon :name="ICON.search" :size="13" />
          {{ t('studio.api.variables.rename.findUsages.label') }}</Button
        >
        <TooltipIconButton
          :icon="ICON.close"
          :label="t('common.action.dismiss.label')"
          data-testid="variables-renamed-dismiss"
          @click="matrix.renamed = null"
        />
      </AlertAction>
    </Alert>
    <Alert
      v-else-if="connectionsStore.secretStorage && !connectionsStore.secretStorage.available"
      variant="warn"
      strip
      data-testid="variables-secrets-unavailable"
    >
      <AlertDescription>{{ connectionsStore.secretStorage.reason }}</AlertDescription>
    </Alert>
    <ApiConfigProblems />

    <EmptyState
      v-if="isEmpty"
      fill
      data-testid="matrix-empty"
      :icon="ICON.variables"
      :title="t('studio.api.variables.empty.title')"
      :description="t('studio.api.variables.empty.description')"
    >
      <div class="flex gap-2">
        <Button
          v-if="hasOwners"
          variant="dialog-primary"
          size="kira-lg"
          data-testid="matrix-empty-add"
          @click="focusNewRow"
          ><CodiconIcon :name="ICON.add" :size="13" />
          {{ t('common.action.addVariable.label') }}</Button
        >
        <Button
          variant="dialog"
          size="kira-lg"
          data-testid="matrix-empty-new-env"
          @click="matrix.addingEnvironment = true"
          ><CodiconIcon :name="ICON.environments" :size="13" />
          {{ t('studio.api.environments.new.label') }}</Button
        >
        <Button
          v-if="model.collections.value.length === 0"
          variant="dialog"
          size="kira-lg"
          data-testid="matrix-empty-new-collection"
          @click="collectionsStore.createCollection()"
          ><CodiconIcon :name="ICON.newCollection" :size="13" />
          {{ t('common.newCollection.label') }}</Button
        >
      </div>
    </EmptyState>
    <template v-else>
      <VariablesMatrix :model="model" :narrow="narrow" />
      <div
        class="flex shrink-0 items-center gap-3 border-t border-border px-3 py-1.5 text-kira-sm text-subtle"
        data-testid="matrix-footer"
      >
        <span data-testid="matrix-footer-counts">{{
          t('common.commaPair.label', {
            first: t('studio.api.importReport.variables.label', {
              count: model.allNames.value.length,
            }),
            second: t('studio.api.variables.footer.environments.label', {
              count: model.envOwners.value.length,
            }),
          })
        }}</span>
        <span
          v-if="model.missingCount.value > 0"
          class="flex items-center gap-1 text-warn"
          data-testid="matrix-footer-missing"
          ><CodiconIcon :name="ICON.warning" :size="12" />
          {{
            t('studio.api.variables.footer.missing.status', { count: model.missingCount.value })
          }}</span
        >
        <span v-if="!narrow" class="ml-auto">{{
          t('studio.api.variables.footer.saved.hint')
        }}</span>
      </div>
    </template>
  </div>
</template>
