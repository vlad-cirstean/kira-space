<script setup lang="ts">
import type { RoutedPrompt } from '@shared/domain/prompts';
import { useMutation } from '@tanstack/vue-query';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { toast } from '@theme/components/ui/sonner';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { computed, onUnmounted } from 'vue';
import { control } from '../bridge/control';
import { useProofSessions } from './queries';

// The routed `proof` popup: a validation session is ready, or has an edit waiting. Open hides the
// popup at once and raises the proof window; the backend then withdraws the entry. Later hides it
// and keeps it routed. The title is the task, the description the proof; no task, the entry title.
// Rendered once the sessions list is known, so the first frame holds the final text.
const props = defineProps<{ entry: RoutedPrompt; more: number }>();
const emit = defineEmits<{ hide: [] }>();
const open = useMutation({
  mutationFn: () => control.proofOpenWindow(props.entry.ref),
  onError: (err) => toast.error(errorText(err)),
});
function openWindow(): void {
  emit('hide');
  open.mutate();
}
const sessions = useProofSessions();
// The agent can attach the session to a task after the popup is up.
onUnmounted(control.onProofChanged(() => void sessions.refetch()));
const session = computed(() => sessions.data.value?.find((s) => s.id === props.entry.ref));
const taskTitle = computed(() => session.value?.taskTitle ?? '');
</script>

<template>
  <Dialog v-if="sessions.isFetched.value" :open="true" @update:open="(v) => !v && emit('hide')">
    <DialogContent size="md" data-testid="proof-ready-dialog">
      <DialogHeader>
        <DialogTitle>{{ taskTitle || entry.title }}</DialogTitle>
        <DialogDescription v-if="taskTitle" data-testid="proof-ready-proof">
          {{ session?.name || entry.title }}
        </DialogDescription>
      </DialogHeader>
      <DialogBody v-if="more > 0">
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
          data-testid="proof-ready-open"
          @click="openWindow()"
        >
          {{ t('common.action.open.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
