<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Field, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import { computed } from 'vue';
import { useProjectOptions } from '../../../../repo/state/projectOptions';
import type { WorkflowStep } from '../../wire';
import AdeStepAgentPick from './AdeStepAgentPick.vue';
import AdeStepStatus from './AdeStepStatus.vue';

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
  <Field v-if="canLeaveRepos" orientation="responsive">
    <FieldLabel :for="id('repo-aware')">{{
      t('space.ade.workflows.stepInspector.repoAware.label')
    }}</FieldLabel>
    <Checkbox
      :id="id('repo-aware')"
      :model-value="!step.outsideRepo"
      data-testid="ade-wf-step-repo-aware"
      @update:model-value="(v) => onRepoAware(v === true)"
    />
    <FieldDescription>{{ t('space.ade.workflows.stepInspector.repoAware.hint') }}</FieldDescription>
  </Field>
  <Field v-if="showRepos" orientation="responsive">
    <FieldLabel>{{ t('space.ade.workflows.field.runsOn.label') }}</FieldLabel>
    <SecondaryTabs
      variant="choice"
      size="kira-lg"
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
    <FieldDescription v-if="missing">
      <span data-testid="ade-wf-step-scope-missing">{{
        t('space.ade.workflows.runsOn.missing.hint', { name: stored })
      }}</span>
    </FieldDescription>
  </Field>
  <Field orientation="responsive">
    <FieldLabel>{{ t('common.action.start.label') }}</FieldLabel>
    <SecondaryTabs
      variant="choice"
      size="kira-lg"
      :aria-label="t('common.action.start.label')"
      :model-value="step.before || 'auto'"
      :items="gates"
      data-testid="ade-wf-step-gate"
      @update:model-value="(v) => emit('patch', { before: v === 'approval' ? 'approval' : 'auto' })"
    />
  </Field>
  <Field orientation="responsive">
    <FieldLabel :for="id('timeout')">{{ t('common.field.timeout.label') }}</FieldLabel>
    <Input
      :id="id('timeout')"
      :model-value="step.timeout"
      class="w-24"
      data-testid="ade-wf-step-timeout"
      @update:model-value="(v: string | number) => emit('patch', { timeout: String(v) })"
    />
  </Field>
  <AdeStepStatus :step="step" @patch="(p) => emit('patch', p)" />
  <AdeStepAgentPick
    v-if="step.kind === 'agent' || step.kind === 'review'"
    :step="step"
    @patch="(p) => emit('patch', p)"
  />
</template>
