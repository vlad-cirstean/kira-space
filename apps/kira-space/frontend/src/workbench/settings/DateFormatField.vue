<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import type { Settings } from '../../state/settingsDomain';

// The commit-date leaf (select, reset button, helper text) was byte-identical between
// kira-studio's and kira-space's own AppearancePane.vue. dateFormat is this app's own leaf
// now (only its git module reads it), so this field types on this app's own Settings['appearance']
// instead of the former shared AppearanceSettings.
type AppearanceSettings = Settings['appearance'];

const props = defineProps<{
  appearance: AppearanceSettings;
  isAtDefault: (section: 'appearance', key: 'dateFormat') => boolean;
  resetLeaf: (section: 'appearance', key: 'dateFormat') => void;
}>();

const DATE_FORMAT_ITEMS = computed(() => [
  {
    value: 'relative',
    label: t('space.settings.appearance.commitDate.relative.option'),
    testid: 'settings-date-format-relative',
  },
  {
    value: 'absolute',
    label: t('space.settings.appearance.commitDate.absolute.option'),
    testid: 'settings-date-format-absolute',
  },
]);

function onDateFormatChange(value: unknown): void {
  props.appearance.dateFormat = String(value) as AppearanceSettings['dateFormat'];
}
</script>

<template>
  <Field>
    <div class="flex items-center justify-between gap-1">
      <FieldLabel>{{ t('space.settings.appearance.commitDate.label') }}</FieldLabel>
      <TooltipIconButton
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        data-testid="settings-reset-appearance-dateFormat"
        :disabled-trigger="isAtDefault('appearance', 'dateFormat')"
        :disabled="isAtDefault('appearance', 'dateFormat')"
        @click="resetLeaf('appearance', 'dateFormat')"
      />
    </div>
    <SecondaryTabs
      variant="choice"
      size="kira-lg"
      :aria-label="t('space.settings.appearance.commitDate.label')"
      :model-value="appearance.dateFormat"
      :items="DATE_FORMAT_ITEMS"
      data-testid="settings-date-format"
      @update:model-value="onDateFormatChange"
    />
    <FieldDescription>{{ t('space.settings.appearance.commitDate.description') }}</FieldDescription>
  </Field>
</template>
