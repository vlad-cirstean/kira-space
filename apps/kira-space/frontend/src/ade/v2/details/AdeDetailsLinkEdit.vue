<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { useFocus } from '@vueuse/core';
import { useInlineEdit } from '@workbench/behaviour/inlineEdit';
import { computed, useTemplateRef } from 'vue';

// One link's inline edit: Enter or blur commits, Escape cancels, an empty commit removes. Mounted
// only while editing, so every edit starts from fresh state.
const props = defineProps<{ text: string; commit: (text: string) => Promise<void> }>();
const emit = defineEmits<{ done: [] }>();

// A blur that follows a successful commit must not commit the stale text again.
let finished = false;
const edit = useInlineEdit({
  source: () => props.text,
  allowEmpty: true,
  commit: async (text) => {
    if (finished) return;
    await props.commit(text);
    finished = true;
    emit('done');
  },
  cancel: () => {
    finished = true;
    emit('done');
  },
});

const input = useTemplateRef<{ $el: HTMLInputElement }>('input');
useFocus(
  computed(() => input.value?.$el),
  { initialValue: true },
);
</script>

<template>
  <div class="flex min-w-0 flex-1 flex-col gap-1">
    <Input
      ref="input"
      :model-value="edit.text.value"
      :aria-label="t('space.ade.details.link.aria')"
      :placeholder="t('space.ade.details.link.placeholder')"
      size="kira"
      data-testid="ade-details-link-input"
      @update:model-value="edit.onInput"
      @blur="edit.onCommit"
      @keydown="edit.onKeydown"
    />
    <Alert v-if="edit.error.value" variant="error" data-testid="ade-details-link-error"
      ><AlertDescription>{{ edit.error.value }}</AlertDescription></Alert
    >
  </div>
</template>
