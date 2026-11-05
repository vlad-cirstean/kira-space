<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { paneFrame } from '@theme/lib/frames';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { useAgentInfo } from '@workbench/agent/agentInfo';
import { errorText } from '@workbench/bridge/codedError';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { computed, ref, watch } from 'vue';
import AdeTip from '../AdeTip.vue';
import { useImportWorkflow, useNewWorkflow, useWorkflows } from '../queries';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import type { WorkflowEntry } from '../wire';
import AdeWorkflowEditor from './AdeWorkflowEditor.vue';

// Workflows tab: the list of workflow files beside the editor for the picked one.
const { info } = useAgentInfo();
const wfUi = useAdeWorkflowsUiStore();
const workflows = useWorkflows();
const importWf = useImportWorkflow();
const newWf = useNewWorkflow();

function rowClass(selected: boolean): string {
  return cn(
    rowVariants({ layout: 'double', selected }),
    'box-border w-full flex-col items-stretch gap-0.5 whitespace-normal rounded-kira border-0 border-l-3 border-solid px-2 py-2 text-left',
    selected ? 'border-l-tone-amber-solid' : 'border-l-transparent',
  );
}

const entries = computed(() => workflows.data.value?.workflows ?? []);
const dir = computed(() => workflows.data.value?.dir ?? '');
const current = computed(
  () => entries.value.find((e) => e.fileName === wfUi.workflowFile) ?? entries.value[0] ?? null,
);

// A file that is broken on disk has no form to show: open it in YAML mode.
watch(
  () => current.value?.fileName,
  () => {
    if (current.value?.error) wfUi.workflowMode = 'yaml';
  },
  { immediate: true },
);

const importOpen = ref(false);
const importPath = ref('');
const listError = ref('');

function stepList(e: WorkflowEntry): string {
  return e.workflow ? e.workflow.steps.map((s) => s.name).join(' › ') : '';
}

const selectedFile = computed(() => current.value?.fileName ?? null);

async function pick(fileName: string): Promise<void> {
  if (fileName !== selectedFile.value && !(await wfUi.leave())) return;
  wfUi.workflowFile = fileName;
}

async function runImport(): Promise<void> {
  const path = importPath.value.trim();
  if (!path || !(await wfUi.leave())) return;
  listError.value = '';
  try {
    const entry = await importWf.mutateAsync({ path });
    wfUi.workflowFile = entry.fileName;
    wfUi.workflowMode = 'yaml';
    importPath.value = '';
    importOpen.value = false;
  } catch (err) {
    listError.value = errorText(err);
  }
}

async function createNew(): Promise<void> {
  if (!(await wfUi.leave())) return;
  listError.value = '';
  try {
    const entry = await newWf.mutateAsync({ name: t('space.ade.workflows.page.new.label') });
    wfUi.workflowFile = entry.fileName;
    wfUi.workflowMode = 'graph';
  } catch (err) {
    listError.value = errorText(err);
  }
}
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 gap-0.5" data-testid="ade-workflows-tab">
    <div :class="cn(paneFrame, 'flex w-75 shrink-0 flex-col')" data-testid="ade-wf-list">
      <PanelHeader>
        {{ t('space.common.workflows.label') }}
        <template #actions>
          <Button
            variant="dialog"
            size="kira-lg"
            class="px-1.5"
            data-testid="ade-wf-import"
            @click="importOpen = !importOpen"
          >
            {{ t('space.ade.workflows.page.import.label') }}
          </Button>
          <Button
            variant="dialog"
            size="kira-lg"
            class="px-1.5"
            data-testid="ade-wf-new"
            @click="createNew"
          >
            {{ t('space.ade.workflows.page.new.label') }}
          </Button>
        </template>
      </PanelHeader>
      <div class="flex min-h-0 flex-1 flex-col gap-1 overflow-auto p-2">
        <div v-if="importOpen" class="flex flex-col gap-1 pb-2" data-testid="ade-wf-import-row">
          <div class="flex gap-1.5">
            <Label for="ade-wf-import-path" class="sr-only">{{
              t('space.ade.workflows.page.importPath.aria')
            }}</Label>
            <Input
              id="ade-wf-import-path"
              v-model="importPath"
              :placeholder="t('space.ade.workflows.page.importPath.example')"
              size="kira-lg"
              class="min-w-0 flex-1 font-data"
              data-testid="ade-wf-import-path"
              @keydown.enter="runImport"
            />
            <Button
              variant="dialog"
              size="kira-lg"
              :pending="importWf.isPending.value"
              data-testid="ade-wf-import-go"
              @click="runImport"
            >
              {{ t('common.import.label') }}
            </Button>
          </div>
        </div>
        <Alert v-if="listError" variant="error" data-testid="ade-wf-error"
          ><AlertDescription>{{ listError }}</AlertDescription></Alert
        >
        <InlineState
          v-if="workflows.isSuccess.value && entries.length === 0"
          kind="empty"
          data-testid="ade-wf-empty"
        >
          {{ t('space.ade.workflows.page.empty.emptyHint', { dir }) }}
        </InlineState>
        <button
          v-for="e in entries"
          :key="e.fileName"
          type="button"
          :class="rowClass(selectedFile === e.fileName)"
          data-testid="ade-wf-row"
          :data-file="e.fileName"
          @click="pick(e.fileName)"
        >
          <span class="flex w-full items-center gap-1.5">
            <span
              class="min-w-0 flex-1 truncate text-kira-lg font-medium"
              data-testid="ade-wf-row-name"
              >{{
                e.workflow?.name ?? e.fileName
              }}</span
            >
            <AdeTip v-if="e.error" :text="e.error.message">
              <span
                class="shrink-0 text-kira-sm font-medium text-tone-red"
                data-testid="ade-wf-row-error"
                >✕</span
              >
            </AdeTip>
          </span>
          <span
            class="max-w-full truncate text-kira-sm text-muted-foreground"
            data-testid="ade-wf-row-steps"
            >{{
              stepList(e)
            }}</span
          >
          <span class="text-kira-sm text-subtle" data-testid="ade-wf-row-used">{{
            t('space.ade.workflows.page.usedBy.label', { count: e.usedBy })
          }}</span>
        </button>
        <div class="px-1 pt-3 text-kira-sm leading-normal text-subtle">
          <CatalogText k="space.ade.workflows.page.intro.body">
            <template #headless><span class="font-data">{{ info.headlessLabel }}</span></template>
          </CatalogText>
        </div>
      </div>
    </div>
    <div :class="cn(paneFrame, 'flex flex-1 flex-col')">
      <PanelHeader>{{
        current?.workflow?.name ?? current?.fileName ?? t('space.common.workflow.label')
      }}</PanelHeader>
      <div class="min-h-0 flex-1 overflow-auto p-4">
        <AdeWorkflowEditor v-if="current" :key="current.fileName" :entry="current" />
      </div>
    </div>
  </div>
</template>
