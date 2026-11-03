<script setup lang="ts">
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Label } from '@theme/components/ui/label';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import ConfigFileBar from '@workbench/config/ConfigFileBar.vue';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { computed, ref, useTemplateRef, watch } from 'vue';
import { control } from '../bridge/control';
import { useRepoConfigFile, useSaveRepoConfigFile } from './state/repoConfigQueries';

// One repo's settings file. The draft is the source while it has unsaved edits, so a push never
// resets it. Save sends the hash of the text the draft was edited from; a file changed elsewhere is
// refused with Reload and Overwrite. A repo with no file shows a template, and Save creates it.
const props = defineProps<{ path: string; file?: string }>();

const FOLDER = 'repos';

const query = useRepoConfigFile(() => ({ path: props.path, file: props.file ?? '' }));
const save = useSaveRepoConfigFile();

const draft = ref('');
const dirty = ref(false);
const baseHash = ref('');
const saveError = ref('');
const conflict = ref(false);

const view = computed(() => query.data.value);
const creating = computed(() => view.value !== undefined && !view.value.exists);
const actionable = computed(() => dirty.value && !save.isPending.value);

watch(
  view,
  (v) => {
    if (!v || dirty.value) return;
    draft.value = v.text;
    baseHash.value = v.hash;
  },
  { immediate: true },
);

async function saveNow(): Promise<void> {
  const v = view.value;
  if (!v || !(dirty.value || creating.value)) return;
  const text = draft.value;
  try {
    const next = await save.mutateAsync({
      path: v.path,
      file: v.file,
      text,
      baseHash: baseHash.value,
    });
    saveError.value = '';
    conflict.value = false;
    baseHash.value = next.hash;
    if (draft.value === text) dirty.value = false;
  } catch (err) {
    saveError.value = errorText(err);
    conflict.value = (err as { code?: string }).code === 'E_CONFLICT';
  }
}

function resetTo(v: { text: string; hash: string } | undefined): void {
  draft.value = v?.text ?? '';
  baseHash.value = v?.hash ?? '';
  dirty.value = false;
  saveError.value = '';
  conflict.value = false;
}

async function reload(): Promise<void> {
  resetTo((await query.refetch()).data);
}

async function overwrite(): Promise<void> {
  const fresh = (await query.refetch()).data;
  if (!fresh) return;
  baseHash.value = fresh.hash;
  conflict.value = false;
  dirty.value = true;
  await saveNow();
}

function onInput(v: string | number): void {
  draft.value = String(v);
  dirty.value = true;
}

const root = useTemplateRef<HTMLElement>('root');
useShortcut(root, ['form.save'], () => void saveNow());
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
      <Label for="repo-file-text" class="sr-only">{{
        t('space.repo.settingsFile.field.aria')
      }}</Label>
      <Textarea
        id="repo-file-text"
        :model-value="draft"
        spellcheck="false"
        class="min-h-72 resize-y font-data leading-relaxed"
        data-testid="repo-file-text"
        @update:model-value="onInput"
      />
      <div class="flex flex-wrap items-center justify-end gap-2">
        <Button
          variant="dialog"
          size="kira-lg"
          :disabled="!actionable"
          data-testid="repo-file-discard"
          @click="resetTo(view)"
        >
          {{ t('common.action.discard.label') }}
        </Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!dirty && !creating"
          :pending="save.isPending.value"
          data-testid="repo-file-save"
          @click="saveNow"
        >
          {{ creating ? t('space.repo.settingsFile.create.label') : t('common.action.save.label') }}
        </Button>
      </div>
    </template>
    <Alert v-if="saveError" variant="error" data-testid="repo-file-save-error">
      <AlertDescription>{{ saveError }}</AlertDescription>
      <AlertAction v-if="conflict">
        <Button variant="link" size="inline" data-testid="repo-file-reload" @click="reload">
          {{ t('common.action.refresh.label') }}
        </Button>
        <Button variant="link" size="inline" data-testid="repo-file-overwrite" @click="overwrite">
          {{ t('space.ade.workflows.conflict.overwrite.label') }}
        </Button>
      </AlertAction>
    </Alert>
  </div>
</template>
