<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldContent, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { computed, useId } from 'vue';
import { chordFromEvent } from '../../capture/chord';
import { useCaptureStatus } from '../../capture/queries';
import type { SettingsPaneProps } from './types';

// Clipboard capture: on/off plus the global chord. The chord is recorded from the next keydown, saved
// with the dialog, and re-registered live by the backend; the status line shows what the OS accepted.
const props = defineProps<SettingsPaneProps>();

const switchId = useId();
const hotkeyId = useId();
const status = useCaptureStatus();
const mac = /Mac/.test(navigator.userAgent);

const hotkeyError = computed<string | null>(() =>
  props.draft.capture.enabled && props.draft.capture.hotkey === ''
    ? t('space.settings.capture.hotkey.required.error')
    : null,
);
props.registerFieldError('capture.hotkey', hotkeyError);

function record(e: KeyboardEvent): void {
  if (e.key === 'Tab') return;
  e.preventDefault();
  if (e.key === 'Escape') {
    (e.target as HTMLElement).blur();
    return;
  }
  if (e.key === 'Backspace' || e.key === 'Delete') {
    props.draft.capture.hotkey = '';
    return;
  }
  const chord = chordFromEvent(e, mac);
  if (chord) props.draft.capture.hotkey = chord;
}

const statusText = computed(() => {
  const s = status.data.value;
  if (!s) return '';
  return t(`space.settings.capture.status.${s.state}.body`, {
    accelerator: s.accelerator,
    reason: s.reason ?? '',
  });
});
</script>

<template>
  <div class="contents" v-show="active">
    <Field orientation="horizontal">
      <Switch
        :id="switchId"
        :model-value="draft.capture.enabled"
        data-testid="settings-capture-enabled"
        @update:model-value="(v) => (draft.capture.enabled = v === true)"
      />
      <FieldContent>
        <FieldLabel :for="switchId">{{ t('space.settings.capture.enabled.label') }}</FieldLabel>
        <FieldDescription>{{ t('space.settings.capture.enabled.description') }}</FieldDescription>
      </FieldContent>
      <TooltipIconButton
        class="ml-auto"
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        data-testid="settings-reset-capture-enabled"
        :disabled-trigger="isAtDefault('capture', 'enabled')"
        :disabled="isAtDefault('capture', 'enabled')"
        @click="resetLeaf('capture', 'enabled')"
      />
    </Field>
    <Field>
      <div class="flex items-center justify-between gap-1">
        <FieldLabel :for="hotkeyId">{{ t('space.settings.capture.hotkey.label') }}</FieldLabel>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          data-testid="settings-reset-capture-hotkey"
          :disabled-trigger="isAtDefault('capture', 'hotkey')"
          :disabled="isAtDefault('capture', 'hotkey')"
          @click="resetLeaf('capture', 'hotkey')"
        />
      </div>
      <Input
        :id="hotkeyId"
        readonly
        :disabled="!draft.capture.enabled"
        :model-value="draft.capture.hotkey"
        :placeholder="t('space.settings.capture.hotkey.placeholder')"
        :aria-invalid="hotkeyError !== null"
        data-testid="settings-capture-hotkey"
        @keydown="record"
      />
      <FieldDescription>{{ t('space.settings.capture.hotkey.description') }}</FieldDescription>
      <FieldDescription v-if="statusText" data-testid="settings-capture-status">{{
        statusText
      }}</FieldDescription>
    </Field>
  </div>
</template>
