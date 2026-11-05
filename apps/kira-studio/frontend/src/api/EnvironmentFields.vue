<script setup lang="ts">
import type { ApiEnvironmentFileModel } from '@shared/domain/apiConfig';
import { PALETTE_COLOR_CHOICES } from '@shared/domain/color';
import { Field, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colourLabel } from '@theme/connColor';
import { t } from '@theme/i18n';
import SwatchRadio from '@theme/SwatchRadio.vue';

// The environment file's own keys: name, description and colour. Edits land in the dialog's draft.
const props = defineProps<{ errorFor: (pointer: string) => string | undefined }>();
const env = defineModel<ApiEnvironmentFileModel>({ required: true });
</script>

<template>
  <div class="flex flex-col gap-3 p-3" data-testid="api-section-environment">
    <div class="flex items-start gap-2">
      <Field class="flex-1">
        <FieldLabel>{{ t('common.field.name.label') }}</FieldLabel>
        <Input
          :model-value="env.name"
          :aria-invalid="props.errorFor('/name') ? true : undefined"
          data-testid="api-environment-name"
          @update:model-value="(v) => (env.name = String(v))"
        />
        <FieldError v-if="props.errorFor('/name')">{{ props.errorFor('/name') }}</FieldError>
      </Field>
      <Field class="flex-1">
        <FieldLabel>{{ t('common.field.description.label') }}</FieldLabel>
        <Input
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
        <FieldLabel>{{ t('common.field.colour.label') }}</FieldLabel>
        <fieldset
          class="color-picker m-0 flex h-6.5 flex-wrap items-center gap-1 border-0 p-0"
          :aria-label="t('studio.api.variablesDialog.colour.aria')"
          data-testid="api-environment-color-picker"
        >
          <Tooltip v-for="color in PALETTE_COLOR_CHOICES" :key="color">
            <TooltipTrigger as-child>
              <SwatchRadio
                name="environment-color"
                :value="color"
                :color="color"
                :checked="(env.color ?? 'none') === color"
                @change="env.color = color"
              />
            </TooltipTrigger>
            <TooltipContent>{{ colourLabel(color) }}</TooltipContent>
          </Tooltip>
        </fieldset>
        <FieldError v-if="props.errorFor('/color')">{{ props.errorFor('/color') }}</FieldError>
      </Field>
    </div>
  </div>
</template>
