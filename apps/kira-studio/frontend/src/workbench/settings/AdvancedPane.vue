<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldDescription, FieldError } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
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

// P103 Part 2 (§5.5): extracted verbatim from workbench/SettingsDialog.vue's own
// `v-else-if="activeSection === 'Advanced'"` branch.
const props = defineProps<SettingsPaneProps>();

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
    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="opLogRetentionDaysId">{{
          t('studio.settings.advanced.opLogRetention.label')
        }}</Label>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          data-testid="settings-reset-advanced-opLogRetentionDays"
          :disabled-trigger="isAtDefault('advanced', 'opLogRetentionDays')"
          :disabled="isAtDefault('advanced', 'opLogRetentionDays')"
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
    <FieldDescription>{{ t('studio.settings.restartNote.hint') }}</FieldDescription>

    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="expensiveQueryRowsId">{{
          t('studio.settings.advanced.expensiveRows.label')
        }}</Label>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          data-testid="settings-reset-advanced-expensiveQueryRows"
          :disabled-trigger="isAtDefault('advanced', 'expensiveQueryRows')"
          :disabled="isAtDefault('advanced', 'expensiveQueryRows')"
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

    <LogLevelField
      :advanced="draft.advanced"
      leaf="logLevel"
      :label="t('studio.settings.advanced.logLevel.label')"
      select-testid="settings-log-level"
      :is-at-default="isAtDefault"
      :reset-leaf="resetLeaf"
    >
      <FieldDescription>{{ t('studio.settings.advanced.logLevel.description') }}</FieldDescription>
    </LogLevelField>
    <NotifyPromptsField
      :advanced="draft.advanced"
      :is-at-default="isAtDefault"
      :reset-leaf="resetLeaf"
      :send-test="control.promptsSendTest"
    />
  </div>
</template>
