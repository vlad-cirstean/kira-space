<script setup lang="ts">
import ResetToDefaultButton from '@theme/components/ResetToDefaultButton.vue';
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@theme/components/ui/field';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import NumberStepperInput from '@theme/NumberStepperInput.vue';
import { formatCount } from '@workbench/behaviour/format';
import LogLevelField from '@workbench/settings/fields/LogLevelField.vue';
import NotifyPromptsField from '@workbench/settings/fields/NotifyPromptsField.vue';
import { computed, useId } from 'vue';
import { control } from '../../bridge/control';
import {
  EXPENSIVE_QUERY_ROWS_RANGE,
  OP_LOG_RETENTION_DAYS_RANGE,
} from '../../state/settingsDomain';
import { parseIntField, type SettingsPaneProps } from './types';

const props = defineProps<SettingsPaneProps>();

// `user` = background agents ignore the repo's own .claude settings.
const ignoreRepoSettings = computed(() => props.draft.ade.headlessSettingSources === 'user');
const switchId = useId();

function setSources(ignore: boolean): void {
  props.draft.ade.headlessSettingSources = ignore ? 'user' : 'all';
}
function onOpLogRetentionInput(e: Event): void {
  props.draft.advanced.opLogRetentionDays = parseIntField((e.target as HTMLInputElement).value);
}

function onExpensiveQueryRowsInput(e: Event): void {
  props.draft.advanced.expensiveQueryRows = parseIntField((e.target as HTMLInputElement).value);
}

const opLogRetentionError = computed<string | null>(() => {
  const v = props.draft.advanced.opLogRetentionDays;
  if (!Number.isInteger(v)) return t('common.enterWholeNumber.error');
  if (v < OP_LOG_RETENTION_DAYS_RANGE.min || v > OP_LOG_RETENTION_DAYS_RANGE.max) {
    return t('studio.settings.range.days.status', {
      min: OP_LOG_RETENTION_DAYS_RANGE.min,
      max: OP_LOG_RETENTION_DAYS_RANGE.max,
    });
  }
  return null;
});
props.registerFieldError('advanced.opLogRetentionDays', opLogRetentionError);

const expensiveQueryRowsError = computed<string | null>(() => {
  const v = props.draft.advanced.expensiveQueryRows;
  if (!Number.isInteger(v)) return t('common.enterWholeNumber.error');
  if (v < EXPENSIVE_QUERY_ROWS_RANGE.min || v > EXPENSIVE_QUERY_ROWS_RANGE.max) {
    return t('studio.settings.range.plain.status', {
      min: formatCount(EXPENSIVE_QUERY_ROWS_RANGE.min),
      max: formatCount(EXPENSIVE_QUERY_ROWS_RANGE.max),
    });
  }
  return null;
});
props.registerFieldError('advanced.expensiveQueryRows', expensiveQueryRowsError);

// P110 I2-26: `for`/`id` preserves the old label-wraps-control implicit association (see
// FontSizeField.vue's own precedent comment) now that the field wrapper is a plain <Field> div.
const opLogRetentionDaysId = useId();
const expensiveQueryRowsId = useId();
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
    <Field orientation="horizontal">
      <Switch
        :id="switchId"
        :model-value="ignoreRepoSettings"
        data-testid="settings-ade-headless-sources"
        @update:model-value="(v) => setSources(v === true)"
      />
      <FieldContent>
        <FieldLabel :for="switchId">{{
          t('space.settings.advanced.ignoreRepoSettings.label')
        }}</FieldLabel>
        <FieldDescription>{{
          t('space.settings.advanced.ignoreRepoSettings.description')
        }}</FieldDescription>
      </FieldContent>
      <ResetToDefaultButton
        class="ml-auto"
        data-testid="settings-reset-ade-headlessSettingSources"
        :at-default="isAtDefault('ade', 'headlessSettingSources')"
        @click="resetLeaf('ade', 'headlessSettingSources')"
      />
    </Field>
    <Field>
      <div class="flex items-center justify-between gap-1">
        <FieldLabel :for="opLogRetentionDaysId">{{
          t('studio.settings.advanced.opLogRetention.label')
        }}</FieldLabel>
        <ResetToDefaultButton
          data-testid="settings-reset-advanced-opLogRetentionDays"
          :at-default="isAtDefault('advanced', 'opLogRetentionDays')"
          @click="resetLeaf('advanced', 'opLogRetentionDays')"
        />
      </div>
      <NumberStepperInput
        :id="opLogRetentionDaysId"
        :min="OP_LOG_RETENTION_DAYS_RANGE.min"
        :max="OP_LOG_RETENTION_DAYS_RANGE.max"
        :aria-invalid="!!opLogRetentionError || undefined"
        data-testid="settings-oplog-retention"
        :model-value="String(draft.advanced.opLogRetentionDays)"
        @input="onOpLogRetentionInput"
      />
      <FieldError v-if="opLogRetentionError" data-testid="settings-oplog-retention-error">
        {{ opLogRetentionError }}
      </FieldError>
    </Field>

    <Field>
      <div class="flex items-center justify-between gap-1">
        <FieldLabel :for="expensiveQueryRowsId">{{
          t('studio.settings.advanced.expensiveRows.label')
        }}</FieldLabel>
        <ResetToDefaultButton
          data-testid="settings-reset-advanced-expensiveQueryRows"
          :at-default="isAtDefault('advanced', 'expensiveQueryRows')"
          @click="resetLeaf('advanced', 'expensiveQueryRows')"
        />
      </div>
      <NumberStepperInput
        :id="expensiveQueryRowsId"
        :min="EXPENSIVE_QUERY_ROWS_RANGE.min"
        :max="EXPENSIVE_QUERY_ROWS_RANGE.max"
        :aria-invalid="!!expensiveQueryRowsError || undefined"
        data-testid="settings-expensive-query-rows"
        :model-value="String(draft.advanced.expensiveQueryRows)"
        @input="onExpensiveQueryRowsInput"
      />
      <FieldError v-if="expensiveQueryRowsError" data-testid="settings-expensive-query-rows-error">
        {{ expensiveQueryRowsError }}
      </FieldError>
      <FieldDescription v-else data-testid="settings-expensive-query-rows-description">{{
        t('studio.settings.advanced.expensiveRows.description')
      }}</FieldDescription>
    </Field>
    <NotifyPromptsField
      :advanced="draft.advanced"
      :is-at-default="isAtDefault"
      :reset-leaf="resetLeaf"
      :send-test="control.promptsSendTest"
    />
  </div>
</template>
