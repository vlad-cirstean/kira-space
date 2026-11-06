<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldDescription, FieldError } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import NumberStepperInput from '@theme/NumberStepperInput.vue';
import { formatCount } from '@workbench/behaviour/format';
import { computed, useId } from 'vue';
import { parseIntField, type SettingsPaneProps } from '../../../workbench/settings/types';
import {
  EXPENSIVE_QUERY_ROWS_RANGE,
  OP_LOG_RETENTION_DAYS_RANGE,
} from '../../state/settingsDomain';

// The Studio modules' two Advanced leaves, rendered inside Space's AdvancedPane.
const props = defineProps<SettingsPaneProps>();

function onOpLogRetentionInput(e: Event): void {
  props.draft.studioAdvanced.opLogRetentionDays = parseIntField(
    (e.target as HTMLInputElement).value,
  );
}

function onExpensiveQueryRowsInput(e: Event): void {
  props.draft.studioAdvanced.expensiveQueryRows = parseIntField(
    (e.target as HTMLInputElement).value,
  );
}

const opLogRetentionError = computed<string | null>(() => {
  const v = props.draft.studioAdvanced.opLogRetentionDays;
  if (!Number.isInteger(v)) return t('common.enterWholeNumber.error');
  if (v < OP_LOG_RETENTION_DAYS_RANGE.min || v > OP_LOG_RETENTION_DAYS_RANGE.max) {
    return t('studio.settings.range.days.status', {
      min: OP_LOG_RETENTION_DAYS_RANGE.min,
      max: OP_LOG_RETENTION_DAYS_RANGE.max,
    });
  }
  return null;
});
props.registerFieldError('studioAdvanced.opLogRetentionDays', opLogRetentionError);

const expensiveQueryRowsError = computed<string | null>(() => {
  const v = props.draft.studioAdvanced.expensiveQueryRows;
  if (!Number.isInteger(v)) return t('common.enterWholeNumber.error');
  if (v < EXPENSIVE_QUERY_ROWS_RANGE.min || v > EXPENSIVE_QUERY_ROWS_RANGE.max) {
    return t('studio.settings.range.plain.status', {
      min: formatCount(EXPENSIVE_QUERY_ROWS_RANGE.min),
      max: formatCount(EXPENSIVE_QUERY_ROWS_RANGE.max),
    });
  }
  return null;
});
props.registerFieldError('studioAdvanced.expensiveQueryRows', expensiveQueryRowsError);

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
          :disabled-trigger="isAtDefault('studioAdvanced', 'opLogRetentionDays')"
          :disabled="isAtDefault('studioAdvanced', 'opLogRetentionDays')"
          @click="resetLeaf('studioAdvanced', 'opLogRetentionDays')"
        />
      </div>
      <NumberStepperInput
        :id="opLogRetentionDaysId"
        :min="OP_LOG_RETENTION_DAYS_RANGE.min"
        :max="OP_LOG_RETENTION_DAYS_RANGE.max"
        :aria-invalid="!!opLogRetentionError || undefined"
        data-testid="settings-oplog-retention"
        :model-value="String(draft.studioAdvanced.opLogRetentionDays)"
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
          :disabled-trigger="isAtDefault('studioAdvanced', 'expensiveQueryRows')"
          :disabled="isAtDefault('studioAdvanced', 'expensiveQueryRows')"
          @click="resetLeaf('studioAdvanced', 'expensiveQueryRows')"
        />
      </div>
      <NumberStepperInput
        :id="expensiveQueryRowsId"
        :min="EXPENSIVE_QUERY_ROWS_RANGE.min"
        :max="EXPENSIVE_QUERY_ROWS_RANGE.max"
        :aria-invalid="!!expensiveQueryRowsError || undefined"
        data-testid="settings-expensive-query-rows"
        :model-value="String(draft.studioAdvanced.expensiveQueryRows)"
        @input="onExpensiveQueryRowsInput"
      />
      <FieldError v-if="expensiveQueryRowsError" data-testid="settings-expensive-query-rows-error">
        {{ expensiveQueryRowsError }}
      </FieldError>
      <FieldDescription v-else data-testid="settings-expensive-query-rows-description">{{
        t('studio.settings.advanced.expensiveRows.description')
      }}</FieldDescription>
    </Field>
  </div>
</template>
