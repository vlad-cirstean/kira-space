<script setup lang="ts">
import type { AppearanceSettings } from '@shared/domain/settings';
import ResetToDefaultButton from '@theme/components/ResetToDefaultButton.vue';
import { Field, FieldContent, FieldGroup, FieldLabel } from '@theme/components/ui/field';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { useId } from 'vue';

// I2-18: the word-wrap checkbox row (checkbox, reset button) was byte-identical between
// kira-studio's and kira-space's own AppearancePane.vue; the helper text differs per app (each
// names its own word-wrap surfaces), so it stays app-side through the default slot.
const props = defineProps<{
  appearance: AppearanceSettings;
  isAtDefault: (section: 'appearance', key: 'wordWrap') => boolean;
  resetLeaf: (section: 'appearance', key: 'wordWrap') => void;
}>();

function onWordWrapChange(checked: boolean): void {
  props.appearance.wordWrap = checked;
}

// P110 I2-26: `for`/`id` preserves the old label-wraps-control implicit association (see
// FontSizeField.vue's own precedent comment) now that the field wrapper is a plain <Field> div.
const wordWrapId = useId();
</script>

<template>
  <FieldGroup>
    <Field orientation="horizontal">
      <Switch
        :id="wordWrapId"
        :model-value="appearance.wordWrap"
        data-testid="settings-word-wrap"
        @update:model-value="(v) => onWordWrapChange(v === true)"
      />
      <FieldContent>
        <FieldLabel :for="wordWrapId">{{ t('workbench.settings.wordWrap.label') }}</FieldLabel>
        <slot />
      </FieldContent>
    </Field>
    <ResetToDefaultButton
      class="ml-auto"
      data-testid="settings-reset-appearance-wordWrap"
      :at-default="isAtDefault('appearance', 'wordWrap')"
      @click="resetLeaf('appearance', 'wordWrap')"
    />
  </FieldGroup>
</template>
