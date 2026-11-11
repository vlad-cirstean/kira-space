<script setup lang="ts">
import type { RoutedPrompt } from '@shared/domain/prompts';
import { useMutation } from '@tanstack/vue-query';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { t } from '@theme/i18n';
import { computed, onUnmounted } from 'vue';
import { control } from '../bridge/control';
import { useProofSessions } from './queries';

// The routed `proof` popup: a validation session is ready, or has an edit waiting. Open raises the
// proof window on it and closes the popup; the backend then withdraws the entry. Later hides it here
// and keeps it routed. A session belongs to at most one task; no task, no task line.
const props = defineProps<{ entry: RoutedPrompt; more: number }>();
const emit = defineEmits<{ hide: [] }>();
const open = useMutation({
  mutationFn: () => control.proofOpenWindow(props.entry.ref),
  onSuccess: () => emit('hide'),
});
const sessions = useProofSessions();
// The agent can attach the session to a task after the popup is up.
onUnmounted(control.onProofChanged(() => void sessions.refetch()));
const taskTitle = computed(
  () => sessions.data.value?.find((s) => s.id === props.entry.ref)?.taskTitle ?? '',
);
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('hide')">
    <DialogContent size="md" data-testid="proof-ready-dialog">
      <DialogHeader>
        <DialogTitle>{{ entry.title }}</DialogTitle>
      </DialogHeader>
      <DialogBody v-if="taskTitle || more > 0">
        <p v-if="taskTitle" class="m-0" data-testid="proof-ready-task">
          {{ t('studio.proof.ready.task.label', { taskTitle }) }}
        </p>
        <p v-if="more > 0" class="m-0 text-muted-foreground" data-testid="proof-ready-more">
          {{ t('common.moreWaiting.status', { count: more }) }}
        </p>
      </DialogBody>
      <DialogFooter>
        <Button
          data-dialog-initial
          variant="dialog"
          size="kira-lg"
          data-testid="proof-ready-later"
          @click="emit('hide')"
        >
          {{ t('common.later.label') }}
        </Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :pending="open.isPending.value"
          data-testid="proof-ready-open"
          @click="open.mutate()"
        >
          {{ t('common.action.open.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
