<script setup lang="ts">
import type {
  ApiCollectionFileModel,
  ApiEnvironmentFileModel,
  ApiFileVariable,
} from '@shared/domain/apiConfig';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import ConfigFileBar from '@workbench/config/ConfigFileBar.vue';
import { useConfigFormDraft } from '@workbench/config/useConfigFormDraft';
import CodeTextEditor from '@workbench/editor/CodeTextEditor.vue';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';
import { computed, ref, watch } from 'vue';
import { control } from '../bridge/control';
import ApiAgentAccessForm from './ApiAgentAccessForm.vue';
import ApiVariablesTable from './ApiVariablesTable.vue';
import BulkVariablesEditor from './BulkVariablesEditor.vue';
import EnvironmentFields from './EnvironmentFields.vue';
import { useApiConfig, useSetApiSecret } from './state/apiConfig';
import { useApiFile, useApiForm, useSaveApiFile } from './state/apiFileQueries';
import { useApiVariablesDialogStore } from './state/apiVariablesDialog';
import {
  cleanVariable,
  FILE_FIELDS,
  fileStem,
  secretNameOf,
  VARIABLES_SECTION_POINTERS,
  type VariablesSection,
} from './variablesForm';

// One collection or environment file's variables as a form and as YAML over one draft. An
// environment file also edits its name, description and colour; a collection file edits `variables`
// only. Save sends the hash of the text the draft was edited from; a file changed elsewhere is refused
// with Reload and Overwrite. A typed secret goes through the write-only call after the file is written.
type FileModel = ApiEnvironmentFileModel | ApiCollectionFileModel;

const FOLDER = 'api';

const dialogStore = useApiVariablesDialogStore();
const config = useApiConfig();
const query = useApiFile(() => dialogStore.file);
const save = useSaveApiFile();
const setSecret = useSetApiSecret();
const applyForm = useApiForm(() => dialogStore.file ?? '');

const detail = computed(() => query.data.value);
const isEnvironment = computed(() => detail.value?.kind === 'environment');
const stem = computed(() => fileStem(detail.value?.file ?? ''));
const storageAvailable = computed(() => config.data.value?.storage.available ?? false);
const savedSecrets = computed(
  () => new Set((config.data.value?.secrets ?? []).filter((s) => s.set).map((s) => s.name)),
);

const typed = ref<Record<string, string>>({});
const secretError = ref('');

async function flushSecrets(): Promise<void> {
  const referenced = new Set(
    (draft.model.value?.variables ?? []).map((v) => secretNameOf(v.secret)),
  );
  for (const [name, value] of Object.entries(typed.value)) {
    if (!value || !referenced.has(name)) continue;
    await setSecret.mutateAsync({ name, value });
    typed.value = { ...typed.value, [name]: '' };
  }
}

const draft = useConfigFormDraft<FileModel>({
  file: () => (detail.value ? { text: detail.value.text, hash: detail.value.hash } : undefined),
  form: applyForm,
  save: (text, baseHash) => save.mutateAsync({ file: detail.value?.file ?? '', text, baseHash }),
  refetch: async () => {
    const fresh = (await query.refetch()).data;
    return fresh ? { text: fresh.text, hash: fresh.hash } : undefined;
  },
  fields: FILE_FIELDS,
  afterSave: flushSecrets,
});

const section = ref<VariablesSection>('variables');
const bulk = ref(false);
watch(section, (s) => {
  if (s !== 'variables') bulk.value = false;
  void draft.showTab(s === 'yaml' ? 'yaml' : 'form');
});
watch(
  () => [dialogStore.file, detail.value?.kind] as const,
  () => {
    typed.value = {};
    secretError.value = '';
    bulk.value = false;
    section.value =
      isEnvironment.value && dialogStore.tab !== 'variables' ? dialogStore.tab : 'variables';
  },
);

const sections = computed<VariablesSection[]>(() =>
  isEnvironment.value ? ['environment', 'variables', 'agent', 'yaml'] : ['variables', 'yaml'],
);
const LABEL = {
  environment: () => t('docker.field.env.label'),
  variables: () => t('common.module.variables.label'),
  agent: () => t('studio.project.connectionDialog.mcp.access.label'),
  yaml: () => t('common.yaml.label'),
} as const satisfies Record<VariablesSection, () => string>;
const items = computed(() =>
  sections.value.map((id) => ({ value: id, label: LABEL[id](), testid: `api-tab-${id}` })),
);
const unclaimed = computed(() => draft.unmatched(Object.values(VARIABLES_SECTION_POINTERS).flat()));

const model = computed(() => draft.model.value);
const variablesProblem = computed(() =>
  draft.errorFor('/variables') ? draft.problem.value : null,
);
const variables = computed<ApiFileVariable[]>({
  get: () => model.value?.variables ?? [],
  set: (v) => {
    if (draft.model.value) draft.model.value.variables = v.map(cleanVariable);
  },
});

const hasTyped = computed(() => Object.values(typed.value).some((v) => v !== ''));
const canSave = computed(() => draft.canSave.value || hasTyped.value);
const saveError = computed(() => draft.saveError.value || secretError.value);

async function saveNow(): Promise<void> {
  secretError.value = '';
  await draft.saveNow();
  if (draft.dirty.value || draft.saveError.value) return;
  try {
    await flushSecrets();
  } catch (err) {
    secretError.value = errorText(err);
  }
}

function close(): void {
  dialogStore.close();
}

const onSubmitKey = shortcutKeydown(['form.submit'], () => void saveNow());
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && close()">
    <DialogContent
      size="2xl"
      fixed-height
      :dirty="draft.dirty.value"
      data-testid="api-variables-dialog"
      @keydown="onSubmitKey"
    >
      <DialogHeader close-testid="api-variables-close">
        <DialogTitle>{{
          t('studio.api.variablesDialog.title', { name: model?.name ?? detail?.file ?? '' })
        }}</DialogTitle>
      </DialogHeader>
      <ViewToolbar v-if="detail" class="px-3">
        <SecondaryTabs
          :model-value="section"
          :items="items"
          :aria-label="t('studio.api.variablesDialog.tabs.aria')"
          @update:model-value="(v) => (section = v as VariablesSection)"
        />
      </ViewToolbar>

      <DialogBody v-if="detail">
        <div class="flex flex-col gap-3">
          <ConfigFileBar
            :path="detail.path"
            :error="detail.problem"
            :open="() => control.configFilesOpen({ folder: FOLDER, file: detail!.file })"
            :reveal="() => control.configFilesReveal({ folder: FOLDER, file: detail!.file })"
            :about="() => control.configFilesOpen({ folder: FOLDER, file: 'AGENTS.md' })"
          />
          <div
            v-if="detail.lastGood"
            class="text-kira-sm text-subtle"
            data-testid="api-variables-last-good"
          >
            {{ t('studio.api.config.lastGood.hint') }}
          </div>
          <template v-if="section !== 'yaml'">
            <Alert v-if="unclaimed" variant="error" data-testid="api-variables-problem">
              <AlertDescription>{{
                t('common.configForm.problem.status', {
                  line: unclaimed.line,
                  message: unclaimed.message,
                })
              }}</AlertDescription>
              <AlertAction>
                <Button
                  variant="link"
                  size="inline"
                  data-testid="api-variables-show-yaml"
                  @click="section = 'yaml'"
                >
                  {{ t('common.configForm.showYaml.label') }}
                </Button>
              </AlertAction>
            </Alert>
            <fieldset
              v-if="model"
              :disabled="draft.formDisabled.value"
              class="m-0 min-w-0 border-0 p-0"
            >
              <EnvironmentFields
                v-if="section === 'environment' && isEnvironment"
                v-model="draft.model.value as ApiEnvironmentFileModel"
                :error-for="draft.errorFor"
              />
              <ApiAgentAccessForm
                v-else-if="section === 'agent' && isEnvironment"
                v-model="draft.model.value as ApiEnvironmentFileModel"
                :error-for="draft.errorFor"
              />
              <div
                v-else-if="section === 'variables'"
                class="flex flex-col gap-2"
                data-testid="api-section-variables"
              >
                <Alert v-if="variablesProblem" variant="error" data-testid="api-variables-error">
                  <AlertDescription>{{
                    t('common.configForm.problem.status', {
                      line: variablesProblem.line,
                      message: variablesProblem.message,
                    })
                  }}</AlertDescription>
                </Alert>
                <div class="flex justify-end px-1.5">
                  <Button
                    variant="toolbar"
                    size="kira"
                    :aria-pressed="bulk"
                    data-testid="api-variables-bulk-toggle"
                    @click="bulk = !bulk"
                  >
                    {{ t('studio.api.variablesDialog.bulk.toggle.label') }}
                  </Button>
                </div>
                <BulkVariablesEditor
                  v-if="bulk"
                  :variables="variables"
                  @update:variables="(v) => (variables = v)"
                  @update:typed="(v) => (typed = { ...typed, ...v })"
                />
                <ApiVariablesTable
                  v-else
                  v-model="variables"
                  v-model:typed="typed"
                  :stem="stem"
                  :saved-secrets="savedSecrets"
                  :storage-available="storageAvailable"
                />
                <div
                  v-if="variables.length === 0 && !bulk"
                  class="px-1.5 text-kira-sm text-subtle"
                  data-testid="api-variables-empty"
                >
                  {{ t('studio.api.config.variables.empty') }}
                </div>
              </div>
            </fieldset>
          </template>
          <CodeTextEditor
            v-else
            :model-value="draft.draft.value"
            language="yaml"
            :problem="draft.problem.value"
            :ariaLabel="t('studio.api.variablesDialog.file.aria')"
            class="h-72"
            data-testid="api-variables-text"
            @update:model-value="draft.onYamlInput"
            @save="saveNow"
            @submit="saveNow"
          />
          <Alert v-if="saveError" variant="error" data-testid="api-variables-save-error">
            <AlertDescription>{{ saveError }}</AlertDescription>
            <AlertAction v-if="draft.conflict.value">
              <Button
                variant="link"
                size="inline"
                data-testid="api-variables-reload"
                @click="draft.reload"
              >
                {{ t('common.action.refresh.label') }}
              </Button>
              <Button
                variant="link"
                size="inline"
                data-testid="api-variables-overwrite"
                @click="draft.overwrite"
              >
                {{ t('space.ade.workflows.conflict.overwrite.label') }}
              </Button>
            </AlertAction>
          </Alert>
        </div>
      </DialogBody>

      <DialogFooter>
        <Button
          variant="dialog"
          size="kira-lg"
          :disabled="!draft.dirty.value"
          data-testid="api-variables-discard"
          @click="draft.discard"
        >
          {{ t('common.action.discard.label') }}
        </Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!canSave"
          :pending="draft.saving.value"
          data-testid="api-variables-save"
          @click="saveNow"
        >
          {{ t('common.action.save.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
