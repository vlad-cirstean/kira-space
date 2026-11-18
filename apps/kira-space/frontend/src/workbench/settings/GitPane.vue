<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import NumberStepperInput from '@theme/NumberStepperInput.vue';
import { computed, ref, useId, watch } from 'vue';
import {
  defaultSettings,
  FETCH_AUTO_INTERVAL_MINUTES_RANGE,
  FONT_SIZE_RANGE,
} from '../../state/settingsDomain';
import type { SettingsPaneProps } from './types';

// P103 Part 2 (§5.5): extracted verbatim from workbench/SettingsDialog.vue's own
// `v-else-if="activeSection === 'Git'"` branch — server-owned remote-op settings, moved to this
// app wholesale from Kira Studio (P100 Part 2) along with the repo workspace they configure.
const props = defineProps<SettingsPaneProps>();

function onFetchAutoIntervalInput(e: Event): void {
  props.draft.git.fetchAutoIntervalMinutes = Number((e.target as HTMLInputElement).value);
}

// P92 item 9: an emptied field writes 0 (the "follow appearance.fontSize" sentinel), same rule
// applyAppearance() reads — not NaN, which a plain Number(...) would let through unnoticed for a
// genuinely blank input.
function onGraphFontSizeInput(e: Event): void {
  const raw = (e.target as HTMLInputElement).value;
  props.draft.git.graphFontSize = raw === '' ? 0 : Number(raw);
}

// `*` matches any run of characters except `/` — the same rule server-side git preflight
// enforces; this dialog only edits the pattern list, never evaluates it. A plain ref, not a
// computed bound straight to draft.git.protectedBranches: parsing on every keystroke and feeding
// the result back into the textarea's own value would snap away a blank line the instant it's
// created, fighting the user mid-edit — parsed into the draft by the watcher below instead,
// one-directionally.
const protectedBranchesText = ref(props.draft.git.protectedBranches.join('\n'));
watch(protectedBranchesText, (v) => {
  props.draft.git.protectedBranches = v
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
});
function resetProtectedBranches(): void {
  props.draft.git.protectedBranches = [...defaultSettings.git.protectedBranches];
  protectedBranchesText.value = props.draft.git.protectedBranches.join('\n');
}

const fetchAutoIntervalError = computed<string | null>(() => {
  const v = props.draft.git.fetchAutoIntervalMinutes;
  if (!Number.isFinite(v)) return t('common.enterNumber.error');
  if (v < FETCH_AUTO_INTERVAL_MINUTES_RANGE.min || v > FETCH_AUTO_INTERVAL_MINUTES_RANGE.max) {
    return t('space.settings.git.fetchRange.error', {
      min: FETCH_AUTO_INTERVAL_MINUTES_RANGE.min,
      max: FETCH_AUTO_INTERVAL_MINUTES_RANGE.max,
    });
  }
  return null;
});
props.registerFieldError('git.fetchAutoIntervalMinutes', fetchAutoIntervalError);

// P92 item 9: 0 is the valid "follow appearance.fontSize" sentinel, but 1..8 is below
// FONT_SIZE_RANGE.min and unreadable — reject that gap here, in the control, since the schema
// itself accepts the full 0..max range (a stored out-of-range value must still hydrate).
const graphFontSizeError = computed<string | null>(() => {
  const v = props.draft.git.graphFontSize;
  if (!Number.isFinite(v)) return t('common.enterNumber.error');
  if (v === 0) return null;
  if (v < FONT_SIZE_RANGE.min || v > FONT_SIZE_RANGE.max) {
    return t('space.settings.git.graphFont.range.error', {
      min: FONT_SIZE_RANGE.min,
      max: FONT_SIZE_RANGE.max,
    });
  }
  return null;
});
props.registerFieldError('git.graphFontSize', graphFontSizeError);

// P110 I2-26: `for`/`id` preserves the old label-wraps-control implicit association (see
// FontSizeField.vue's own precedent comment) now that the field wrapper is a plain <Field> div.
const protectedBranchesId = useId();
const fetchAutoIntervalMinutesId = useId();
const gitPathId = useId();
const graphFontSizeId = useId();
</script>

<template>
  <div class="contents" v-show="active">
    <FieldSet>
      <FieldLegend>{{ t('space.settings.git.remote.title') }}</FieldLegend>
      <FieldDescription>
        {{ t('space.settings.git.remote.description') }}
      </FieldDescription>
      <Field>
        <div class="flex items-center justify-between gap-1">
          <FieldLabel :for="protectedBranchesId">{{
            t('space.settings.git.protected.label')
          }}</FieldLabel>
          <TooltipIconButton
            icon="discard"
            :label="t('common.action.resetToDefault.label')"
            data-testid="settings-reset-git-protectedBranches"
            :disabled-trigger="isAtDefault('git', 'protectedBranches')"
            :disabled="isAtDefault('git', 'protectedBranches')"
            @click="resetProtectedBranches"
          />
        </div>
        <Textarea
          class="font-data"
          :id="protectedBranchesId"
          v-model="protectedBranchesText"
          :placeholder="t('space.settings.git.protected.example')"
          data-testid="settings-git-protected-branches"
        />
        <FieldDescription>{{ t('space.settings.git.protected.description') }}</FieldDescription>
      </Field>
      <Field>
        <div class="flex items-center justify-between gap-1">
          <FieldLabel :for="fetchAutoIntervalMinutesId">{{
            t('space.settings.git.fetchInterval.label')
          }}</FieldLabel>
          <TooltipIconButton
            icon="discard"
            :label="t('common.action.resetToDefault.label')"
            data-testid="settings-reset-git-fetchAutoIntervalMinutes"
            :disabled-trigger="isAtDefault('git', 'fetchAutoIntervalMinutes')"
            :disabled="isAtDefault('git', 'fetchAutoIntervalMinutes')"
            @click="resetLeaf('git', 'fetchAutoIntervalMinutes')"
          />
        </div>
        <NumberStepperInput
          :id="fetchAutoIntervalMinutesId"
          :min="FETCH_AUTO_INTERVAL_MINUTES_RANGE.min"
          :max="FETCH_AUTO_INTERVAL_MINUTES_RANGE.max"
          :aria-invalid="!!fetchAutoIntervalError || undefined"
          data-testid="settings-git-fetch-auto-interval"
          :model-value="String(draft.git.fetchAutoIntervalMinutes)"
          @input="onFetchAutoIntervalInput"
        />
        <FieldError
          v-if="fetchAutoIntervalError"
          data-testid="settings-git-fetch-auto-interval-error"
        >
          {{ fetchAutoIntervalError }}
        </FieldError>
        <FieldDescription v-else>{{
          t('space.settings.git.fetchInterval.description')
        }}</FieldDescription>
      </Field>
      <Field>
        <div class="flex items-center justify-between gap-1">
          <FieldLabel :for="gitPathId">{{ t('space.settings.git.gitPath.label') }}</FieldLabel>
          <TooltipIconButton
            icon="discard"
            :label="t('common.action.resetToDefault.label')"
            data-testid="settings-reset-git-gitPath"
            :disabled-trigger="isAtDefault('git', 'gitPath')"
            :disabled="isAtDefault('git', 'gitPath')"
            @click="resetLeaf('git', 'gitPath')"
          />
        </div>
        <Input
          :id="gitPathId"
          type="text"
          class="w-full font-data"
          data-testid="settings-git-path"
          v-model="draft.git.gitPath"
        />
        <FieldDescription>{{ t('space.settings.git.gitPath.description') }}</FieldDescription>
      </Field>
    </FieldSet>
    <FieldSet>
      <FieldLegend>{{ t('common.graph.label') }}</FieldLegend>
      <Field>
        <div class="flex items-center justify-between gap-1">
          <FieldLabel :for="graphFontSizeId">{{
            t('space.settings.git.graphFont.label')
          }}</FieldLabel>
          <TooltipIconButton
            icon="discard"
            :label="t('common.action.resetToDefault.label')"
            data-testid="settings-reset-git-graphFontSize"
            :disabled-trigger="isAtDefault('git', 'graphFontSize')"
            :disabled="isAtDefault('git', 'graphFontSize')"
            @click="resetLeaf('git', 'graphFontSize')"
          />
        </div>
        <NumberStepperInput
          :id="graphFontSizeId"
          :min="FONT_SIZE_RANGE.min"
          :max="FONT_SIZE_RANGE.max"
          :aria-invalid="!!graphFontSizeError || undefined"
          data-testid="settings-git-graphFontSize"
          :model-value="String(draft.git.graphFontSize)"
          @input="onGraphFontSizeInput"
        />
        <FieldError v-if="graphFontSizeError" data-testid="settings-git-graphFontSize-error">
          {{ graphFontSizeError }}
        </FieldError>
        <FieldDescription v-else>{{
          t('space.settings.git.graphFont.description')
        }}</FieldDescription>
      </Field>
    </FieldSet>
  </div>
</template>
