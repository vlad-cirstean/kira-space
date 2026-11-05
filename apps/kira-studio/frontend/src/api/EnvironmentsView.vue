<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { RadioGroup, RadioGroupItem } from '@theme/components/ui/radio-group';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import RunState from '@theme/RunState.vue';
import { matchesQuery } from '@workbench/behaviour/match';
import { moveId, useSortableReorder } from '@workbench/behaviour/sortableReorder';
import { errorText } from '@workbench/bridge/codedError';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, useTemplateRef } from 'vue';
import { control } from '../bridge/control';
import { useConnectionsStore } from '../state/connections';
import { useRunState } from '../state/runState';
import type { EnvironmentsTabRecord } from '../state/tabDomain';
import ApiConfigProblems from './ApiConfigProblems.vue';
import ApiSecrets from './ApiSecrets.vue';
import { useApiConfig } from './state/apiConfig';
import { useApiVariablesDialogStore } from './state/apiVariablesDialog';
import { useVariableSetStore, useVariablesStore } from './state/variables';
import { openVariableSetTab } from './tabs';

const confirmDialogStore = useConfirmDialogStore();
const variablesStore = useVariablesStore();
const variableSetStore = useVariableSetStore();
const variablesDialogStore = useApiVariablesDialogStore();
const connectionsStore = useConnectionsStore();
const config = useApiConfig();

// The environment list, re-hosted in a tab. Environments are config files (P473): a row picks the
// active one, opens its variables or its file, and shows whether the file is valid. Creating or
// duplicating one writes a new file, deleting one removes it; renaming and coloring are done in the
// file or its dialog. The order is app state: drag a row by its grip.
//
// ViewChrome/ViewHeader/RunState inlined at this call site (no library counterpart).
const props = defineProps<{ tab: EnvironmentsTabRecord }>();

const connRecord = computed(() => connectionsStore.connectionRecord(props.tab.connectionId));
const railColor = computed(() => (connRecord.value ? (connRecord.value.color ?? null) : undefined));
const runState = useRunState(() => props.tab.id);

const filterQuery = computed({
  get: () => variableSetStore.filterQueryFor(props.tab.id),
  set: (v: string) => variableSetStore.setFilterQuery(props.tab.id, v),
});
const isFiltered = computed(() => filterQuery.value.trim() !== '');
const displayEnvironments = computed(() =>
  variablesStore.environments.filter((env) => matchesQuery(env.name, filterQuery.value)),
);

// Drag by the grip, or Alt+Arrow on a focused grip: rows hold a radio and buttons, which a whole-row
// drag would start from. The radio group owns the arrow keys, hence the keyboard handle.
const listEl = useTemplateRef<HTMLElement>('listEl');
useSortableReorder(
  listEl,
  () => displayEnvironments.value.map((env) => env.id),
  (from, to) => {
    const ids = variablesStore.environments.map((env) => env.id);
    void variablesStore.reorderEnvironments(moveId(ids, from, to));
  },
  {
    draggable: '[data-testid="environment-row"]',
    handle: '[data-testid="environment-grip"]',
    direction: 'vertical',
    disabled: () => isFiltered.value,
    keyboard: true,
    keyboardHandle: '[data-testid="environment-grip"]',
  },
);

const brokenFiles = computed(
  () => new Set((config.data.value?.files ?? []).filter((f) => f.problem).map((f) => f.file)),
);

async function onNewEnvironment(): Promise<void> {
  await variablesStore.createEnvironment(t('studio.api.environments.new.label'));
}

function onEditVariables(id: string, name: string): void {
  openVariableSetTab('environment', id, name);
}

async function onOpenFile(file: string): Promise<void> {
  try {
    await control.configFilesOpen({ folder: 'api', file });
  } catch (err) {
    variablesStore.error = errorText(err);
  }
}

async function onDelete(id: string, name: string): Promise<void> {
  const confirmed = await confirmDialogStore.confirm({
    title: t('studio.api.environments.deleteConfirm.title'),
    message: [
      t('studio.api.environments.deleteConfirm.body', { name }),
      t('common.confirm.irreversible.body'),
    ].join(' '),
    confirmLabel: t('common.action.delete.label'),
    tone: 'danger',
  });
  if (!confirmed) return;
  await variablesStore.deleteEnvironment(id);
}

async function onDuplicate(id: string): Promise<void> {
  await variablesStore.duplicateEnvironment(id);
}

// The group is controlled by the store's active row, so a failed activation leaves the radio on
// the previous environment.
const activeEnvironmentId = computed(
  () => variablesStore.environments.find((e) => e.isActive)?.id ?? '',
);
async function onSetActive(id: string): Promise<void> {
  await variablesStore.setActiveEnvironment(id);
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="environments-dialog">
    <ViewToolbar>
      <span v-if="railColor !== undefined" :class="colorMarkClass('dot', railColor)" />
      <span class="size-4 flex items-center justify-center shrink-0"
        ><CodiconIcon :name="ICON.environments" :size="13" /></span
      >
      <span class="text-kira-md text-fg truncate" data-testid="environments-target">{{
        t('studio.api.common.environments.label')
      }}</span>
      <span class="ml-auto flex items-center gap-1" />
    </ViewToolbar>
    <div :class="colorMarkClass('band', railColor)" />
    <ViewToolbar>
      <div v-if="variablesStore.environments.length > 0" class="w-64"
        ><SearchField
          v-model="filterQuery"
          :placeholder="t('studio.api.common.filterByName.placeholder')"
          data-testid="environments-filter"
        /></div
      >
      <span class="ml-auto" />
      <RunState :state="runState" />
      <div class="flex items-center gap-1.5 shrink-0">
        <Button
          variant="dialog-primary"
          size="kira"
          data-testid="new-environment"
          @click="onNewEnvironment"
        >
          {{ t('studio.api.environments.new.label') }}
        </Button>
      </div>
    </ViewToolbar>

    <Alert v-if="variablesStore.error" variant="error" strip data-testid="environments-error">
      <AlertDescription>{{ variablesStore.error }}</AlertDescription>
      <AlertAction>
        <Button
          variant="toolbar"
          size="kira-icon"
          :aria-label="t('common.action.dismiss.label')"
          data-testid="environments-error-dismiss"
          @click="variablesStore.dismissError"
        >
          <CodiconIcon :name="ICON.close" :size="13" />
        </Button>
      </AlertAction>
    </Alert>

    <div class="flex flex-1 min-h-0 flex-col overflow-y-auto">
      <RadioGroup
        class="contents"
        :aria-label="t('studio.api.environments.activeGroup.aria')"
        :model-value="activeEnvironmentId"
        @update:model-value="(id) => onSetActive(String(id))"
      >
        <div ref="listEl" class="flex flex-col gap-0.5 p-1">
          <div
            v-for="env in displayEnvironments"
            :key="env.id"
            class="flex items-center gap-2 px-1.5 py-1"
            data-testid="environment-row"
            :data-id="env.id"
          >
            <span :class="colorMarkClass('dot', env.color)" data-testid="environment-color-dot" />
            <Tooltip v-if="isFiltered">
              <TooltipTrigger as-child>
                <span
                  class="flex cursor-grab items-center text-subtle"
                  aria-hidden="true"
                  data-testid="environment-grip"
                >
                  <CodiconIcon name="gripper" :size="13" />
                </span>
              </TooltipTrigger>
              <TooltipContent>{{
                t('studio.api.environments.reorderBlocked.hint')
              }}</TooltipContent>
            </Tooltip>
            <button
              v-else
              type="button"
              class="flex cursor-grab items-center text-subtle"
              :aria-label="t('studio.api.environments.grip.aria')"
              data-testid="environment-grip"
            >
              <CodiconIcon name="gripper" :size="13" />
            </button>
            <Tooltip>
              <TooltipTrigger as-child>
                <RadioGroupItem
                  :value="env.id"
                  :aria-label="t('studio.api.environments.active.aria', { name: env.name })"
                  data-testid="environment-active"
                />
              </TooltipTrigger>
              <TooltipContent>{{ t('studio.api.environments.active.tooltip') }}</TooltipContent>
            </Tooltip>
            <span class="min-w-0 flex-1 truncate text-kira-md" data-testid="environment-name">{{
              env.name
            }}</span>
            <span
              v-if="env.description"
              class="min-w-0 flex-1 truncate text-kira-sm text-subtle"
              data-testid="environment-description"
              >{{
                env.description
              }}</span
            >
            <Badge
              v-if="brokenFiles.has(env.file)"
              variant="warn"
              data-testid="environment-invalid"
              >{{
                t('studio.api.config.invalid.status')
              }}</Badge
            >
            <Button
              variant="toolbar"
              size="kira"
              data-testid="environment-open-file"
              @click="onOpenFile(env.file)"
            >
              {{ t('studio.api.config.openFile.label') }}
            </Button>
            <Button
              variant="toolbar"
              size="kira"
              data-testid="environment-edit-variables"
              @click="onEditVariables(env.id, env.name)"
            >
              {{ t('studio.api.environments.editVariables.label') }}
            </Button>
            <Button
              variant="toolbar"
              size="kira"
              data-testid="environment-edit"
              @click="variablesDialogStore.open(env.file, 'environment')"
            >
              {{ t('studio.api.variablesDialog.editEnvironment.label') }}
            </Button>
            <TooltipIconButton
              :icon="ICON.duplicate"
              :label="t('common.action.duplicate.label')"
              data-testid="environment-duplicate"
              @click="onDuplicate(env.id)"
            />
            <TooltipIconButton
              :icon="ICON.delete"
              :label="t('common.action.delete.label')"
              data-testid="environment-remove"
              @click="onDelete(env.id, env.name)"
            />
          </div>
          <EmptyState
            v-if="variablesStore.environments.length === 0"
            data-testid="environments-empty"
            :icon="ICON.environments"
            :title="t('common.noEnvironments.empty')"
          />
          <EmptyState
            v-else-if="isFiltered && displayEnvironments.length === 0"
            data-testid="environments-filter-empty"
            :icon="ICON.search"
            :title="t('common.state.noMatches.empty')"
          />
        </div>
      </RadioGroup>
      <ApiSecrets />
    </div>
    <ApiConfigProblems />
  </div>
</template>
