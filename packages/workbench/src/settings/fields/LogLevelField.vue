<script setup lang="ts" generic="K extends string">
import type { LogLevel } from '@shared/domain/settings';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { computed, useId } from 'vue';

// P120: generalized from the former GitLogLevelField.vue — the select/reset markup was
// byte-identical between Kira Studio's own `advanced.logLevel` and Kira Space's own
// `advanced.gitLogLevel`, only the leaf key, label, select testid and helper text (still the
// default slot) differ per app.
const props = defineProps<{
  advanced: Record<K, LogLevel>;
  leaf: K;
  label: string;
  selectTestid: string;
  isAtDefault: (section: 'advanced', key: K) => boolean;
  resetLeaf: (section: 'advanced', key: K) => void;
}>();

const resetTestid = computed(() => `settings-reset-advanced-${props.leaf}`);

function onLogLevelChange(value: unknown): void {
  props.advanced[props.leaf] = String(value) as LogLevel;
}

// F3: see DateFormatField.vue's own comment -- `for` + `id` ties the label to the select
// explicitly, so the Reset button (a sibling, outside the label) is no longer what a title/helper
// click activates.
const fieldId = useId();
</script>

<template>
  <Field>
    <div class="flex items-center justify-between gap-1">
      <FieldLabel :for="fieldId">{{ label }}</FieldLabel>
      <TooltipIconButton
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        :data-testid="resetTestid"
        :disabled-trigger="isAtDefault('advanced', leaf)"
        :disabled="isAtDefault('advanced', leaf)"
        @click="resetLeaf('advanced', leaf)"
      />
    </div>
    <NativeSelect
      :id="fieldId"
      variant="bordered"
      :data-testid="selectTestid"
      :model-value="advanced[leaf]"
      @update:model-value="onLogLevelChange"
    >
      <option value="off">{{ t('common.off.status') }}</option>
      <option value="error">{{ t('workbench.opLog.error.status') }}</option>
      <option value="warn">{{ t('workbench.settings.logLevelWarn.option') }}</option>
      <option value="info">{{ t('workbench.settings.logLevelInfo.option') }}</option>
      <option value="debug">{{ t('workbench.settings.logLevelDebug.option') }}</option>
    </NativeSelect>
    <slot />
  </Field>
</template>
