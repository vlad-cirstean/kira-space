<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Card } from '@theme/components/ui/card';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Textarea } from '@theme/components/ui/textarea';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import ParamsForm from '@workbench/automations/run/ParamsForm.vue';
import { computed } from 'vue';
import { useCustomScriptsStore } from '../../../../state/customScripts';
import { promptParts, smartBodyParts } from '../../automation/smartStepBody';
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
      ? { smartScript: smartScripts.value[0]?.name ?? '', prompt: '', allowedTools: [], params: {} }
      : { smartScript: '', params: {} },
  );
}
const bodyParts = computed(() =>
  script.value ? smartBodyParts(script.value.command, props.step.params) : [],
);
const preview = computed(() => promptParts(props.step.prompt));
const id = (k: string): string => `ade-wf-step-${k}-${props.step.id}`;
</script>

<template>
  <SecondaryTabs
    :model-value="smart ? 'smart' : 'prompt'"
    :items="modeItems"
    data-testid="ade-wf-step-mode"
    @update:model-value="setMode"
  />
  <div v-if="smart" class="flex flex-col gap-2" data-testid="ade-wf-step-smart">
    <Label :for="id('script')" class="text-kira-sm text-muted-foreground">{{
      t('common.smartScript.label')
    }}</Label>
    <NativeSelect
      :id="id('script')"
      variant="bordered"
      :model-value="step.smartScript"
      data-testid="ade-wf-step-script"
      @update:model-value="(v) => emit('patch', { smartScript: String(v), params: {} })"
    >
      <option v-if="!script && step.smartScript" :value="step.smartScript">{{
        step.smartScript
      }}</option>
      <option v-for="r in smartScripts" :key="r.id" :value="r.name">✦ {{ r.name }}</option>
    </NativeSelect>
    <span
      v-if="step.smartScript && !script"
      class="text-kira-sm text-error"
      data-testid="ade-wf-step-script-missing"
    >
      {{ t('space.ade.workflows.stepInspector.scriptMissing.error') }}
    </span>
    <ParamsForm
      v-if="plainParams.length > 0"
      :model-value="step.params"
      :params="plainParams"
      @update:model-value="(v) => emit('patch', { params: v })"
    />
    <span
      v-if="secretNames"
      class="text-kira-sm text-muted-foreground"
      data-testid="ade-wf-step-secret-params"
    >
      {{ t('space.ade.workflows.stepInspector.secretParams.hint', { names: secretNames }) }}
    </span>
    <Card
      v-if="script"
      class="font-data text-kira-sm whitespace-pre-wrap"
      data-testid="ade-wf-step-body"
    >
      <VarText :parts="bodyParts" />
    </Card>
  </div>
  <template v-else>
    <Label :for="id('prompt')" class="sr-only">{{ t('common.prompt.label') }}</Label>
    <Textarea
      :id="id('prompt')"
      :model-value="step.prompt"
      :placeholder="t('common.prompt.label')"
      class="min-h-24 w-auto resize-y leading-normal"
      data-testid="ade-wf-step-prompt"
      @update:model-value="(v: string | number) => emit('patch', { prompt: String(v) })"
    />
    <Card
      v-if="step.prompt"
      class="text-kira-sm text-muted-foreground"
      data-testid="ade-wf-step-preview"
    >
      <VarText :parts="preview" />
    </Card>
  </template>
</template>
