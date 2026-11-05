<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import { paneFrame } from '@theme/lib/frames';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { errorText } from '@workbench/bridge/codedError';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { computed, ref, watch } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeTip from '../AdeTip.vue';
import { KIND_TONE, kindLabel, STEP_KINDS } from '../board/workflowForm';
import { useNewStep } from '../queries';
import { useWorkflows } from '../readQueries';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import type { StepKind } from '../wire';
import AdeLibraryStepEditor from './AdeLibraryStepEditor.vue';

// Steps tab: the step library, the reusable steps workflows reference, beside the editor for the picked one.
const wfUi = useAdeWorkflowsUiStore();
const workflows = useWorkflows();
const newStep = useNewStep();
const listError = ref('');

const entries = computed(() => workflows.data.value?.steps ?? []);
const dir = computed(() => workflows.data.value?.stepsDir ?? '');
const current = computed(
  () => entries.value.find((e) => e.fileName === wfUi.stepFile) ?? entries.value[0] ?? null,
);

// A file that is broken on disk has no form to show: open it in YAML mode.
watch(
  () => current.value?.fileName,
  () => {
    if (current.value?.error) wfUi.stepMode = 'yaml';
  },
  { immediate: true },
);

function rowClass(selected: boolean): string {
  return cn(
    rowVariants({ layout: 'double', selected }),
    'box-border w-full flex-col items-stretch gap-0.5 whitespace-normal rounded-kira border-0 border-l-3 border-solid px-2 py-2 text-left',
    selected ? 'border-l-tone-amber-solid' : 'border-l-transparent',
  );
}

async function pick(fileName: string): Promise<void> {
  if (fileName !== current.value?.fileName && !(await wfUi.leave())) return;
  wfUi.stepFile = fileName;
}

async function createNew(kind: StepKind): Promise<void> {
  if (!(await wfUi.leave())) return;
  listError.value = '';
  try {
    const entry = await newStep.mutateAsync({
      name: t('space.ade.workflows.step.defaultName.label'),
      kind,
      from: null,
    });
    wfUi.openStep(entry.fileName);
  } catch (err) {
    listError.value = errorText(err);
  }
}
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 gap-0.5" data-testid="ade-steps-tab">
    <div :class="cn(paneFrame, 'flex w-75 shrink-0 flex-col')" data-testid="ade-step-list">
      <PanelHeader>
        {{ t('space.ade.workflows.tabs.steps.label') }}
        <template #actions>
          <DropdownMenu>
            <DropdownMenuTrigger as-child>
              <Button variant="dialog" size="kira-lg" class="px-1.5" data-testid="ade-step-new">
                {{ t('space.ade.workflows.step.defaultName.label') }}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                v-for="k in STEP_KINDS"
                :key="k"
                :data-testid="`ade-step-new-${k}`"
                @select="createNew(k)"
              >
                {{ kindLabel(k) }}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </template>
      </PanelHeader>
      <div class="flex min-h-0 flex-1 flex-col gap-1 overflow-auto p-2">
        <Alert v-if="listError" variant="error" data-testid="ade-step-error"
          ><AlertDescription>{{ listError }}</AlertDescription></Alert
        >
        <InlineState
          v-if="workflows.isSuccess.value && entries.length === 0"
          kind="empty"
          data-testid="ade-step-empty"
        >
          {{ t('space.ade.steps.empty.emptyHint', { dir }) }}
        </InlineState>
        <button
          v-for="e in entries"
          :key="e.fileName"
          type="button"
          :class="rowClass(current?.fileName === e.fileName)"
          data-testid="ade-step-row"
          :data-file="e.fileName"
          @click="pick(e.fileName)"
        >
          <span class="flex w-full items-center gap-1.5">
            <span
              class="min-w-0 flex-1 truncate text-kira-lg font-medium"
              data-testid="ade-step-row-name"
              >{{
                e.step?.name ?? e.fileName
              }}</span
            >
            <AdeChip v-if="e.step" :label="kindLabel(e.step.kind)" :tone="KIND_TONE[e.step.kind]" />
            <AdeTip v-if="e.error" :text="e.error.message">
              <span
                class="shrink-0 text-kira-sm font-medium text-tone-red"
                data-testid="ade-step-row-error"
                >✕</span
              >
            </AdeTip>
          </span>
          <span class="text-kira-sm text-subtle" data-testid="ade-step-row-used">{{
            t('space.ade.steps.usedBy.label', { count: e.usedBy.length })
          }}</span>
        </button>
        <div class="px-1 pt-3 text-kira-sm leading-normal text-subtle">
          {{ t('space.ade.steps.intro.body') }}
        </div>
      </div>
    </div>
    <div :class="cn(paneFrame, 'flex flex-1 flex-col')">
      <PanelHeader>{{
        current?.step?.name ?? current?.fileName ?? t('space.ade.workflows.tabs.steps.label')
      }}</PanelHeader>
      <div class="min-h-0 flex-1 overflow-auto p-4">
        <AdeLibraryStepEditor v-if="current" :key="current.fileName" :entry="current" />
      </div>
    </div>
  </div>
</template>
