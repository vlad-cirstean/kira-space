<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useCopy } from '@workbench/behaviour/clipboard';
import { errorText } from '@workbench/bridge/codedError';
import SaveBar from '@workbench/components/SaveBar.vue';
import ConfigFileProblem from '@workbench/config/ConfigFileProblem.vue';
import ConfigFormProblemAlert from '@workbench/config/ConfigFormProblemAlert.vue';
import SaveConflictAlert from '@workbench/config/SaveConflictAlert.vue';
import { useConfigFormDraft } from '@workbench/config/useConfigFormDraft';
import CodeTextEditor from '@workbench/editor/CodeTextEditor.vue';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { computed, onBeforeUnmount, useTemplateRef, watch } from 'vue';
import { useRepoSharedText } from '../configsync/queries';
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

const query = useRepoConfigFile(() => ({ path: props.path, file: props.file ?? '' }));
const save = useSaveRepoConfigFile();
const view = computed(() => query.data.value);
// The shared repos file of this repo, for a shared config repository: origin plus the shared keys.
const sharedText = useRepoSharedText();
const { copy: copyShared } = useCopy();
// A failed export shows through the mutation's error alert; mutate keeps it from also rejecting here.
function onCopyShared(): void {
  sharedText.mutate(props.path, { onSuccess: (text) => void copyShared(text) });
}
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
    <ConfigFileProblem
      v-if="view.exists && (!view.editable || draft.tab.value === 'yaml')"
      :error="view.problem"
    />
    <div v-if="!view.exists" class="text-kira-sm text-subtle" data-testid="repo-file-none">
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
        <ConfigFileProblem
          :error="view.problem"
          :shown="unclaimed"
          :show-yaml="() => void draft.showTab('yaml')"
        />
        <ConfigFormProblemAlert
          v-if="unclaimed"
          :problem="unclaimed"
          testid="repo-form-problem"
          show-yaml
          @show-yaml="draft.showTab('yaml')"
        />
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
      <SaveBar
        :dirty="draft.dirty.value"
        :saving="draft.saving.value"
        :can-save="draft.canSave.value"
        :save-label="creating ? t('space.repo.settingsFile.create.label') : undefined"
        testid-prefix="repo-file"
        @save="draft.saveNow"
        @discard="draft.discard"
      >
        <template #start>
          <Button
            variant="dialog"
            size="kira-lg"
            :pending="sharedText.isPending.value"
            data-testid="repo-file-copy-shared"
            @click="onCopyShared"
          >
            {{ t('space.repo.settingsFile.copyShared.label') }}
          </Button>
        </template>
      </SaveBar>
    </template>
    <Alert v-if="sharedText.error.value" variant="error" data-testid="repo-file-copy-shared-error">
      <AlertDescription>{{ errorText(sharedText.error.value) }}</AlertDescription>
    </Alert>
    <SaveConflictAlert
      v-if="draft.saveError.value"
      :error="draft.saveError.value"
      :conflict="draft.conflict.value"
      testid-prefix="repo-file"
      @reload="draft.reload"
      @overwrite="draft.overwrite"
    />
  </div>
</template>
