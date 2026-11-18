<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { Progress } from '@theme/components/ui/progress';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useMemoryUiStore } from '../store';
import { ACTIVE_JOB_STATES, jobTitle } from './importFormat';
import { useImportJobs } from './importQueries';
import { useImportUiStore } from './importStore';

// P211: panel row for the newest unfinished import — progress and a way into the imports view.
const ui = useImportUiStore();
const memoryUi = useMemoryUiStore();
const jobs = useImportJobs();

const STATUS_LABEL = {
  paused: 'memory.import.status.paused.label',
  running: 'memory.import.status.running.label',
  ready: 'memory.import.status.ready.label',
} as const;

// A freshly scanned job is confirmed in its dialog, not listed here.
const job = computed(() =>
  (jobs.data.value ?? []).find(
    (j) => ACTIVE_JOB_STATES.includes(j.state) && j.id !== ui.confirmJobId,
  ),
);
const percent = computed(() => {
  const p = job.value?.progress;
  return p && p.chunksTotal > 0 ? (p.chunksDone / p.chunksTotal) * 100 : 0;
});

function open(): void {
  if (!job.value) return;
  ui.selectedJobId = job.value.id;
  memoryUi.view = 'imports';
}
</script>

<template>
  <div
    v-if="job"
    class="flex flex-col gap-1 text-kira-sm text-muted-foreground"
    data-testid="memory-import-status"
    :data-state="job.state"
  >
    <div class="flex items-center gap-1.5">
      <span class="min-w-0 flex-1 truncate">
        {{
          t(
            STATUS_LABEL[
              job.state === 'paused' ? 'paused' : job.state === 'running' ? 'running' : 'ready'
            ],
            {
              title: jobTitle(job),
              done: job.progress.filesDone,
              count: job.progress.filesTotal,
            },
          )
        }}
      </span>
      <Button variant="toolbar" size="kira" data-testid="memory-import-open" @click="open"
        ><CodiconIcon name="eye" :size="13" />{{ t('common.view.label') }}</Button
      >
    </div>
    <Progress v-if="job.state === 'running'" :model-value="percent" />
  </div>
</template>
