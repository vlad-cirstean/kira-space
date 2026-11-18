<script setup lang="ts">
import type { AppearanceSettings } from '@shared/domain/settings';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { computed, useId } from 'vue';
import { FONT_CHOICES, type FontChoice, fontStackAvailable, resolveFontFallback } from '../fonts';

// The data font picker, shared by both Appearance panes. The pane is created on open and destroyed
// on close, so the canvas availability probe runs once per open and needs no invalidation.
const props = defineProps<{
  appearance: AppearanceSettings;
  isAtDefault: (section: 'appearance', key: 'fontFamily') => boolean;
  resetLeaf: (section: 'appearance', key: 'fontFamily') => void;
}>();

const unavailable = computed(() => !fontStackAvailable(props.appearance.fontFamily));
const fallback = computed(() => resolveFontFallback(props.appearance.fontFamily));

function fontLabel(f: FontChoice): string {
  return f.system ? t('theme.fontFamily.system.label') : f.family;
}

const stacks = computed(() => ({
  on: FONT_CHOICES.filter((f) => fontStackAvailable(f.stack)),
  off: FONT_CHOICES.filter((f) => !fontStackAvailable(f.stack)),
}));

// A stored value outside FONT_CHOICES stays selectable as its own "Current" option.
const currentIsListed = computed(() =>
  FONT_CHOICES.some((f) => f.stack === props.appearance.fontFamily),
);

function onChange(value: unknown): void {
  props.appearance.fontFamily = String(value);
}

const fieldId = useId();
</script>

<template>
  <Field>
    <div class="flex items-center justify-between gap-1">
      <FieldLabel :for="fieldId">{{ t('theme.fontFamily.label') }}</FieldLabel>
      <TooltipIconButton
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        data-testid="settings-reset-appearance-fontFamily"
        :disabled-trigger="isAtDefault('appearance', 'fontFamily')"
        :disabled="isAtDefault('appearance', 'fontFamily')"
        @click="resetLeaf('appearance', 'fontFamily')"
      />
    </div>
    <NativeSelect
      :id="fieldId"
      variant="bordered"
      class="self-start"
      data-testid="settings-font-family"
      :model-value="appearance.fontFamily"
      @update:model-value="onChange"
    >
      <optgroup v-if="!currentIsListed" :label="t('theme.fontFamily.current.label')">
        <option :value="appearance.fontFamily">{{ appearance.fontFamily }}</option>
      </optgroup>
      <optgroup :label="t('theme.fontFamily.installed.label')">
        <option
          v-for="f in stacks.on"
          :key="f.stack"
          :value="f.stack"
          :style="{ fontFamily: f.stack }"
        >
          {{ fontLabel(f) }}
        </option>
      </optgroup>
      <optgroup :label="t('theme.fontFamily.notInstalled.label')">
        <option
          v-for="f in stacks.off"
          :key="f.stack"
          :value="f.stack"
          :style="{ fontFamily: f.stack }"
        >
          {{ fontLabel(f) }}
        </option>
      </optgroup>
    </NativeSelect>
    <span
      class="truncate text-kira-sm text-fg"
      data-testid="font-preview"
      :style="{ fontFamily: appearance.fontFamily }"
      >{{
        t('theme.fontFamily.preview.label')
      }}</span
    >
    <FieldError v-if="unavailable" data-testid="font-unavailable">
      <template v-if="fallback">{{
        t('theme.fontFamily.unavailable.fallback.error', { fallback })
      }}</template>
      <template v-else>{{ t('theme.fontFamily.unavailable.error') }}</template>
    </FieldError>
    <FieldDescription v-else>{{ t('theme.fontFamily.description') }}</FieldDescription>
  </Field>
</template>
