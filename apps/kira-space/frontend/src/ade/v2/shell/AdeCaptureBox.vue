<script setup lang="ts">
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { useTimeoutFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import { ref } from 'vue';
import { useCaptureTask } from '../queries';

// Enter adds the text as a task at the top of Later; the note confirms briefly.
const text = ref('');
const note = ref('');
const add = useCaptureTask();
const { start: hideNote } = useTimeoutFn(
  () => {
    note.value = '';
  },
  TIMING.copiedFeedback,
  { immediate: false },
);

async function onEnter(): Promise<void> {
  const value = text.value.trim();
  if (!value || add.isPending.value) return;
  try {
    await add.mutateAsync({ title: value, day: null });
    text.value = '';
    note.value = t('space.ade.later.added.announce');
  } catch (err) {
    note.value = errorText(err);
  }
  hideNote();
}
</script>

<template>
  <div class="relative flex items-center gap-2">
    <Label for="ade-capture" class="sr-only">{{ t('space.ade.later.add.label') }}</Label>
    <Input
      id="ade-capture"
      v-model="text"
      :placeholder="t('space.ade.later.add.placeholder')"
      size="kira-lg"
      class="w-72"
      data-testid="ade-capture"
      @keydown.enter="onEnter"
    />
    <span
      v-if="note"
      class="absolute inset-y-0 right-full mr-2 flex items-center whitespace-nowrap bg-chrome text-kira-sm text-muted-foreground"
      data-testid="ade-capture-note"
      >{{
        note
      }}</span
    >
  </div>
</template>
