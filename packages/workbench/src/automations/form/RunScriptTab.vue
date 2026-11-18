<script setup lang="ts">
import { PALETTE_COLOR_CHOICES, type PaletteColor } from '@shared/domain/color';
import type { RunFile } from '@shared/domain/scripts';
import { Field, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import SwatchPicker from '@theme/SwatchPicker.vue';
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
const colourId = useId();

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
      <Field class="flex-none">
        <FieldLabel :id="colourId">{{ t('common.field.colour.label') }}</FieldLabel>
        <SwatchPicker
          :model-value="color"
          :colors="PALETTE_COLOR_CHOICES"
          class="h-control"
          :labelledby="colourId"
          @update:model-value="(v) => (color = v as PaletteColor)"
        />
      </Field>
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
