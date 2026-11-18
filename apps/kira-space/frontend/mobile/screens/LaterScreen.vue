<script setup lang="ts">
import { shelfIds } from '@ade/board/shelf';
import { useBoard } from '@ade/readQueries';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import QueryState from '@workbench/components/QueryState.vue';
import { storeToRefs } from 'pinia';
import { computed, ref } from 'vue';
import PermissionHint from '../components/PermissionHint.vue';
import { useAuthStore } from '../state/auth';
import { newIntentKey, useAdeWrites } from '../state/useAdeWrites';
import LaterShelf from './LaterShelf.vue';

// Later and Backlog: add a task to Later, drag rows by their handle to reorder or to move them
// between the two lists, tap a row for its notes.
const board = useBoard();
const { permissions } = storeToRefs(useAuthStore());
const writes = useAdeWrites();
const text = ref('');
const error = ref('');
const canWrite = computed(() => permissions.value.write);

const tasksOf = (shelf: 'later' | 'backlog') => {
  const b = board.data.value;
  if (!b) return [];
  const byId = new Map(b.tasks.map((task) => [task.id, task]));
  return shelfIds(b, shelf).flatMap((id) => byId.get(id) ?? []);
};
const later = computed(() => tasksOf('later'));
const backlog = computed(() => tasksOf('backlog'));

async function add(): Promise<void> {
  const value = text.value.trim();
  if (!value || writes.captureTask.isPending.value) return;
  error.value = '';
  try {
    await writes.captureTask.mutateAsync({ title: value, key: newIntentKey() });
    text.value = '';
  } catch (err) {
    error.value = errorText(err);
  }
}
</script>

<template>
  <section class="flex flex-col" data-testid="later-screen">
    <PermissionHint v-if="!canWrite" :what="t('mobile.later.writeOff.hint')" />
    <form v-else class="flex gap-2 border-b border-border p-3" @submit.prevent="add">
      <Input
        :aria-label="t('space.ade.later.add.label')"
        :model-value="text"
        size="touch"
        class="flex-1"
        :placeholder="t('space.ade.later.add.label')"
        enterkeyhint="done"
        @update:model-value="(v) => (text = String(v))"
        data-testid="later-add"
      />
      <Button
        size="touch"
        type="submit"
        variant="dialog-primary"
        :pending="writes.captureTask.isPending.value"
        :disabled="!text.trim()"
        data-testid="later-add-submit"
      >
        {{ t('common.action.add.label') }}
      </Button>
    </form>
    <Alert v-if="error" variant="error" strip data-testid="later-write-error">
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>
    <QueryState :query="board" :is-empty="() => false">
      <template #error>
        <Alert variant="error" strip data-testid="later-error">{{
          t('mobile.later.loadError.error')
        }}</Alert>
      </template>
      <LaterShelf shelf="later" :tasks="later" :can-write="canWrite" @error="(m) => (error = m)" />
      <LaterShelf
        shelf="backlog"
        :tasks="backlog"
        :can-write="canWrite"
        @error="(m) => (error = m)"
      />
    </QueryState>
  </section>
</template>
