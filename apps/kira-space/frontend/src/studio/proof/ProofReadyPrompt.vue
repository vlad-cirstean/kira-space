<script setup lang="ts">
import type { RoutedPrompt } from '@shared/domain/prompts';
import { useMutation } from '@tanstack/vue-query';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { toast } from '@theme/components/ui/sonner';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { formatShortcut } from '@workbench/shortcuts/keys';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';
import { computed } from 'vue';
import { control } from '../bridge/control';
import { useProofSession } from './queries';

// The routed `proof` popup: a validation session is ready, or has an edit waiting. Open hides the
// popup at once and raises the proof window; the backend then withdraws the entry. Later hides it
// and keeps it routed. The title is the task, the description the proof; no task, the entry title.
// Rendered once the proof is known, so the first frame holds the final text.
const props = defineProps<{ entry: RoutedPrompt; more: number }>();
const emit = defineEmits<{ hide: [] }>();
const open = useMutation({
  mutationFn: () => control.proofOpenWindow(session.value?.taskId ?? '', session.value?.slug ?? ''),
  onError: (err) => toast.error(errorText(err)),
});
function openWindow(): void {
  emit('hide');
  open.mutate();
}
const sessionQuery = useProofSession(() => props.entry.ref);
const session = computed(() => sessionQuery.data.value);
const taskTitle = computed(() => session.value?.taskTitle ?? '');

const onChord = shortcutKeydown(['form.submit'], () => openWindow());
</script>

<template>
  <Dialog v-if="sessionQuery.isFetched.value" :open="true" @update:open="(v) => !v && emit('hide')">
    <DialogContent size="sm" prompt data-testid="proof-ready-dialog" @keydown="onChord">
      <DialogHeader>
        <DialogTitle>{{ taskTitle || entry.title }}</DialogTitle>
        <DialogDescription v-if="taskTitle" data-testid="proof-ready-proof">
          {{ session?.name || entry.title }}
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <template v-if="more > 0" #start>
          <span data-testid="proof-ready-more">{{
            t('common.moreWaiting.status', { count: more })
          }}</span>
        </template>
        <Button
          :kbd="formatShortcut('prompt.hide')"
          variant="dialog"
          size="kira-lg"
          data-testid="proof-ready-later"
          @click="emit('hide')"
        >
          {{ t('common.later.label') }}
        </Button>
        <Button
          variant="dialog-primary"
          :kbd="formatShortcut('form.submit')"
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
