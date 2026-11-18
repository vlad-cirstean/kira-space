<script setup lang="ts">
import type { ApiEnvironmentFileModel } from '@shared/domain/apiConfig';
import { PALETTE_COLOR_CHOICES } from '@shared/domain/color';
import { Field, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import SwatchPicker from '@theme/SwatchPicker.vue';
import { useId } from 'vue';

// The environment file's own keys: name, description and colour. Edits land in the dialog's draft.
const props = defineProps<{ errorFor: (pointer: string) => string | undefined }>();
const env = defineModel<ApiEnvironmentFileModel>({ required: true });
const idBase = useId();
</script>

<template>
  <div class="flex flex-col gap-3 p-3" data-testid="api-section-environment">
    <div class="flex items-start gap-2">
      <Field class="flex-1">
        <FieldLabel :for="`${idBase}-name`">{{ t('common.field.name.label') }}</FieldLabel>
        <Input
          :id="`${idBase}-name`"
          :model-value="env.name"
          :aria-invalid="props.errorFor('/name') ? true : undefined"
          data-testid="api-environment-name"
          @update:model-value="(v) => (env.name = String(v))"
        />
        <FieldError v-if="props.errorFor('/name')">{{ props.errorFor('/name') }}</FieldError>
      </Field>
      <Field class="flex-1">
        <FieldLabel :for="`${idBase}-description`">{{
          t('common.field.description.label')
        }}</FieldLabel>
        <Input
          :id="`${idBase}-description`"
          :model-value="env.description ?? ''"
          :aria-invalid="props.errorFor('/description') ? true : undefined"
          data-testid="api-environment-description"
          @update:model-value="(v) => (env.description = String(v))"
        />
        <FieldError v-if="props.errorFor('/description')">{{
          props.errorFor('/description')
        }}</FieldError>
      </Field>
      <Field class="flex-none">
        <FieldLabel :id="`${idBase}-colour`">{{ t('common.field.colour.label') }}</FieldLabel>
        <SwatchPicker
          :model-value="env.color ?? 'none'"
          :colors="PALETTE_COLOR_CHOICES"
          class="h-control"
          :labelledby="`${idBase}-colour`"
          data-testid="api-environment-color-picker"
          @update:model-value="(v) => (env.color = v)"
        />
        <FieldError v-if="props.errorFor('/color')">{{ props.errorFor('/color') }}</FieldError>
      </Field>
    </div>
  </div>
</template>
