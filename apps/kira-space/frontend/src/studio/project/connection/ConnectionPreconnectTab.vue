<script setup lang="ts">
import { PRECONNECT_DELAY_RANGE } from '@shared/domain/connection';
import type { ConnectionFileModel } from '@shared/domain/connectionConfig';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import NumberStepperInput from '@theme/NumberStepperInput.vue';
import { wrapSelectionOnType } from '@theme/wrapSelection';
import { computed } from 'vue';
import CheckboxRow from './CheckboxRow.vue';

// The command run before a connect. An emptied command removes the whole preconnect mapping.
const props = defineProps<{ errorFor: (pointer: string) => string | undefined }>();
const file = defineModel<ConnectionFileModel>({ required: true });

const command = computed({
  get: () => file.value.preconnect?.command ?? '',
  set: (v: string) => {
    file.value.preconnect = v.trim() === '' ? undefined : { ...file.value.preconnect, command: v };
  },
});

const delayError = computed(() => {
  const v = file.value.preconnect?.delay_sec ?? 0;
  if (!Number.isInteger(v) || v < PRECONNECT_DELAY_RANGE.min || v > PRECONNECT_DELAY_RANGE.max) {
    return t('studio.project.connectionDialog.preconnectDelay.range.status', {
      min: PRECONNECT_DELAY_RANGE.min,
      max: PRECONNECT_DELAY_RANGE.max,
    });
  }
  return undefined;
});

function patch(change: { sidecar?: boolean; delay_sec?: number }): void {
  if (file.value.preconnect) file.value.preconnect = { ...file.value.preconnect, ...change };
}
function setDelay(value: string): void {
  const n = Number.parseInt(value, 10);
  patch({ delay_sec: Number.isNaN(n) ? 0 : n });
}
</script>

<template>
  <div class="flex flex-col gap-3 p-3" data-testid="connection-section-preconnect">
    <Field class="flex-1">
      <FieldLabel>{{ t('studio.project.connectionDialog.preconnect.label') }}</FieldLabel>
      <Textarea
        v-model="command"
        :aria-invalid="props.errorFor('/preconnect') ? true : undefined"
        class="font-data"
        rows="4"
        maxlength="2000"
        data-testid="connection-preconnect"
        @keydown="wrapSelectionOnType"
      />
      <FieldDescription class="w-full">
        {{ t('studio.project.connectionDialog.preconnect.description') }}
      </FieldDescription>
      <span
        v-if="command"
        class="text-warn text-kira-sm"
        data-testid="connection-preconnect-warning"
      >
        {{ t('studio.project.connectionDialog.preconnect.warning.body') }}
      </span>
      <FieldError v-if="props.errorFor('/preconnect')">{{
        props.errorFor('/preconnect')
      }}</FieldError>
    </Field>

    <CheckboxRow
      v-if="command"
      :model-value="file.preconnect?.sidecar ?? false"
      :label="t('studio.project.connectionDialog.sidecar.label')"
      :description="t('studio.project.connectionDialog.sidecar.description')"
      testid="connection-preconnect-sidecar"
      @update:model-value="(v) => patch({ sidecar: v })"
    />

    <Field v-if="command" class="flex-1">
      <FieldLabel>{{ t('studio.project.connectionDialog.preconnectDelay.label') }}</FieldLabel>
      <div class="w-24">
        <NumberStepperInput
          :min="0"
          :max="PRECONNECT_DELAY_RANGE.max"
          step="1"
          :aria-invalid="delayError ? true : undefined"
          data-testid="connection-preconnect-delay"
          :model-value="String(file.preconnect?.delay_sec ?? 0)"
          @update:model-value="(v: string | number) => setDelay(String(v))"
        />
      </div>
      <FieldError v-if="delayError" data-testid="connection-preconnect-delay-error">
        {{ delayError }}
      </FieldError>
      <FieldDescription v-else class="w-full">
        {{ t('studio.project.connectionDialog.preconnectDelay.description') }}
      </FieldDescription>
    </Field>
  </div>
</template>
