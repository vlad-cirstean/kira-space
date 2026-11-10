<script setup lang="ts">
import {
  CONNECTION_THROTTLE_RANGE,
  EXPLAIN_THRESHOLD_ROWS_RANGE,
  ROW_ESTIMATE_KINDS,
} from '@shared/domain/connection';
import type { ConnectionFileModel } from '@shared/domain/connectionConfig';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import NumberStepperInput from '@theme/NumberStepperInput.vue';
import { formatCount } from '@workbench/behaviour/format';
import { computed } from 'vue';
import { useSettingsStore } from '../../../state/settings';
import { schemaDialectFor } from '../../state/schemas';
import CheckboxRow from './CheckboxRow.vue';

// Read-only, auto-explain, the command throttle and the expensive query threshold.
const props = defineProps<{ errorFor: (pointer: string) => string | undefined }>();
const file = defineModel<ConnectionFileModel>({ required: true });

// Auto-explain is SQL-only; absent, not disabled, on the other kinds. Read through
// state/schemas.ts because project/ must not import views/ directly.
const isSqlKind = computed(() => schemaDialectFor(file.value.kind as never) !== undefined);

const throttleError = computed(() => {
  const v = file.value.throttle_per_sec ?? 0;
  if (!Number.isFinite(v)) return t('common.enterNumber.error');
  if (v !== 0 && (v < CONNECTION_THROTTLE_RANGE.min || v > CONNECTION_THROTTLE_RANGE.max)) {
    return t('studio.project.connectionDialog.throttle.range.status', {
      min: CONNECTION_THROTTLE_RANGE.min,
      max: CONNECTION_THROTTLE_RANGE.max,
    });
  }
  return props.errorFor('/throttle_per_sec');
});

// Only kinds whose plan carries a row estimate have a threshold; absent, not disabled, elsewhere.
const hasRowEstimate = computed(() => ROW_ESTIMATE_KINDS.has(file.value.kind as never));
const appThreshold = computed(() => useSettingsStore().advanced.expensiveQueryRows);

const explainThresholdError = computed(() => {
  const v = file.value.explain_threshold_rows;
  if (v === undefined) return props.errorFor('/explain_threshold_rows');
  if (
    !Number.isInteger(v) ||
    v < EXPLAIN_THRESHOLD_ROWS_RANGE.min ||
    v > EXPLAIN_THRESHOLD_ROWS_RANGE.max
  ) {
    return t('studio.settings.range.plain.status', {
      min: formatCount(EXPLAIN_THRESHOLD_ROWS_RANGE.min),
      max: formatCount(EXPLAIN_THRESHOLD_ROWS_RANGE.max),
    });
  }
  return props.errorFor('/explain_threshold_rows');
});

// Empty means the app default: the key is dropped, never written as 0.
function setExplainThreshold(value: string): void {
  const n = Number.parseInt(value, 10);
  file.value.explain_threshold_rows = Number.isNaN(n) ? undefined : n;
}

function setThrottle(value: string): void {
  const n = Number.parseFloat(value);
  file.value.throttle_per_sec = Number.isNaN(n) ? 0 : n;
}
</script>

<template>
  <div class="flex flex-col gap-3 p-3" data-testid="connection-section-advanced">
    <CheckboxRow
      :model-value="file.read_only ?? false"
      :label="t('common.state.readOnly.status')"
      :description="t('studio.project.connectionDialog.readOnly.description')"
      testid="connection-readonly"
      @update:model-value="(v) => (file.read_only = v)"
    />
    <CheckboxRow
      v-if="isSqlKind"
      :model-value="file.auto_explain ?? false"
      :label="t('studio.project.connectionDialog.autoExplain.label')"
      :description="t('studio.project.connectionDialog.autoExplain.description')"
      testid="connection-auto-explain"
      @update:model-value="(v) => (file.auto_explain = v)"
    />

    <Field v-if="hasRowEstimate" class="flex-1">
      <FieldLabel>{{ t('studio.settings.advanced.expensiveRows.label') }}</FieldLabel>
      <div class="flex items-center gap-1">
        <div class="w-40">
          <NumberStepperInput
            :min="EXPLAIN_THRESHOLD_ROWS_RANGE.min"
            :max="EXPLAIN_THRESHOLD_ROWS_RANGE.max"
            step="1000"
            :placeholder="formatCount(appThreshold)"
            :aria-invalid="explainThresholdError ? true : undefined"
            data-testid="connection-explain-threshold"
            :model-value="
              file.explain_threshold_rows === undefined ? '' : String(file.explain_threshold_rows)
            "
            @update:model-value="(v: string | number) => setExplainThreshold(String(v))"
          />
        </div>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          data-testid="connection-explain-threshold-reset"
          :disabled-trigger="file.explain_threshold_rows === undefined"
          :disabled="file.explain_threshold_rows === undefined"
          @click="file.explain_threshold_rows = undefined"
        />
      </div>
      <FieldError v-if="explainThresholdError" data-testid="connection-explain-threshold-error">
        {{ explainThresholdError }}
      </FieldError>
      <template v-else>
        <FieldDescription class="w-full" data-testid="connection-explain-threshold-description">
          {{
            t('studio.project.connectionDialog.explainThreshold.description', {
              default: formatCount(appThreshold),
            })
          }}
        </FieldDescription>
        <FieldDescription class="w-full" data-testid="connection-explain-threshold-engine">
          {{
            file.kind === 'clickhouse'
              ? t('studio.project.connectionDialog.explainThreshold.clickhouse.body')
              : t('studio.project.connectionDialog.explainThreshold.scan.body')
          }}
        </FieldDescription>
      </template>
    </Field>

    <Field class="flex-1">
      <FieldLabel>{{ t('studio.project.connectionDialog.throttle.label') }}</FieldLabel>
      <div class="w-24">
        <NumberStepperInput
          :min="0"
          :max="CONNECTION_THROTTLE_RANGE.max"
          step="0.5"
          :aria-invalid="throttleError ? true : undefined"
          data-testid="connection-throttle"
          :model-value="String(file.throttle_per_sec ?? 0)"
          @update:model-value="(v: string | number) => setThrottle(String(v))"
        />
      </div>
      <FieldError v-if="throttleError" data-testid="connection-throttle-error">
        {{ throttleError }}
      </FieldError>
      <FieldDescription v-else class="w-full" data-testid="connection-throttle-description">
        {{ t('studio.project.connectionDialog.throttle.description') }}
      </FieldDescription>
    </Field>
  </div>
</template>
