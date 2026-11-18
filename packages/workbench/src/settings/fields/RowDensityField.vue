<script setup lang="ts">
import type { AppearanceSettings, RowDensity } from '@shared/domain/settings';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';

// I2-18: the row-density button pair (compact/comfortable) was byte-identical between kira-studio's
// and kira-space's own AppearancePane.vue; each app's own helper text and (kira-studio only) preview
// table stay app-side through the default slot.
const props = defineProps<{
  appearance: AppearanceSettings;
  isAtDefault: (section: 'appearance', key: 'rowDensity') => boolean;
  resetLeaf: (section: 'appearance', key: 'rowDensity') => void;
}>();

const DENSITY_ITEMS = computed(() => [
  {
    value: 'compact',
    label: t('workbench.settings.rowCompact.option'),
    testid: 'settings-appearance-rowDensity-compact',
  },
  {
    value: 'comfortable',
    label: t('workbench.settings.rowComfortable.option'),
    testid: 'settings-appearance-rowDensity-comfortable',
  },
]);

function setRowDensity(density: RowDensity): void {
  props.appearance.rowDensity = density;
}
</script>

<template>
  <Field>
    <div class="flex items-center justify-between gap-1">
      <FieldLabel>{{ t('workbench.settings.rowHeight.label') }}</FieldLabel>
      <TooltipIconButton
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        data-testid="settings-reset-appearance-rowDensity"
        :disabled-trigger="isAtDefault('appearance', 'rowDensity')"
        :disabled="isAtDefault('appearance', 'rowDensity')"
        @click="resetLeaf('appearance', 'rowDensity')"
      />
    </div>
    <SecondaryTabs
      variant="choice"
      size="kira-lg"
      :aria-label="t('workbench.settings.rowHeight.label')"
      :model-value="appearance.rowDensity"
      :items="DENSITY_ITEMS"
      @update:model-value="(v) => setRowDensity(v as RowDensity)"
    />
    <slot />
  </Field>
</template>
