<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useCopy } from '@workbench/behaviour/clipboard';
import ConfigFileBar from '@workbench/config/ConfigFileBar.vue';
import { defineAsyncComponent } from 'vue';
import { control } from '../../../bridge/control';
import { useWorkflowYaml } from '../queries';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import type { WorkflowEntry } from '../wire';
import AdeWorkflowYaml from './AdeWorkflowYaml.vue';

// One workflow: mode toggle, the config file bar (path, status, Open, Reveal), Copy YAML, then the Graph or YAML pane. The graph editor (Vue Flow)
// loads on first use so the board bundle does not carry it.
const AdeWorkflowGraph = defineAsyncComponent(() => import('./AdeWorkflowGraph.vue'));
const modeItems = [
  { value: 'graph', label: t('common.graph.label'), testid: 'ade-wf-mode-graph' },
  { value: 'yaml', label: t('space.ade.workflows.editor.yaml.label'), testid: 'ade-wf-mode-yaml' },
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
        size="kira-lg"
        :model-value="wfUi.workflowMode"
        :items="modeItems"
        :aria-label="t('space.ade.workflows.editor.mode.aria')"
        data-testid="ade-wf-mode"
        @update:model-value="setMode"
      />
      <ConfigFileBar
        :path="entry.path"
        :error="entry.error"
        :open="() => control.configFilesOpen({ folder: 'workflows', file: entry.fileName })"
        :reveal="() => control.configFilesReveal({ folder: 'workflows', file: entry.fileName })"
      />
      <Button
        variant="dialog"
        size="kira-lg"
        :disabled="!yaml.data.value"
        data-testid="ade-wf-copy"
        @click="yaml.data.value && copy(yaml.data.value.yaml)"
      >
        {{
          copied ? t('common.copy.copied.announce') : t('space.ade.workflows.editor.copyYaml.label')
        }}
      </Button>
    </div>
    <AdeWorkflowYaml v-if="wfUi.workflowMode === 'yaml'" :entry="entry" />
    <AdeWorkflowGraph v-else :entry="entry" />
  </div>
</template>
