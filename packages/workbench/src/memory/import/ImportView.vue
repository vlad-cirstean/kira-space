<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { storeToRefs } from 'pinia';
import { computed } from 'vue';
import PanelBar from '../../components/PanelBar.vue';
import { useMemoryUiStore } from '../store';
import ImportJobDetail from './ImportJobDetail.vue';
import { jobTitle } from './importFormat';
import { useImportJobs } from './importQueries';
import { useImportUiStore } from './importStore';

// P211: the main area's imports view — job list on top, the selected job below.
const ui = useImportUiStore();
const { selectedJobId, confirmJobId } = storeToRefs(ui);
const memoryUi = useMemoryUiStore();
const jobs = useImportJobs();

const listed = computed(() => (jobs.data.value ?? []).filter((j) => j.id !== confirmJobId.value));
const current = computed(
  () => listed.value.find((j) => j.id === selectedJobId.value) ?? listed.value[0],
);
const jobItems = computed(() =>
  listed.value.map((job) => ({
    value: job.id,
    label:
      job.channel === 'mcp'
        ? `${jobTitle(job)} · ${t('memory.common.origin.agent.label')}`
        : jobTitle(job),
    testid: `import-job-${job.id}`,
  })),
);
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="import-view">
    <PanelBar class="flex-row items-center gap-1.5">
      <Button
        variant="toolbar"
        size="kira"
        data-testid="import-back"
        @click="memoryUi.view = 'memory'"
        >{{
          t('memory.import.back.label')
        }}</Button
      >
      <SecondaryTabs
        class="min-w-0 overflow-x-auto"
        :model-value="current?.id ?? ''"
        :items="jobItems"
        @update:model-value="(v) => (selectedJobId = v)"
      />
    </PanelBar>
    <EmptyState
      v-if="!current"
      fill
      icon="cloud-download"
      data-testid="import-empty"
      :title="t('memory.import.list.empty')"
      :description="t('memory.import.list.emptyHint')"
    />
    <ImportJobDetail v-else :key="current.id" :job-id="current.id" class="min-h-0 flex-1" />
  </div>
</template>
