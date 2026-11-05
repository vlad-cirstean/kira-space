<script setup lang="ts">
import { CONNECTION_THROTTLE_RANGE } from '@shared/domain/connection';
import type { ConnectionFileModel } from '@shared/domain/connectionConfig';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import NumberStepperInput from '@theme/NumberStepperInput.vue';
import { computed } from 'vue';
import { schemaDialectFor } from '../../state/schemas';
import CheckboxRow from './CheckboxRow.vue';

// Read-only, auto-explain and the command throttle.
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
      <FieldDescription v-else class="w-full">
        {{ t('studio.project.connectionDialog.throttle.description') }}
      </FieldDescription>
    </Field>
  </div>
</template>
