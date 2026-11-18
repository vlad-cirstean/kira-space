<script setup lang="ts" generic="K extends string">
import type { LogLevel } from '@shared/domain/settings';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';

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

const LEVEL_ITEMS = computed(() =>
  (
    [
      ['off', t('common.off.status')],
      ['error', t('workbench.opLog.error.status')],
      ['warn', t('workbench.settings.logLevelWarn.option')],
      ['info', t('workbench.settings.logLevelInfo.option')],
      ['debug', t('workbench.settings.logLevelDebug.option')],
    ] as const
  ).map(([value, label]) => ({ value, label, testid: `${props.selectTestid}-${value}` })),
);
</script>

<template>
  <Field>
    <div class="flex items-center justify-between gap-1">
      <FieldLabel>{{ label }}</FieldLabel>
      <TooltipIconButton
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        :data-testid="resetTestid"
        :disabled-trigger="isAtDefault('advanced', leaf)"
        :disabled="isAtDefault('advanced', leaf)"
        @click="resetLeaf('advanced', leaf)"
      />
    </div>
    <SecondaryTabs
      variant="choice"
      size="kira-lg"
      :aria-label="label"
      :model-value="advanced[leaf]"
      :items="LEVEL_ITEMS"
      :data-testid="selectTestid"
      @update:model-value="onLogLevelChange"
    />
    <slot />
  </Field>
</template>
