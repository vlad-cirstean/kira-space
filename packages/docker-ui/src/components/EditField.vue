<script setup lang="ts">
import StatusDot from '@theme/components/StatusDot.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { useFormErrors } from '@workbench/behaviour/formErrors';
import { computed, inject, useId } from 'vue';
import type { EditMode } from '../lib/editDiff';
import { inPlaceSectionKey } from '../lib/editSection';

const props = defineProps<{
  field: string;
  label: string;
  mode: EditMode;
  changed?: boolean;
  error?: string;
  description?: string;
  // Several controls: the label names the group, not one control.
  multi?: boolean;
}>();

const id = useId();

const inPlaceTab = inject(inPlaceSectionKey, undefined);
const form = useFormErrors({
  field: { value: () => props.changed, error: () => props.error ?? null },
});
const needsRecreate = computed(() => inPlaceTab === true && props.mode === 'recreate');
</script>

<template>
  <!-- biome-ignore lint/a11y/useSemanticElements: a fieldset cannot be the responsive grid. -->
  <Field
    orientation="responsive"
    role="group"
    :aria-label="label"
    :data-testid="`docker-edit-field-${field}`"
    :data-changed="changed ? 'true' : 'false'"
  >
    <FieldLabel :for="multi ? undefined : id">
      {{ label }}
      <TextTooltip v-if="changed" :text="t('common.changed.status')"
        ><StatusDot tone="warn" :label="t('common.changed.status')" /></TextTooltip
      >
    </FieldLabel>
    <slot v-bind="{ invalid: form.invalidAttrs('field'), id }" />
    <FieldError
      v-if="form.errorFor('field')"
      :id="form.errorId('field')"
      data-testid="docker-edit-field-error"
      >{{
        form.errorFor('field')
      }}</FieldError
    >
    <FieldDescription v-else-if="needsRecreate" data-testid="docker-edit-field-recreate-hint">{{
      t('docker.edit.needsRecreate.hint')
    }}</FieldDescription>
    <FieldDescription v-else-if="description">{{ description }}</FieldDescription>
  </Field>
</template>
