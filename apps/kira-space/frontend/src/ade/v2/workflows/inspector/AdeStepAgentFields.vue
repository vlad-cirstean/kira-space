<script setup lang="ts">
import CodeBlock from '@theme/components/CodeBlock.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Card } from '@theme/components/ui/card';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Textarea } from '@theme/components/ui/textarea';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import ParamsForm from '@workbench/automations/run/ParamsForm.vue';
import { computed } from 'vue';
import { useCustomScriptsStore } from '../../../../state/customScripts';
import { promptParts, smartBodyParts } from '../../automation/smartStepBody';
import { usesRepoTokens } from '../../board/workflowForm';
import type { WorkflowStep } from '../../wire';

const props = defineProps<{ step: WorkflowStep }>();
const emit = defineEmits<{ patch: [p: Partial<WorkflowStep>] }>();

const scripts = useCustomScriptsStore();
const smartScripts = computed(() => scripts.records.filter((r) => r.kind === 'smart'));
const smart = computed(() => props.step.smartScript !== '');
const script = computed(
  () => smartScripts.value.find((r) => r.name === props.step.smartScript) ?? null,
);
const plainParams = computed(() => script.value?.params.filter((p) => !p.secret) ?? []);
const secretNames = computed(() =>
  (script.value?.params.filter((p) => p.secret) ?? []).map((p) => p.name).join(', '),
);
const modeItems = computed(() => [
  { value: 'prompt', label: t('common.prompt.label'), testid: 'ade-wf-step-mode-prompt' },
  { value: 'smart', label: t('common.smartScript.label'), testid: 'ade-wf-step-mode-smart' },
]);
function setMode(v: string): void {
  if (v === (smart.value ? 'smart' : 'prompt')) return;
  emit(
    'patch',
    v === 'smart'
      ? {
          smartScript: smartScripts.value[0]?.name ?? '',
          prompt: '',
          allowedTools: [],
          dbMcp: false,
          params: {},
        }
      : { smartScript: '', params: {}, dbMcp: true },
  );
}
const bodyParts = computed(() =>
  script.value ? smartBodyParts(script.value.command, props.step.params) : [],
);
const preview = computed(() => promptParts(props.step.prompt));
const outsideVars = computed(() => props.step.outsideRepo && usesRepoTokens(props.step.prompt));
const id = (k: string): string => `ade-wf-step-${k}-${props.step.id}`;
</script>

<template>
  <Field orientation="responsive">
    <FieldLabel>{{ t('common.mode.label') }}</FieldLabel>
    <SecondaryTabs
      variant="choice"
      size="kira-lg"
      :aria-label="t('common.mode.label')"
      :model-value="smart ? 'smart' : 'prompt'"
      :items="modeItems"
      data-testid="ade-wf-step-mode"
      @update:model-value="setMode"
    />
  </Field>
  <div v-if="smart" class="flex flex-col gap-3" data-testid="ade-wf-step-smart">
    <Field orientation="responsive">
      <FieldLabel :for="id('script')">{{ t('common.smartScript.label') }}</FieldLabel>
      <NativeSelect
        :id="id('script')"
        variant="bordered"
        class="w-full"
        :model-value="step.smartScript"
        data-testid="ade-wf-step-script"
        @update:model-value="(v) => emit('patch', { smartScript: String(v), params: {} })"
      >
        <option v-if="!script && step.smartScript" :value="step.smartScript">{{
          step.smartScript
        }}</option>
        <option v-for="r in smartScripts" :key="r.id" :value="r.name">✦ {{ r.name }}</option>
      </NativeSelect>
      <FieldError v-if="step.smartScript && !script">
        <span data-testid="ade-wf-step-script-missing">{{
          t('space.ade.workflows.stepInspector.scriptMissing.error')
        }}</span>
      </FieldError>
    </Field>
    <ParamsForm
      v-if="plainParams.length > 0"
      :model-value="step.params"
      :params="plainParams"
      @update:model-value="(v) => emit('patch', { params: v })"
    />
    <FieldDescription v-if="secretNames" data-testid="ade-wf-step-secret-params">
      {{ t('space.ade.workflows.stepInspector.secretParams.hint', { names: secretNames }) }}
    </FieldDescription>
    <CodeBlock v-if="script" data-testid="ade-wf-step-body">
      <VarText :parts="bodyParts" />
    </CodeBlock>
  </div>
  <template v-else>
    <Field>
      <FieldLabel :for="id('prompt')">{{ t('common.prompt.label') }}</FieldLabel>
      <Textarea
        class="min-h-24 max-h-90 resize-y"
        :id="id('prompt')"
        :model-value="step.prompt"
        :placeholder="t('common.prompt.label')"
        data-testid="ade-wf-step-prompt"
        @update:model-value="(v: string | number) => emit('patch', { prompt: String(v) })"
      />
      <FieldError v-if="outsideVars">
        <span data-testid="ade-wf-step-outside-vars-error">{{
          t('space.ade.workflows.stepInspector.outsideVars.error', {
            repo: '{repo}',
            branch: '{branch}',
          })
        }}</span>
      </FieldError>
    </Field>
    <Card v-if="step.prompt" data-testid="ade-wf-step-preview">
      <FieldDescription><VarText :parts="preview" /></FieldDescription>
    </Card>
  </template>
</template>
