<script setup lang="ts">
import type { RoutedPrompt } from '@shared/domain/prompts';
import { useQuery } from '@tanstack/vue-query';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { t } from '@theme/i18n';
import { control } from '../bridge/control';
import { useConnectionDialogStore } from '../state/connectionDialogs';
import { useConnectionsStore } from '../state/connections';

// The routed `masking` popup: columns on a connection await a masking review. Review masking opens the
// connection's Privacy tab; Later hides it here and the backend keeps it routed until reviewed.
const props = defineProps<{ entry: RoutedPrompt; more: number }>();
const emit = defineEmits<{ hide: [] }>();
const connectionsStore = useConnectionsStore();
const dialogStore = useConnectionDialogStore();

const pending = useQuery({
  queryKey: ['maskReviewPending', props.entry.ref],
  queryFn: () => control.maskReviewPending(props.entry.ref),
});

function review(): void {
  const record = connectionsStore.connectionRecord(props.entry.ref);
  if (record) dialogStore.openFile(record.file, 'privacy');
  emit('hide');
}

import { formatShortcut } from '@workbench/shortcuts/keys';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';

const onChord = shortcutKeydown(['form.submit'], () => review());
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('hide')">
    <DialogContent size="sm" prompt data-testid="masking-review-dialog" @keydown="onChord">
      <DialogHeader close-testid="masking-review-close">
        <DialogTitle>{{ entry.title }}</DialogTitle>
        <DialogDescription data-testid="masking-review-body">
          {{ t('studio.prompt.maskingReview.body', { count: pending.data.value?.count ?? 0 }) }}
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <template v-if="more > 0" #start>
          <span data-testid="masking-review-more">{{
            t('common.moreWaiting.status', { count: more })
          }}</span>
        </template>
        <Button
          variant="dialog-primary"
          :kbd="formatShortcut('form.submit')"
          size="kira-lg"
          data-testid="masking-review-open"
          @click="review"
        >
          {{ t('studio.prompt.maskingReview.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
