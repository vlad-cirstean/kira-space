<script setup lang="ts">
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { useTimeoutFn } from '@vueuse/core';
import { useMutationAction } from '@workbench/behaviour/asyncAction';
import { TIMING } from '@workbench/behaviour/timing';
import { computed, ref } from 'vue';
import { useCaptureTask } from '../queries';

// Enter adds the text as a task at the top of Later; the note confirms briefly.
const text = ref('');
const note = ref('');
const add = useCaptureTask();
const capturing = useMutationAction(add);
const shown = computed(() => capturing.error.value ?? note.value);
const { start: hideNote } = useTimeoutFn(
  () => {
    note.value = '';
    capturing.reset();
  },
  TIMING.copiedFeedback,
  { immediate: false },
);

async function onEnter(): Promise<void> {
  const value = text.value.trim();
  if (!value || add.isPending.value) return;
  note.value = '';
  if (await capturing.run({ title: value, day: null })) {
    text.value = '';
    note.value = t('space.ade.later.added.announce');
  }
  hideNote();
}
</script>

<template>
  <div class="relative flex min-w-0 flex-1 items-center gap-2 @5xl:flex-none">
    <Label for="ade-capture" class="sr-only">{{ t('space.ade.later.add.label') }}</Label>
    <Input
      id="ade-capture"
      v-model="text"
      :placeholder="t('space.ade.later.add.placeholder')"
      class="w-full min-w-32 max-w-72 @5xl:w-72"
      data-testid="ade-capture"
      @keydown.enter="onEnter"
    />
    <span
      v-if="shown"
      class="absolute inset-y-0 right-full mr-2 flex items-center whitespace-nowrap bg-chrome text-kira-sm text-muted-foreground"
      data-testid="ade-capture-note"
      >{{
        shown
      }}</span
    >
  </div>
</template>
