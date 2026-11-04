<script setup lang="ts">
import { PALETTE_COLOR_CHOICES, type PaletteColor } from '@shared/domain/color';
import type { RunFile } from '@shared/domain/scripts';
import { Field, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import SwatchRadio from '@theme/SwatchRadio.vue';
import { computed, useId } from 'vue';
import { useAutomationsModule } from '../module';
import { paramsOf } from './runForm';
import ScriptBodyField from './ScriptBodyField.vue';

// Name, collection, colour and the command or prompt. A new collection name is just typed: the
// collection exists once a run names it.
const props = defineProps<{
  /** Secret params that will have a saved value once the form is saved. */
  saved: readonly string[];
  errorFor: (pointer: string) => string | undefined;
}>();
const file = defineModel<RunFile>({ required: true });
const ctx = useAutomationsModule();
const listId = useId();

const smart = computed(() => file.value.kind === 'smart');
const params = computed(() => paramsOf(file.value));
const body = computed({
  get: () => (smart.value ? (file.value.prompt ?? '') : (file.value.command ?? '')),
  set: (v: string) => {
    if (smart.value) file.value.prompt = v;
    else file.value.command = v;
  },
});
const color = computed({
  get: () => (file.value.color || 'none') as PaletteColor,
  set: (v: PaletteColor) => {
    file.value.color = v === 'none' ? '' : v;
  },
});
const nameError = computed(() => props.errorFor('/name'));
const bodyError = computed(() => props.errorFor(smart.value ? '/prompt' : '/command'));
</script>

<template>
  <div class="flex flex-col gap-3" data-testid="run-tab-script">
    <Field>
      <FieldLabel for="script-name">{{ t('common.field.name.label') }}</FieldLabel>
      <Input
        id="script-name"
        :model-value="file.name"
        :placeholder="t('automations.dialog.script.nameExample.example')"
        :aria-invalid="nameError ? true : undefined"
        data-testid="script-dialog-name"
        @update:model-value="(v) => (file.name = String(v))"
      />
      <FieldError v-if="nameError" data-testid="script-dialog-name-error">{{
        nameError
      }}</FieldError>
    </Field>

    <div class="flex items-end gap-3">
      <Field class="min-w-0 flex-1">
        <FieldLabel for="script-collection">{{ t('common.collection.label') }}</FieldLabel>
        <Input
          id="script-collection"
          :list="listId"
          :model-value="file.collection ?? ''"
          :placeholder="t('automations.form.collection.placeholder')"
          data-testid="script-dialog-collection"
          @update:model-value="(v) => (file.collection = String(v))"
        />
        <datalist :id="listId">
          <option v-for="c in ctx.scripts.collections()" :key="c.name" :value="c.name" />
        </datalist>
      </Field>
      <fieldset
        class="color-picker m-0 flex h-control flex-wrap items-center gap-1 border-0 p-0"
        :aria-label="t('common.field.colour.label')"
      >
        <Tooltip v-for="swatch in PALETTE_COLOR_CHOICES" :key="swatch">
          <TooltipTrigger as-child>
            <SwatchRadio
              name="script-color"
              :value="swatch"
              :color="swatch"
              :checked="color === swatch"
              @change="color = swatch"
            />
          </TooltipTrigger>
          <TooltipContent>{{
            swatch === 'none' ? t('common.field.noColour.label') : swatch
          }}</TooltipContent>
        </Tooltip>
      </fieldset>
    </div>

    <ScriptBodyField
      v-model="body"
      :kind="smart ? 'smart' : 'script'"
      :params="params"
      :ade="ctx.ade"
      :saved="saved"
      :error="bodyError"
      error-id="script-body-error"
    />
  </div>
</template>
