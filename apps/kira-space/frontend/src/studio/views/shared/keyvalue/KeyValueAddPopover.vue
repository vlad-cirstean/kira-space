<script setup lang="ts">
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { FieldError } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { useFormErrors } from '@workbench/behaviour/formErrors';
import { computed, nextTick, ref } from 'vue';
import { addKey } from './mutations';

const props = defineProps<{
  viewKey: string;
  disabled: boolean;
  label: string;
  /** An S3 object page uploads instead of adding a key. */
  upload: boolean;
}>();
const emit = defineEmits<{ upload: [] }>();

const open = ref(false);
const anchorRef = ref<HTMLElement | null>(null);
const name = ref('');
const value = ref('');

const { run, pending, error } = useAsyncAction(() =>
  addKey(props.viewKey, name.value.trim(), value.value),
);
const { errorFor, errorId, invalidAttrs, submit, reset } = useFormErrors({
  name: {
    value: () => name.value,
    error: () => (name.value.trim() ? null : t('studio.keyValue.add.nameRequired.error')),
  },
});

async function onOpen(): Promise<void> {
  if (props.disabled) return;
  if (props.upload) {
    emit('upload');
    return;
  }
  name.value = '';
  value.value = '';
  error.value = null;
  open.value = true;
  await nextTick();
  reset();
}

async function onSave(): Promise<void> {
  if (props.disabled || !(await submit())) return;
  if (await run()) open.value = false;
}

defineExpose({ dirty: computed(() => open.value && (name.value !== '' || value.value !== '')) });
</script>

<template>
  <Popover :open="open" @update:open="(v) => (open = v)">
    <div ref="anchorRef" class="relative">
      <TooltipIconButton
        :icon="ICON.add"
        :label="label"
        :aria-label="t('common.action.add.label')"
        disabled-trigger
        :disabled="disabled"
        data-testid="keyvalue-add"
        @click="onOpen"
      />
      <PopoverAnchor :reference="anchorRef ?? undefined" />
    </div>
    <PopoverContent align="start" class="w-80" data-testid="keyvalue-add-popover">
      <div class="flex flex-col gap-1.5 p-1.5">
        <SectionHeading :label="t('studio.keyValue.add.title')" />
        <Input
          v-model="name"
          v-bind="invalidAttrs('name')"
          size="kira-lg"
          :placeholder="t('studio.keyValue.add.namePlaceholder.placeholder')"
          class="w-full"
          data-testid="keyvalue-add-name"
        />
        <FieldError
          v-if="errorFor('name')"
          :id="errorId('name')"
          data-testid="keyvalue-add-name-error"
          >{{
            errorFor('name')
          }}</FieldError
        >
        <Input
          v-model="value"
          size="kira-lg"
          :placeholder="t('studio.keyValue.add.valuePlaceholder.placeholder')"
          class="w-full"
          data-testid="keyvalue-add-value"
          @keydown.enter="onSave"
        />
        <Alert v-if="error" variant="error" data-testid="keyvalue-add-error">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
        <div class="flex justify-end gap-1">
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="keyvalue-add-cancel"
            @click="open = false"
            >{{
              t('common.action.cancel.label')
            }}</Button
          >
          <Button
            variant="dialog-primary"
            size="kira-lg"
            data-testid="keyvalue-add-save"
            :pending="pending"
            :disabled="disabled"
            @click="onSave"
            >{{
              t('common.action.save.label')
            }}</Button
          >
        </div>
      </div>
    </PopoverContent>
  </Popover>
</template>
