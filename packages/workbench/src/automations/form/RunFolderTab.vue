<script setup lang="ts">
import type { RunFile } from '@shared/domain/scripts';
import { useQuery } from '@tanstack/vue-query';
import PathChip from '@theme/components/PathChip.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import type { TextPart } from '@theme/varText';
import { computed } from 'vue';
import { useAsyncAction } from '../../behaviour/asyncAction';
import { useAutomationsModule } from '../module';

// Where the run starts: a folder Kira keeps for it, or one picked folder. Kira Space adds the
// task-worktree switch.
const props = defineProps<{ id: string; errorFor: (pointer: string) => string | undefined }>();
const file = defineModel<RunFile>({ required: true });
const ctx = useAutomationsModule();
const choosing = useAsyncAction(async () => {
  const path = await ctx.chooseFolder(t('automations.dialog.script.chooseFolder.title'));
  if (path === null) return;
  file.value.path = path;
  file.value.dir = 'fixed';
});

const dirMode = computed(() => (file.value.dir === 'fixed' ? 'fixed' : 'kira'));
const dir = useQuery({
  queryKey: ['scriptRuns', 'dir', props.id],
  queryFn: () => ctx.runs.resolveDir(props.id),
  staleTime: 0,
});
const blocker = computed(() => dir.data.value?.blocker ?? '');
const folderParts = computed<TextPart[]>(() => [
  {
    name: 'folder',
    kind: 'path',
    value: `${dir.data.value?.base ?? '…'}/automations/${props.id}`,
  },
]);
const folderError = computed(() => props.errorFor('/path') ?? props.errorFor('/dir'));

const dirItems = computed(() => [
  { value: 'kira', label: t('automations.dir.kira.label'), testid: 'script-dialog-dir-kira' },
  {
    value: 'fixed',
    label: t('automations.dialog.script.chooseFolder.label'),
    testid: 'script-dialog-dir-fixed',
  },
]);

function setMode(mode: string): void {
  file.value.dir = mode === 'fixed' ? 'fixed' : '';
  if (mode !== 'fixed') file.value.path = '';
}
</script>

<template>
  <div class="flex flex-col gap-3" data-testid="run-tab-folder">
    <Field>
      <FieldLabel>{{ t('automations.dialog.script.runFolder.label') }}</FieldLabel>
      <SecondaryTabs
        variant="choice"
        size="kira-lg"
        :aria-label="t('automations.dialog.script.runFolder.label')"
        :items="dirItems"
        :model-value="dirMode"
        data-testid="script-dialog-dirmode"
        @update:model-value="setMode"
      />
      <div v-if="dirMode === 'fixed'" class="flex items-center gap-1.5">
        <span class="flex min-w-0 flex-1" data-testid="script-dialog-workingdir">
          <PathChip v-if="file.path" :path="file.path" />
          <span v-else class="truncate font-data">{{
            t('automations.dialog.script.noFolder.status')
          }}</span>
        </span>
        <Button
          variant="dialog"
          size="kira-lg"
          data-testid="script-dialog-workingdir-choose"
          @click="choosing.run()"
        >
          {{ t('common.action.choose.label') }}
        </Button>
      </div>
      <FieldDescription v-else data-testid="script-dialog-dir-preview">
        <VarText :parts="folderParts" />
      </FieldDescription>
      <FieldError v-if="blocker" data-testid="script-dialog-dir-blocker">{{ blocker }}</FieldError>
      <FieldError v-if="folderError" data-testid="script-dialog-folder-error">{{
        folderError
      }}</FieldError>
      <FieldError v-if="choosing.error.value" data-testid="script-dialog-dir-error">{{
        choosing.error.value
      }}</FieldError>
    </Field>

    <Field v-if="ctx.ade" orientation="horizontal">
      <Checkbox
        id="script-use-ade-dir"
        :model-value="file.use_ade_dir ?? false"
        data-testid="script-use-ade-dir"
        @update:model-value="(v) => (file.use_ade_dir = v === true)"
      />
      <FieldLabel for="script-use-ade-dir">{{
        t('automations.dialog.script.useAdeDir.label')
      }}</FieldLabel>
    </Field>
  </div>
</template>
