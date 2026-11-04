<script setup lang="ts">
import type { MemoryOrigin, MemoryScope } from '@shared/domain/memory';
import type { ImportAction } from '@shared/domain/memoryImport';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { FieldDescription } from '@theme/components/ui/field';
import { Progress } from '@theme/components/ui/progress';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { storeToRefs } from 'pinia';
import { computed, useTemplateRef } from 'vue';
import { useVirtualRows } from '../../behaviour/virtualRows';
import { errorText } from '../../bridge/codedError';
import Identifier from '../../components/Identifier.vue';
import { useRowHeight } from '../../lib/rowHeight';
import { originText, SCOPE_LABEL } from '../labels';
import ImportFileRow from './ImportFileRow.vue';
import { JOB_STATE_LABEL, jobTitle } from './importFormat';
import { useImportAction, useImportJob, useRetryImportFile, useStartImport } from './importQueries';
import { useImportUiStore } from './importStore';

// P211: one import — state, totals, job actions and its virtualized file list.
const props = defineProps<{ jobId: string }>();
const ui = useImportUiStore();
const { selectedFileId } = storeToRefs(ui);
const detail = useImportJob(() => props.jobId);
const act = useImportAction();
const start = useStartImport();
const retryFile = useRetryImportFile();
const { confirm } = useConfirmDialogStore();

const job = computed(() => detail.data.value?.job);
const files = computed(() => detail.data.value?.files ?? []);
const selectedFile = computed(() => files.value.find((f) => f.id === selectedFileId.value));
const percent = computed(() => {
  const p = job.value?.progress;
  return p && p.chunksTotal > 0 ? (p.chunksDone / p.chunksTotal) * 100 : 0;
});

const scrollRef = useTemplateRef<HTMLDivElement>('scroll');
const { single: rowHeight } = useRowHeight();
const { virtualItems, totalSize } = useVirtualRows({
  count: () => files.value.length,
  rowHeight: () => rowHeight.value,
  scrollElement: scrollRef,
});
const actError = computed(() => {
  if (act.isError.value) return errorText(act.error.value);
  return start.isError.value ? errorText(start.error.value) : '';
});

function startJob(): void {
  start.mutate({ id: props.jobId });
}

function run(action: ImportAction): void {
  act.mutate({ action, id: props.jobId });
}

async function discard(): Promise<void> {
  const ok = await confirm({
    title: t('memory.import.discardConfirm.title'),
    message: t('memory.import.discardConfirm.body'),
    confirmLabel: t('common.action.discard.label'),
    tone: 'danger',
  });
  if (ok) run('discard');
}
</script>

<template>
  <div
    v-if="job"
    class="flex h-full min-h-0 flex-col"
    data-testid="import-job"
    :data-state="job.state"
  >
    <div class="flex shrink-0 flex-col gap-1.5 p-3">
      <div class="flex items-center gap-1.5">
        <Tooltip>
          <TooltipTrigger as-child>
            <span class="min-w-0 flex-1 truncate text-kira-md font-medium">{{
              jobTitle(job)
            }}</span>
          </TooltipTrigger>
          <TooltipContent>{{ job.roots.join(', ') }}</TooltipContent>
        </Tooltip>
        <span class="text-kira-sm text-muted-foreground" data-testid="import-job-state">{{
          t(JOB_STATE_LABEL[job.state])
        }}</span>
        <Tooltip>
          <TooltipTrigger as-child>
            <Badge variant="default" data-testid="import-job-channel">{{
              t(
                job.channel === 'mcp'
                  ? 'memory.common.origin.agent.label'
                  : 'memory.import.channel.ui.label',
              )
            }}</Badge>
          </TooltipTrigger>
          <TooltipContent v-if="job.sessionId">{{
            t('memory.import.channel.agent.description', { session: job.sessionId })
          }}</TooltipContent>
        </Tooltip>
        <Badge variant="default" data-testid="import-job-origin">{{
          originText(job.origin as MemoryOrigin, job.sourceTag)
        }}</Badge>
        <Badge variant="default" data-testid="import-job-scope">{{
          job.project ? job.project : t(SCOPE_LABEL[job.scope as MemoryScope])
        }}</Badge>
        <Button
          v-if="job.state === 'awaiting'"
          variant="toolbar"
          size="kira"
          data-testid="import-start"
          @click="startJob"
          >{{
            t('common.action.start.label')
          }}</Button
        >
        <Button
          v-if="job.state === 'running'"
          variant="toolbar"
          size="kira"
          data-testid="import-pause"
          @click="run('pause')"
          >{{
            t('memory.common.pause.label')
          }}</Button
        >
        <Button
          v-if="job.state === 'paused'"
          variant="toolbar"
          size="kira"
          data-testid="import-resume"
          @click="run('resume')"
          >{{
            t('common.action.resume.label')
          }}</Button
        >
        <Button
          v-if="job.state === 'running' || job.state === 'paused'"
          variant="toolbar"
          size="kira"
          data-testid="import-cancel"
          @click="run('cancel')"
          >{{
            t('common.action.cancel.label')
          }}</Button
        >
        <Button
          v-if="job.totals.failedFiles > 0 && job.state !== 'running'"
          size="kira"
          variant="toolbar"
          data-testid="import-retry-failed"
          @click="run('retryFailed')"
        >
          {{ t('memory.import.retryFailed.label') }}
        </Button>
        <Button
          v-if="job.state === 'done' || job.state === 'cancelled' || job.state === 'failed'"
          variant="toolbar"
          size="kira"
          data-testid="import-dismiss"
          @click="run('dismiss')"
          >{{
            t('common.action.dismiss.label')
          }}</Button
        >
        <Button
          v-if="job.state !== 'running'"
          variant="toolbar"
          size="kira"
          data-testid="import-discard"
          @click="discard"
          >{{
            t('common.action.discard.label')
          }}</Button
        >
      </div>
      <Alert
        v-if="
          job.reason &&
          (job.state === 'paused' || job.state === 'failed' || job.state === 'awaiting')
        "
        :variant="job.state === 'awaiting' ? 'warn' : 'error'"
        data-testid="import-reason"
      >
        <AlertDescription>{{ job.reason }}</AlertDescription>
      </Alert>
      <Alert v-if="actError" variant="error" data-testid="import-action-error">
        <AlertDescription>{{ actError }}</AlertDescription>
      </Alert>
      <Progress v-if="job.state === 'running' || job.state === 'paused'" :model-value="percent" />
      <div
        class="flex flex-wrap gap-x-3 text-kira-sm text-muted-foreground"
        data-testid="import-totals"
      >
        <span>{{
          t('memory.import.totals.files.label', {
            done: job.progress.filesDone,
            count: job.progress.filesTotal,
          })
        }}</span>
        <span>{{ t('memory.import.totals.added.label', { count: job.totals.added }) }}</span>
        <span>{{ t('memory.import.totals.updated.label', { count: job.totals.updated }) }}</span>
        <span>{{ t('memory.import.totals.known.label', { count: job.totals.noop }) }}</span>
        <span v-if="job.totals.dropped > 0">{{
          t('memory.import.totals.dropped.label', { count: job.totals.dropped })
        }}</span>
        <span v-if="job.totals.unresolved > 0">{{
          t('common.unresolved.label', { count: job.totals.unresolved })
        }}</span>
        <span v-if="job.totals.failedFiles > 0">{{
          t('common.failedCount.label', { count: job.totals.failedFiles })
        }}</span>
        <span v-if="job.totals.skippedFiles > 0">{{
          t('memory.import.totals.skipped.label', { count: job.totals.skippedFiles })
        }}</span>
      </div>
    </div>
    <div ref="scroll" class="min-h-0 flex-1 overflow-y-auto" data-testid="import-files">
      <div class="relative w-full" :style="{ height: `${totalSize}px` }">
        <div
          v-for="row in virtualItems"
          :key="files[row.index]?.id ?? row.index"
          class="absolute left-0 top-0 w-full"
          :style="{ height: `${row.size}px`, transform: `translateY(${row.start}px)` }"
        >
          <ImportFileRow
            v-if="files[row.index]"
            :file="files[row.index]!"
            :selected="selectedFileId === files[row.index]!.id"
            @select="selectedFileId = files[row.index]!.id"
            @retry="retryFile.mutate(files[row.index]!.id)"
          />
        </div>
      </div>
    </div>
    <div
      v-if="selectedFile"
      class="flex max-h-1/3 shrink-0 flex-col gap-1 overflow-y-auto p-3 text-kira-md"
      data-testid="import-file-detail"
    >
      <span class="font-medium">{{ selectedFile.title || selectedFile.relPath }}</span>
      <p
        v-if="selectedFile.reason"
        class="m-0 text-muted-foreground"
        data-testid="import-file-reason"
        >{{
          selectedFile.reason
        }}</p
      >
      <div
        v-if="selectedFile.groupId"
        class="flex items-center gap-1.5 text-kira-sm text-muted-foreground"
        data-testid="import-file-group"
      >
        <span>{{ t('memory.import.file.group.label') }}</span>
        <Identifier :value="selectedFile.groupId" copyable />
      </div>
      <FieldDescription v-if="selectedFile.orphaned > 0" data-testid="import-file-orphaned">{{
        t('memory.import.file.orphaned.label', { count: selectedFile.orphaned })
      }}</FieldDescription>
      <div
        v-if="selectedFile.docPath"
        class="flex items-center gap-1.5 text-kira-sm text-muted-foreground"
        data-testid="import-file-doc"
      >
        <span>{{ t('memory.import.file.docPath.label') }}</span>
        <Identifier :value="selectedFile.docPath" split="path" copyable />
      </div>
      <Alert
        v-if="selectedFile.docError"
        variant="warn"
        :icon="null"
        data-testid="import-file-doc-error"
      >
        <span>{{ t('memory.import.file.docError.label', { error: selectedFile.docError }) }}</span>
      </Alert>
      <Alert v-for="(u, i) in selectedFile.unresolved" :key="`u${i}`" variant="warn" :icon="null">
        <span>{{ u.fact }}</span>
        <span v-for="q in u.questions" :key="q" class="text-kira-sm text-muted-foreground">{{
          q
        }}</span>
      </Alert>
      <Card v-for="(d, i) in selectedFile.dropped" :key="`d${i}`" class="gap-0">
        <span>{{ d.fact }}</span>
        <span class="text-kira-sm text-muted-foreground">{{
          t('memory.import.file.dropped.label', { why: d.why })
        }}</span>
      </Card>
    </div>
  </div>
</template>
