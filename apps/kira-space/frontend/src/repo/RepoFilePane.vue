<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import ConfigFileBar from '@workbench/config/ConfigFileBar.vue';
import { useConfigFormDraft } from '@workbench/config/useConfigFormDraft';
import CodeTextEditor from '@workbench/editor/CodeTextEditor.vue';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { computed, onBeforeUnmount, useTemplateRef, watch } from 'vue';
import { control } from '../bridge/control';
import RepoConfigForm from './RepoConfigForm.vue';
import { useRepoConfigFile, useSaveRepoConfigFile } from './state/repoConfigQueries';
import { REPO_FIELDS, REPO_FORM_POINTERS } from './state/repoFields';
import { useRepoForm } from './state/repoFormQueries';
import type { RepoFileModel } from './state/repoFormTypes';

// One repo's settings file, as a form and as YAML over one draft. Save sends the hash of the text
// the draft was edited from; a file changed elsewhere is refused with Reload and Overwrite. A repo
// with no file shows a template, and Save creates it.
const props = defineProps<{ path: string; file?: string }>();
const emit = defineEmits<{ dirty: [value: boolean] }>();

const FOLDER = 'repos';

const query = useRepoConfigFile(() => ({ path: props.path, file: props.file ?? '' }));
const save = useSaveRepoConfigFile();
const view = computed(() => query.data.value);
const creating = computed(() => view.value !== undefined && !view.value.exists);
const applyForm = useRepoForm(() => view.value?.file ?? props.file ?? '');

const draft = useConfigFormDraft<RepoFileModel>({
  file: () => (view.value ? { text: view.value.text, hash: view.value.hash } : undefined),
  form: applyForm,
  save: (text, baseHash) =>
    save.mutateAsync({
      path: view.value?.path ?? props.path,
      file: view.value?.file ?? '',
      text,
      baseHash,
    }),
  refetch: async () => {
    const fresh = (await query.refetch()).data;
    return fresh ? { text: fresh.text, hash: fresh.hash } : undefined;
  },
  fields: REPO_FIELDS,
});

// The template of a repo with no file is unsaved by nature: Save creates it.
watch(
  view,
  (v) => {
    if (v && !v.exists && !draft.dirty.value) draft.onYamlInput(v.text);
  },
  { immediate: true },
);

watch(draft.dirty, (v) => emit('dirty', v));
onBeforeUnmount(() => emit('dirty', false));

const items = computed(() => [
  { value: 'form', label: t('common.form.label'), testid: 'repo-file-tab-form' },
  { value: 'yaml', label: t('common.yaml.label'), testid: 'repo-file-tab-yaml' },
]);
const unclaimed = computed(() => draft.unmatched(REPO_FORM_POINTERS));

const root = useTemplateRef<HTMLElement>('root');
useShortcut(root, ['form.save'], () => void draft.saveNow());
</script>

<template>
  <div v-if="view" ref="root" class="flex max-w-4xl flex-col gap-3" data-testid="repo-file">
    <ConfigFileBar
      v-if="view.exists"
      :path="view.filePath"
      :error="view.problem"
      :open="() => control.configFilesOpen({ folder: FOLDER, file: view!.file })"
      :reveal="() => control.configFilesReveal({ folder: FOLDER, file: view!.file })"
    />
    <div v-else class="text-kira-sm text-subtle" data-testid="repo-file-none">
      {{ t('space.repo.settingsFile.none.hint') }}
    </div>
    <div v-if="view.lastGood" class="text-kira-sm text-subtle" data-testid="repo-file-last-good">
      {{ t('space.repo.settingsFile.lastGood.hint') }}
    </div>
    <div v-if="!view.editable" class="text-kira-sm text-subtle" data-testid="repo-file-read-only">
      {{ t('space.ade.context.readOnly.hint') }}
    </div>
    <template v-else>
      <SecondaryTabs
        :model-value="draft.tab.value"
        :items="items"
        :aria-label="t('space.repo.settingsFile.tabs.aria')"
        @update:model-value="(v) => draft.showTab(v as 'form' | 'yaml')"
      />
      <template v-if="draft.tab.value === 'form'">
        <Alert v-if="unclaimed" variant="error" data-testid="repo-form-problem">
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
              data-testid="repo-form-show-yaml"
              @click="draft.showTab('yaml')"
            >
              {{ t('common.configForm.showYaml.label') }}
            </Button>
          </AlertAction>
        </Alert>
        <fieldset
          v-if="draft.model.value"
          :disabled="draft.formDisabled.value"
          class="m-0 min-w-0 border-0 p-0"
        >
          <RepoConfigForm v-model="draft.model.value" :error-for="draft.errorFor" />
        </fieldset>
      </template>
      <CodeTextEditor
        v-else
        :model-value="draft.draft.value"
        language="yaml"
        :problem="draft.problem.value"
        :ariaLabel="t('space.repo.settingsFile.field.aria')"
        class="h-72"
        data-testid="repo-file-text"
        @update:model-value="draft.onYamlInput"
        @save="draft.saveNow"
        @submit="draft.saveNow"
      />
      <div class="flex flex-wrap items-center justify-end gap-2">
        <Button
          variant="dialog"
          size="kira-lg"
          :disabled="!draft.dirty.value"
          data-testid="repo-file-discard"
          @click="draft.discard"
        >
          {{ t('common.action.discard.label') }}
        </Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!draft.canSave.value"
          :pending="draft.saving.value"
          data-testid="repo-file-save"
          @click="draft.saveNow"
        >
          {{ creating ? t('space.repo.settingsFile.create.label') : t('common.action.save.label') }}
        </Button>
      </div>
    </template>
    <Alert v-if="draft.saveError.value" variant="error" data-testid="repo-file-save-error">
      <AlertDescription>{{ draft.saveError.value }}</AlertDescription>
      <AlertAction v-if="draft.conflict.value">
        <Button variant="link" size="inline" data-testid="repo-file-reload" @click="draft.reload">
          {{ t('common.action.refresh.label') }}
        </Button>
        <Button
          variant="link"
          size="inline"
          data-testid="repo-file-overwrite"
          @click="draft.overwrite"
        >
          {{ t('space.ade.workflows.conflict.overwrite.label') }}
        </Button>
      </AlertAction>
    </Alert>
  </div>
</template>
