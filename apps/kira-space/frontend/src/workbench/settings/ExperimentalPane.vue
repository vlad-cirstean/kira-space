<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldDescription } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { Switch } from '@theme/components/ui/switch';
import { type MessageKey, t } from '@theme/i18n';
import type { Settings } from '../../state/settingsDomain';
import type { SettingsPaneProps } from './types';

// One row per flag. Literal keys only: the i18n check rejects built keys.
defineProps<SettingsPaneProps>();

const FLAGS = [
  {
    key: 'memory',
    label: 'space.settings.experimental.memory.label',
    description: 'space.settings.experimental.memory.description',
  },
] as const satisfies readonly {
  key: keyof Settings['experimental'];
  label: MessageKey;
  description: MessageKey;
}[];
</script>

<template>
  <div class="contents" v-show="active">
    <FieldDescription>{{ t('space.settings.experimental.intro.description') }}</FieldDescription>
    <Field v-for="flag in FLAGS" :key="flag.key">
      <div class="flex items-center justify-between gap-1">
        <Label :for="`settings-experimental-${flag.key}`">{{ t(flag.label) }}</Label>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          :data-testid="`settings-reset-experimental-${flag.key}`"
          :disabled-trigger="isAtDefault('experimental', flag.key)"
          :disabled="isAtDefault('experimental', flag.key)"
          @click="resetLeaf('experimental', flag.key)"
        />
      </div>
      <Switch
        :id="`settings-experimental-${flag.key}`"
        :model-value="draft.experimental[flag.key]"
        :data-testid="`settings-experimental-${flag.key}`"
        @update:model-value="(v) => (draft.experimental[flag.key] = v === true)"
      />
      <FieldDescription>{{ t(flag.description) }}</FieldDescription>
    </Field>
  </div>
</template>
