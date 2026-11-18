<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { useId } from 'vue';
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

function onDateFormatChange(value: unknown): void {
  props.appearance.dateFormat = String(value) as AppearanceSettings['dateFormat'];
}

// A native label delegates its click to its first labelable descendant -- with the Reset
// button living inside the label (the old markup), that was the button, not the select, so
// clicking the title/helper text reset the field instead of focusing the control. `for` + `id`
// ties the label to the select explicitly instead, with the button as a sibling outside it.
const fieldId = useId();
</script>

<template>
  <Field>
    <div class="flex items-center justify-between gap-1">
      <FieldLabel :for="fieldId">{{ t('space.settings.appearance.commitDate.label') }}</FieldLabel>
      <TooltipIconButton
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        data-testid="settings-reset-appearance-dateFormat"
        :disabled-trigger="isAtDefault('appearance', 'dateFormat')"
        :disabled="isAtDefault('appearance', 'dateFormat')"
        @click="resetLeaf('appearance', 'dateFormat')"
      />
    </div>
    <NativeSelect
      :id="fieldId"
      variant="bordered"
      size="kira-lg"
      data-testid="settings-date-format"
      :model-value="appearance.dateFormat"
      @update:model-value="onDateFormatChange"
    >
      <option value="relative">{{
        t('space.settings.appearance.commitDate.relative.option')
      }}</option>
      <option value="absolute">{{
        t('space.settings.appearance.commitDate.absolute.option')
      }}</option>
    </NativeSelect>
    <FieldDescription>{{ t('space.settings.appearance.commitDate.description') }}</FieldDescription>
  </Field>
</template>
