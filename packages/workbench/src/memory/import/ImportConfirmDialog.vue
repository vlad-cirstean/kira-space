<script setup lang="ts">
import type { MemoryScope } from '@shared/domain/memory';
import { SOURCE_TAG_PATTERN } from '@shared/domain/memoryImport';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldError } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { storeToRefs } from 'pinia';
import { computed, ref } from 'vue';
import { formatDuration } from '../../behaviour/format';
import { errorText } from '../../bridge/codedError';
import { ORIGIN_LABEL } from '../labels';
import ScopeField from '../ScopeField.vue';
import { useMemoryUiStore } from '../store';
import { useImportAction, useImportJob, useStartImport } from './importQueries';
import { useImportUiStore } from './importStore';

// P211: the scan result and its cost estimate, confirmed before any agent call is made.
const emit = defineEmits<{ close: [] }>();
const ui = useImportUiStore();
const memoryUi = useMemoryUiStore();
const { confirmJobId } = storeToRefs(ui);
const detail = useImportJob(confirmJobId);
const act = useImportAction();
const startMutation = useStartImport();
const actError = computed(() => {
  if (act.isError.value) return errorText(act.error.value);
  return startMutation.isError.value ? errorText(startMutation.error.value) : '';
});

const ORIGINS = ['user', 'external'] as const;
const SCOPES: readonly MemoryScope[] = ['personal', 'project'];
const originItems = ORIGINS.map((value) => ({
  value,
  label: t(ORIGIN_LABEL[value]),
  testid: `import-origin-${value}`,
}));
const origin = ref<(typeof ORIGINS)[number]>('external');
const sourceTag = ref('doc');
const scope = ref<MemoryScope>('personal');
const project = ref('');
const writeDoc = ref(false);
const dirty = computed(
  () =>
    ready.value &&
    (origin.value !== 'external' ||
      sourceTag.value !== 'doc' ||
      scope.value !== 'personal' ||
      writeDoc.value),
);
const tagValid = computed(
  () => origin.value !== 'external' || SOURCE_TAG_PATTERN.test(sourceTag.value),
);
const projectValid = computed(() => scope.value !== 'project' || project.value.trim() !== '');

const job = computed(() => detail.data.value?.job);
const scanning = computed(() => !job.value || job.value.state === 'scanning');
const empty = computed(() => job.value?.state === 'awaiting' && job.value.estimate.files === 0);
const ready = computed(() => job.value?.state === 'awaiting' && job.value.estimate.files > 0);
const failed = computed(() => job.value?.state === 'failed');

async function start(): Promise<void> {
  const id = confirmJobId.value;
  if (!id) return;
  try {
    await startMutation.mutateAsync({
      id,
      origin: origin.value,
      sourceTag: origin.value === 'external' ? sourceTag.value : '',
      scope: scope.value === 'project' ? 'project' : 'personal',
      project: scope.value === 'project' ? project.value.trim() : '',
      writeDoc: writeDoc.value,
    });
  } catch {
    return;
  }
  ui.selectedJobId = id;
  memoryUi.view = 'imports';
  emit('close');
}

async function dismiss(): Promise<void> {
  const id = confirmJobId.value;
  if (id && job.value && job.value.state !== 'running') {
    try {
      await act.mutateAsync({ action: 'discard', id });
    } catch {
      // The job stays listed in the imports view; closing still proceeds.
    }
  }
  emit('close');
}
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && dismiss()">
    <DialogContent size="md" :dirty="dirty" data-testid="import-confirm-dialog">
      <DialogHeader>
        <DialogTitle>{{ t('memory.import.documents.label') }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <Alert v-if="actError" variant="error" data-testid="import-confirm-error">
          <AlertDescription>{{ actError }}</AlertDescription>
        </Alert>
        <p v-if="scanning" class="m-0 text-muted-foreground" data-testid="import-scanning">{{
          t('memory.import.confirm.scanning.loading')
        }}</p>
        <Alert v-else-if="failed" variant="error">
          <AlertDescription>{{ job?.reason }}</AlertDescription>
        </Alert>
        <p v-else-if="empty" class="m-0" data-testid="import-nothing">
          {{
            job && job.totals.skippedFiles > 0
              ? t('memory.import.confirm.nothingSkipped.description', {
                  count: job.totals.skippedFiles,
                })
              : t('memory.import.confirm.nothing.description')
          }}
        </p>
        <template v-else-if="ready && job">
          <p class="m-0" data-testid="import-estimate">
            {{
              t('memory.import.confirm.estimate.description', {
                files: t('common.fileCount.label', { count: job.estimate.files }),
                chunks: t('memory.import.confirm.chunks.label', { count: job.estimate.chunks }),
                calls: t('memory.import.confirm.calls.label', { count: job.estimate.calls }),
                duration: formatDuration(job.estimate.seconds * 1000),
              })
            }}
          </p>
          <p class="m-0 text-muted-foreground">
            {{
              t('memory.import.confirm.cost.description', {
                skipped: t('memory.import.confirm.skipped.label', {
                  count: job.totals.skippedFiles,
                }),
                ignored: t('memory.import.confirm.ignored.label', { count: job.ignoredCount }),
              })
            }}
          </p>
          <p v-if="job.truncated" class="m-0 text-warn" data-testid="import-truncated">
            {{ t('memory.import.confirm.truncated.description', { count: job.estimate.files }) }}
          </p>
          <div class="flex flex-col gap-1">
            <Label>{{ t('memory.import.confirm.origin.label') }}</Label>
            <SecondaryTabs
              v-model="origin"
              :items="originItems"
              variant="segmented"
              size="kira-lg"
              :aria-label="t('memory.import.confirm.origin.label')"
              data-testid="import-origin"
            />
          </div>
          <div v-if="origin === 'external'" class="flex flex-col gap-1">
            <Label for="import-source-tag">{{ t('memory.import.confirm.sourceTag.label') }}</Label>
            <Input
              id="import-source-tag"
              v-model="sourceTag"
              class="font-data"
              :aria-invalid="!tagValid"
              data-testid="import-source-tag"
            />
            <FieldError v-if="!tagValid" data-testid="import-source-tag-error">{{
              t('memory.import.confirm.sourceTag.error')
            }}</FieldError>
          </div>
          <ScopeField
            v-model:scope="scope"
            v-model:project="project"
            prefix="import"
            :scopes="SCOPES"
          />
          <div class="flex items-center gap-1.5">
            <Checkbox
              id="import-write-doc"
              class="size-3.5"
              :model-value="writeDoc"
              data-testid="import-write-doc"
              @update:model-value="(v) => (writeDoc = v === true)"
            >
              <CodiconIcon name="check" :size="12" />
            </Checkbox>
            <Label for="import-write-doc">{{ t('memory.import.confirm.writeDoc.label') }}</Label>
          </div>
        </template>
      </DialogBody>
      <DialogFooter>
        <Button
          variant="dialog"
          size="kira-lg"
          data-testid="import-confirm-cancel"
          @click="dismiss"
          >{{
            t('common.action.cancel.label')
          }}</Button
        >
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :pending="startMutation.isPending.value"
          :disabled="!ready || !tagValid || !projectValid"
          data-testid="import-confirm-start"
          @click="start"
        >
          {{ t('memory.import.start.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
