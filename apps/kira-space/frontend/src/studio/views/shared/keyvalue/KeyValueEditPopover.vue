<script setup lang="ts">
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed, ref } from 'vue';
import { saveValueEdit } from './mutations';

const props = defineProps<{
  viewKey: string;
  keyName: string;
  disabled: boolean;
  label: string;
  /** An S3 object page edits through the cell editor dock instead of this popover. */
  object: boolean;
  /** Current string value, read when the toolbar button opens the popover. */
  currentValue: string;
}>();
const emit = defineEmits<{ editObject: [] }>();

const open = ref(false);
const anchorRef = ref<HTMLElement | null>(null);
const draft = ref('');
const original = ref('');

const { run, pending, error } = useAsyncAction(() =>
  saveValueEdit(props.viewKey, props.keyName, draft.value),
);

function openWith(value: string): void {
  original.value = value;
  draft.value = value;
  error.value = null;
  open.value = true;
}

function onOpen(): void {
  if (props.disabled) return;
  if (props.object) emit('editObject');
  else openWith(props.currentValue);
}

async function onSave(): Promise<void> {
  if (!props.keyName || props.disabled) return;
  if (await run()) open.value = false;
}

defineExpose({
  openWith,
  dirty: computed(() => open.value && draft.value !== original.value),
});
</script>

<template>
  <Popover :open="open && !object" @update:open="(v) => (open = v)">
    <div ref="anchorRef" class="relative">
      <TooltipIconButton
        :icon="ICON.edit"
        :label="label"
        :aria-label="t('common.action.edit.label')"
        disabled-trigger
        :disabled="disabled"
        data-testid="keyvalue-edit"
        @click="onOpen"
      />
      <PopoverAnchor :reference="anchorRef ?? undefined" />
    </div>
    <PopoverContent align="start" class="w-80" data-testid="keyvalue-edit-popover">
      <div class="flex flex-col gap-1.5 p-1.5">
        <SectionHeading :label="t('studio.keyValue.editValue.label')" />
        <Input
          v-model="draft"
          size="kira-lg"
          class="w-full"
          data-testid="keyvalue-edit-input"
          @keydown.enter="onSave"
        />
        <Alert v-if="error" variant="error" data-testid="keyvalue-edit-error">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
        <div class="flex justify-end gap-1">
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="keyvalue-edit-cancel"
            @click="open = false"
            >{{
              t('common.action.cancel.label')
            }}</Button
          >
          <Button
            variant="dialog-primary"
            size="kira-lg"
            data-testid="keyvalue-edit-save"
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
