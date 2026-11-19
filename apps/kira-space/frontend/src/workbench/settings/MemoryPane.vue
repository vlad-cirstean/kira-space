<script setup lang="ts">
import ResetToDefaultButton from '@theme/components/ResetToDefaultButton.vue';
import { Field, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { useAgentInfo } from '@workbench/agent/agentInfo';
import AgentMcpSection from '@workbench/memory/settings/AgentMcpSection.vue';
import CheckHookSection from '@workbench/memory/settings/CheckHookSection.vue';
import SemanticModelSection from '@workbench/memory/settings/SemanticModelSection.vue';
import { computed, useId } from 'vue';
import type { SettingsPaneProps } from './types';

// P221: memory setup actions bypass draft/Save like ConnectedEditorsPane: a registration or a
// model download is an action, not a settings leaf. P314: the model per provider is a leaf and saves with
// the dialog.
const props = defineProps<SettingsPaneProps>();
const { info } = useAgentInfo();

const model = computed(() => props.draft.memory.models[info.value.id] ?? info.value.defaultModel);

function onModelChange(value: unknown): void {
  props.draft.memory.models = { ...props.draft.memory.models, [info.value.id]: String(value) };
}

const modelId = useId();
</script>

<template>
  <div class="contents" v-show="active">
    <Field>
      <div class="flex items-center justify-between gap-1">
        <FieldLabel :for="modelId">{{ t('space.settings.memory.agentModel.label') }}</FieldLabel>
        <ResetToDefaultButton
          data-testid="settings-reset-memory-agentModel"
          :at-default="isAtDefault('memory', 'models')"
          @click="resetLeaf('memory', 'models')"
        />
      </div>
      <NativeSelect
        :id="modelId"
        variant="bordered"
        data-testid="settings-memory-agent-model"
        :model-value="model"
        @update:model-value="onModelChange"
      >
        <option v-for="m in info.models" :key="m.id" :value="m.id">{{ m.label }}</option>
      </NativeSelect>
      <FieldDescription>{{ t('space.settings.memory.agentModel.description') }}</FieldDescription>
    </Field>
    <AgentMcpSection />
    <CheckHookSection />
    <SemanticModelSection />
  </div>
</template>
