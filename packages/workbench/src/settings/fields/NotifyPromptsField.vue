<script setup lang="ts">
import ResetToDefaultButton from '@theme/components/ResetToDefaultButton.vue';
import { Button } from '@theme/components/ui/button';
import { Field, FieldContent, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { useId } from 'vue';
import { fireAndForget } from '../../bridge/fireAndForget';

// P246: `advanced.notifyPrompts`, identical in both apps: the switch, its reset, and a button that
// posts a test notification through the saved setting.
const props = defineProps<{
  advanced: { notifyPrompts: boolean };
  isAtDefault: (section: 'advanced', key: 'notifyPrompts') => boolean;
  resetLeaf: (section: 'advanced', key: 'notifyPrompts') => void;
  sendTest: () => Promise<void>;
}>();

const switchId = useId();

function onSendTest(): void {
  // A failed test notification has no UI; the permission state shows the cause.
  fireAndForget(props.sendTest(), 'notifyPrompts');
}
</script>

<template>
  <Field orientation="horizontal">
    <Switch
      :id="switchId"
      :model-value="advanced.notifyPrompts"
      data-testid="settings-notify-prompts"
      @update:model-value="(v) => (advanced.notifyPrompts = v === true)"
    />
    <FieldContent>
      <FieldLabel :for="switchId">{{ t('workbench.settings.notifyPrompts.label') }}</FieldLabel>
      <FieldDescription>
        {{ t('workbench.settings.notifyPrompts.description') }}
      </FieldDescription>
      <!-- conventions-allow N17: sends a test notification, not an approval -->
      <Button
        variant="dialog"
        size="kira-lg"
        class="self-start"
        data-testid="settings-notify-prompts-test"
        @click="onSendTest"
      >
        {{ t('common.sendTest.label') }}
      </Button>
    </FieldContent>
    <ResetToDefaultButton
      class="ml-auto"
      data-testid="settings-reset-advanced-notifyPrompts"
      :at-default="isAtDefault('advanced', 'notifyPrompts')"
      @click="resetLeaf('advanced', 'notifyPrompts')"
    />
  </Field>
</template>
