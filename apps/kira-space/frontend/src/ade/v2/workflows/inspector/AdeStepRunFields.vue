<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Input } from '@theme/components/ui/input';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { useAgentList } from '@workbench/agent/agentInfo';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import { computed } from 'vue';
import { useProjectOptions } from '../../../../repo/state/projectOptions';
import { useBoard } from '../../queries';
import { useAdeWorkflowsUiStore } from '../../state/adeWorkflowsUi';
import type { WorkflowStep } from '../../wire';
import AdeInspectorRow from './AdeInspectorRow.vue';

const props = defineProps<{ step: WorkflowStep }>();
const emit = defineEmits<{ patch: [p: Partial<WorkflowStep>] }>();
const gates = [
  {
    value: 'auto',
    label: t('space.ade.workflows.gate.auto.option'),
    testid: 'ade-wf-step-gate-auto',
  },
  {
    value: 'approval',
    label: t('space.ade.workflows.gate.approval.option'),
    testid: 'ade-wf-step-gate-approval',
  },
];
const scopes = [
  {
    value: 'once',
    label: t('space.ade.workflows.runsOn.once.label'),
    testid: 'ade-wf-step-scope-once',
  },
  {
    value: 'each repo',
    label: t('space.ade.workflows.runsOn.each.label'),
    testid: 'ade-wf-step-scope-each',
  },
  {
    value: 'one repo',
    label: t('space.ade.workflows.runsOn.oneRepo.option'),
    testid: 'ade-wf-step-scope-one',
  },
];

// `only <name>` stores the repo by its nickname or name.
const projects = useProjectOptions();
const scope = computed(() => {
  const v = props.step.runsOn;
  return v === 'once' || v === 'each repo' ? v : 'one repo';
});
const stored = computed(() =>
  props.step.runsOn.startsWith('only ') ? props.step.runsOn.slice(5) : '',
);
const matched = computed(() => projects.options.value.find((o) => o.label === stored.value));
const missing = computed(
  () =>
    scope.value === 'one repo' &&
    stored.value !== '' &&
    !matched.value &&
    !projects.isPending.value,
);

// Review steps need a branch, so only agent and script steps may leave the repos.
const canLeaveRepos = computed(() => props.step.kind !== 'review');
const showRepos = computed(() => !props.step.outsideRepo);
const onRepoAware = (on: boolean): void =>
  emit(
    'patch',
    on ? { outsideRepo: false, runsOn: 'each repo' } : { outsideRepo: true, runsOn: '' },
  );

const board = useBoard();
const { list: agents } = useAgentList();
const wfUi = useAdeWorkflowsUiStore();
const statuses = computed(() => board.data.value?.statuses ?? []);
// A step with no status uses the default of its kind; a deleted status falls back to it too.
const defaultName = computed(() => {
  const d = board.data.value?.statusDefaults;
  const id = props.step.kind === 'review' ? d?.review : d?.step;
  return statuses.value.find((s) => s.id === id)?.name ?? '';
});
const defaultLabel = computed(() =>
  defaultName.value
    ? t('space.ade.workflows.stepInspector.statusDefault.label', { status: defaultName.value })
    : t('space.ade.workflows.stepInspector.statusDefaultKeep.label'),
);
const statusMissing = computed(
  () =>
    props.step.status !== '' &&
    statuses.value.length > 0 &&
    !statuses.value.some((s) => s.id === props.step.status),
);
const missingLabel = computed(() =>
  t('space.ade.workflows.stepInspector.statusMissing.label', {
    id: props.step.status,
    status: defaultName.value || t('space.ade.workflows.stepInspector.statusDefaultKeep.label'),
  }),
);
const id = (k: string): string => `ade-wf-step-${k}-${props.step.id}`;

function setScope(v: string): void {
  if (v === 'one repo') {
    const first = projects.options.value[0];
    if (first) emit('patch', { runsOn: `only ${first.label}` });
    return;
  }
  emit('patch', { runsOn: v as WorkflowStep['runsOn'] });
}
</script>

<template>
  <AdeInspectorRow
    v-if="canLeaveRepos"
    :label="t('space.ade.workflows.stepInspector.repoAware.label')"
    :for="id('repo-aware')"
  >
    <Checkbox
      :id="id('repo-aware')"
      :model-value="!step.outsideRepo"
      data-testid="ade-wf-step-repo-aware"
      @update:model-value="(v) => onRepoAware(v === true)"
    />
    <template #hint>{{ t('space.ade.workflows.stepInspector.repoAware.hint') }}</template>
  </AdeInspectorRow>
  <AdeInspectorRow v-if="showRepos" :label="t('space.ade.workflows.field.runsOn.label')">
    <SecondaryTabs
      :model-value="scope"
      :aria-label="t('space.ade.workflows.field.runsOn.label')"
      :items="scopes"
      data-testid="ade-wf-step-scope"
      @update:model-value="setScope"
    />
    <ProjectPicker
      v-if="scope === 'one repo'"
      :model-value="matched?.value ?? (stored || null)"
      testid="ade-wf-step-scope-repo"
      :projects="projects.options.value"
      :placeholder="t('common.action.choose.label')"
      :loading="projects.isPending.value"
      :error="projects.error.value"
      @select="(o) => emit('patch', { runsOn: `only ${o.label}` })"
    />
    <template v-if="missing" #hint>
      <span data-testid="ade-wf-step-scope-missing">{{
        t('space.ade.workflows.runsOn.missing.hint', { name: stored })
      }}</span>
    </template>
  </AdeInspectorRow>
  <AdeInspectorRow :label="t('common.action.start.label')">
    <SecondaryTabs
      :model-value="step.before || 'auto'"
      :items="gates"
      data-testid="ade-wf-step-gate"
      @update:model-value="(v) => emit('patch', { before: v === 'approval' ? 'approval' : 'auto' })"
    />
  </AdeInspectorRow>
  <AdeInspectorRow :label="t('common.field.timeout.label')" :for="id('timeout')">
    <Input
      :id="id('timeout')"
      :model-value="step.timeout"
      class="w-24"
      data-testid="ade-wf-step-timeout"
      @update:model-value="(v: string | number) => emit('patch', { timeout: String(v) })"
    />
  </AdeInspectorRow>
  <AdeInspectorRow :label="t('space.ade.workflows.stepInspector.status.label')" :for="id('status')">
    <div class="flex items-center gap-2">
      <NativeSelect
        :id="id('status')"
        variant="bordered"
        class="min-w-0 flex-1"
        :model-value="step.status"
        data-testid="ade-wf-step-status"
        @update:model-value="(v) => emit('patch', { status: String(v) as WorkflowStep['status'] })"
      >
        <option value="">{{ defaultLabel }}</option>
        <option v-if="statusMissing" :value="step.status">{{ missingLabel }}</option>
        <option v-for="s in statuses" :key="s.id" :value="s.id">{{ s.name }}</option>
      </NativeSelect>
      <Button
        variant="link"
        size="kira"
        class="shrink-0"
        data-testid="ade-step-edit-statuses"
        @click="wfUi.openStatuses()"
        >{{
          t('space.ade.workflows.stepInspector.editStatuses.label')
        }}</Button
      >
    </div>
  </AdeInspectorRow>
  <AdeInspectorRow
    v-if="step.kind === 'agent' || step.kind === 'review'"
    :label="t('memory.common.origin.agent.label')"
    :for="id('agent')"
  >
    <NativeSelect
      :id="id('agent')"
      variant="bordered"
      class="w-full"
      :model-value="step.agent"
      data-testid="ade-wf-step-agent"
      @update:model-value="(v) => emit('patch', { agent: String(v) })"
    >
      <option value="">{{ t('space.ade.workflows.stepInspector.agentDefault.label') }}</option>
      <option v-for="e in agents" :key="e.id" :value="e.id">{{ e.productLabel }}</option>
    </NativeSelect>
  </AdeInspectorRow>
</template>
