<script setup lang="ts">
import type {
  ApiCollectionFileModel,
  ApiEnvironmentFileModel,
  ApiFileVariable,
} from '@shared/domain/apiConfig';
import { t } from '@theme/i18n';
import ConfigFileDialog from '@workbench/config/ConfigFileDialog.vue';
import ConfigFormProblemAlert from '@workbench/config/ConfigFormProblemAlert.vue';
import { configName } from '@workbench/config/configName';
import { useConfigFormDraft } from '@workbench/config/useConfigFormDraft';
import { computed, ref, watch } from 'vue';
import ApiAgentAccessForm from './ApiAgentAccessForm.vue';
import BulkVariablesEditor from './BulkVariablesEditor.vue';
import EnvironmentFields from './EnvironmentFields.vue';
import { useApiFile, useApiForm, useSaveApiFile } from './state/apiFileQueries';
import { useApiVariablesDialogStore } from './state/apiVariablesDialog';
import UnusedSecrets from './UnusedSecrets.vue';
import {
  cleanVariable,
  FILE_FIELDS,
  VARIABLES_SECTION_POINTERS,
  type VariablesSection,
} from './variablesForm';

// One collection or environment file's variables as a form and as YAML over one draft. An
// environment file also edits its name, description and colour; a collection file edits `variables`
// only. Save sends the hash of the text the draft was edited from; a file changed elsewhere is refused
// with Reload and Overwrite. Values are edited in the Variables tab; this dialog is the file's own
// form, its `.env` view and its YAML.
type FileModel = ApiEnvironmentFileModel | ApiCollectionFileModel;

const dialogStore = useApiVariablesDialogStore();
const query = useApiFile(() => dialogStore.file);
const save = useSaveApiFile();
const applyForm = useApiForm(() => dialogStore.file ?? '');

const detail = computed(() => query.data.value);
const isEnvironment = computed(() => detail.value?.kind === 'environment');
const draft = useConfigFormDraft<FileModel>({
  file: () => (detail.value ? { text: detail.value.text, hash: detail.value.hash } : undefined),
  form: applyForm,
  save: (text, baseHash) => save.mutateAsync({ file: detail.value?.file ?? '', text, baseHash }),
  refetch: async () => {
    const fresh = (await query.refetch()).data;
    return fresh ? { text: fresh.text, hash: fresh.hash } : undefined;
  },
  fields: FILE_FIELDS,
});

const section = ref<VariablesSection>('variables');
watch(
  () => [dialogStore.file, detail.value?.kind] as const,
  () => {
    section.value =
      dialogStore.tab === 'yaml' || (isEnvironment.value && dialogStore.tab !== 'variables')
        ? dialogStore.tab
        : 'variables';
  },
  { immediate: true },
);

const sections = computed<VariablesSection[]>(() =>
  isEnvironment.value ? ['environment', 'variables', 'agent', 'yaml'] : ['variables', 'yaml'],
);
const LABEL = {
  environment: () => t('common.field.environment.label'),
  variables: () => t('common.module.variables.label'),
  agent: () => t('studio.project.connectionDialog.mcp.access.label'),
  yaml: () => t('common.yaml.label'),
} as const satisfies Record<VariablesSection, () => string>;
const items = computed(() =>
  sections.value.map((id) => ({ value: id, label: LABEL[id](), testid: `api-tab-${id}` })),
);

const model = computed(() => draft.model.value);
const variablesProblem = computed(() =>
  draft.errorFor('/variables') ? draft.problem.value : null,
);
const variables = computed<ApiFileVariable[]>({
  get: () => model.value?.variables ?? [],
  set: (v) => {
    if (draft.model.value)
      draft.model.value.variables = v.filter((x) => x.name !== '').map(cleanVariable);
  },
});

function close(): void {
  dialogStore.close();
}
</script>

<template>
  <ConfigFileDialog
    v-model:section="section"
    :draft="draft"
    :query="query"
    :title="
      t('studio.api.variablesDialog.title', {
        name: model?.name ?? configName(detail?.file ?? ''),
      })
    "
    testid-prefix="api-variables"
    :sections="items"
    :tabs-aria="t('studio.api.variablesDialog.tabs.aria')"
    :yaml-aria="t('studio.api.variablesDialog.file.aria')"
    :pointers="VARIABLES_SECTION_POINTERS"
    :last-good="detail?.lastGood"
    :last-good-hint="t('studio.api.config.lastGood.hint')"
    :problem="detail?.problem"
    @close="close"
  >
    <template #form="{ section: current }">
      <EnvironmentFields
        v-if="current === 'environment' && isEnvironment"
        v-model="draft.model.value as ApiEnvironmentFileModel"
        :error-for="draft.errorFor"
      />
      <ApiAgentAccessForm
        v-else-if="current === 'agent' && isEnvironment"
        v-model="draft.model.value as ApiEnvironmentFileModel"
        :error-for="draft.errorFor"
      />
      <div
        v-else-if="current === 'variables'"
        class="flex flex-col gap-2"
        data-testid="api-section-variables"
      >
        <ConfigFormProblemAlert
          v-if="variablesProblem"
          :problem="variablesProblem"
          testid="api-variables-error"
        />
        <BulkVariablesEditor :variables="variables" @update:variables="(v) => (variables = v)" />
      </div>
    </template>
    <template #yaml-after><UnusedSecrets /></template>
  </ConfigFileDialog>
</template>
