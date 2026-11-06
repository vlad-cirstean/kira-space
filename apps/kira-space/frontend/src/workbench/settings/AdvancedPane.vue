<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldDescription } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import LogLevelField from '@workbench/settings/fields/LogLevelField.vue';
import NotifyPromptsField from '@workbench/settings/fields/NotifyPromptsField.vue';
import { computed, useId } from 'vue';
import { control } from '../../bridge/control';
import StudioAdvancedFields from '../../studio/workbench/settings/StudioAdvancedFields.vue';
import type { SettingsPaneProps } from './types';

// P103 Part 2 (§5.5): extracted verbatim from workbench/SettingsDialog.vue's own
// `v-else-if="activeSection === 'Advanced'"` branch — trimmed to the one leaf this app still owns,
// advanced.gitLogLevel (kira-space's own diagnostic log verbosity).
const props = defineProps<SettingsPaneProps>();

// `user` = background agents ignore the repo's own .claude settings.
const ignoreRepoSettings = computed(() => props.draft.ade.headlessSettingSources === 'user');
const switchId = useId();

function setSources(ignore: boolean): void {
  props.draft.ade.headlessSettingSources = ignore ? 'user' : 'all';
}
</script>

<template>
  <div class="contents" v-show="active">
    <LogLevelField
      :advanced="draft.advanced"
      leaf="gitLogLevel"
      :label="t('space.settings.advanced.gitLogLevel.label')"
      select-testid="settings-git-log-level"
      :is-at-default="isAtDefault"
      :reset-leaf="resetLeaf"
    >
      <FieldDescription>{{
        t('space.settings.advanced.gitLogLevel.description')
      }}</FieldDescription>
    </LogLevelField>
    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="switchId">{{ t('space.settings.advanced.ignoreRepoSettings.label') }}</Label>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          data-testid="settings-reset-ade-headlessSettingSources"
          :disabled-trigger="isAtDefault('ade', 'headlessSettingSources')"
          :disabled="isAtDefault('ade', 'headlessSettingSources')"
          @click="resetLeaf('ade', 'headlessSettingSources')"
        />
      </div>
      <Switch
        :id="switchId"
        :model-value="ignoreRepoSettings"
        data-testid="settings-ade-headless-sources"
        @update:model-value="(v) => setSources(v === true)"
      />
      <FieldDescription>{{
        t('space.settings.advanced.ignoreRepoSettings.description')
      }}</FieldDescription>
    </Field>
    <StudioAdvancedFields
      :active="active"
      :draft="draft"
      :is-at-default="isAtDefault"
      :reset-leaf="resetLeaf"
      :register-field-error="registerFieldError"
    />
    <NotifyPromptsField
      :advanced="draft.advanced"
      :is-at-default="isAtDefault"
      :reset-leaf="resetLeaf"
      :send-test="control.promptsSendTest"
    />
  </div>
</template>
