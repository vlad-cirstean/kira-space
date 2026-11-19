<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useCopy } from '@workbench/behaviour/clipboard';
import ConfigFileProblem from '@workbench/config/ConfigFileProblem.vue';
import { defineAsyncComponent } from 'vue';
import { useWorkflowYaml } from '../queries';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import type { WorkflowEntry } from '../wire';
import AdeYamlPane from './AdeYamlPane.vue';

// One workflow: mode toggle, the config file bar in YAML mode (the file's problem in Graph mode), Copy YAML, then the Graph or YAML pane. The graph editor (Vue Flow)
// loads on first use so the board bundle does not carry it.
const AdeWorkflowGraph = defineAsyncComponent(() => import('./AdeWorkflowGraph.vue'));
const modeItems = [
  { value: 'graph', label: t('common.graph.label'), testid: 'ade-wf-mode-graph' },
  { value: 'yaml', label: t('common.yaml.label'), testid: 'ade-wf-mode-yaml' },
];
const props = defineProps<{ entry: WorkflowEntry }>();
const wfUi = useAdeWorkflowsUiStore();
const yaml = useWorkflowYaml(() => props.entry.fileName);
const { copy, copied } = useCopy();

async function setMode(v: unknown): Promise<void> {
  if ((v !== 'graph' && v !== 'yaml') || v === wfUi.workflowMode) return;
  if (await wfUi.leave()) wfUi.workflowMode = v;
}
</script>

<template>
  <div
    class="flex flex-col gap-3"
    :class="wfUi.workflowMode === 'yaml' ? 'max-w-4xl' : 'h-full min-h-0'"
    data-testid="ade-wf-editor"
  >
    <div class="flex flex-wrap items-center gap-3">
      <SecondaryTabs
        variant="segmented"
        :model-value="wfUi.workflowMode"
        :items="modeItems"
        :aria-label="t('space.ade.workflows.editor.mode.aria')"
        data-testid="ade-wf-mode"
        @update:model-value="setMode"
      />
      <Button
        variant="toolbar"
        size="kira"
        class="ml-auto"
        :disabled="!yaml.data.value"
        data-testid="ade-wf-copy"
        @click="yaml.data.value && copy(yaml.data.value.yaml)"
        ><CodiconIcon name="copy" :size="13" />
        {{
          copied ? t('common.copy.copied.announce') : t('space.ade.workflows.editor.copyYaml.label')
        }}
      </Button>
    </div>
    <ConfigFileProblem
      :error="entry.error"
      :show-yaml="wfUi.workflowMode !== 'yaml' ? () => void setMode('yaml') : undefined"
      class="shrink-0"
    />
    <AdeYamlPane
      v-if="wfUi.workflowMode === 'yaml'"
      kind="workflow"
      :file-name="entry.fileName"
      :entry-error="entry.error"
      testid-prefix="ade-wf-yaml"
    />
    <AdeWorkflowGraph v-else :entry="entry" />
  </div>
</template>
