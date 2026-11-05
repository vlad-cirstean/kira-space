<script setup lang="ts">
import type { RunFile } from '@shared/domain/scripts';
import { useQuery } from '@tanstack/vue-query';
import PathChip from '@theme/components/PathChip.vue';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@theme/components/ui/radio-group';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import type { TextPart } from '@theme/varText';
import { computed, ref } from 'vue';
import { errorText } from '../../bridge/codedError';
import { useAutomationsModule } from '../module';

// Where the run starts: a folder Kira keeps for it, or one picked folder. Kira Space adds the
// task-worktree switch.
const props = defineProps<{ id: string; errorFor: (pointer: string) => string | undefined }>();
const file = defineModel<RunFile>({ required: true });
const ctx = useAutomationsModule();
const error = ref<string | null>(null);

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

function setMode(mode: string): void {
  file.value.dir = mode === 'fixed' ? 'fixed' : '';
  if (mode !== 'fixed') file.value.path = '';
}

async function choose(): Promise<void> {
  error.value = null;
  try {
    const path = await ctx.chooseFolder(t('automations.dialog.script.chooseFolder.title'));
    if (path === null) return;
    file.value.path = path;
    file.value.dir = 'fixed';
  } catch (err) {
    error.value = errorText(err);
  }
}
</script>

<template>
  <div class="flex flex-col gap-3" data-testid="run-tab-folder">
    <Field>
      <FieldLabel>{{ t('automations.dialog.script.runFolder.label') }}</FieldLabel>
      <RadioGroup
        :model-value="dirMode"
        class="gap-1.5"
        data-testid="script-dialog-dirmode"
        @update:model-value="(v) => setMode(String(v))"
      >
        <div class="flex items-center gap-2">
          <RadioGroupItem id="script-dir-kira" value="kira" data-testid="script-dialog-dir-kira" />
          <Label for="script-dir-kira">{{ t('automations.dir.kira.label') }}</Label>
        </div>
        <div class="flex items-center gap-2">
          <RadioGroupItem
            id="script-dir-fixed"
            value="fixed"
            data-testid="script-dialog-dir-fixed"
          />
          <Label for="script-dir-fixed">{{
            t('automations.dialog.script.chooseFolder.label')
          }}</Label>
        </div>
      </RadioGroup>
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
          @click="choose"
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
      <FieldError v-if="error" data-testid="script-dialog-dir-error">{{ error }}</FieldError>
    </Field>

    <Field v-if="ctx.ade" orientation="horizontal">
      <Checkbox
        id="script-use-ade-dir"
        :model-value="file.use_ade_dir ?? false"
        data-testid="script-use-ade-dir"
        @update:model-value="(v) => (file.use_ade_dir = v === true)"
      />
      <Label for="script-use-ade-dir">{{ t('automations.dialog.script.useAdeDir.label') }}</Label>
    </Field>
  </div>
</template>
