<script setup lang="ts">
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
import { usePendingDecision } from '@workbench/behaviour/pendingDecision';

// Shared shell for a pairing prompt PromptHost mounts. Renders nothing while `requestId` is absent;
// Escape and the frame's close hide it (`hide`), only Deny denies. The countdown keys on the
// request id (usePendingDecision), so the queue advancing A to B re-arms Deny focus and the timer.
const props = defineProps<{
  title: string;
  requestId?: string;
  expiresAtMs?: number;
  more: number;
  testidPrefix: string;
}>();
const emit = defineEmits<{ deny: []; approve: []; hide: [] }>();

const { remainingSeconds } = usePendingDecision({
  pendingId: () => props.requestId,
  expiresAtMs: () => props.expiresAtMs,
});
</script>

<template>
  <Dialog v-if="requestId" :open="true" @update:open="(v) => !v && emit('hide')">
    <DialogContent size="sm" :data-testid="`${testidPrefix}-dialog`">
      <DialogHeader>
        <DialogTitle>{{ title }}</DialogTitle>
      </DialogHeader>

      <DialogBody>
        <slot />
        <p class="m-0 text-subtle" :data-testid="`${testidPrefix}-expires`">
          {{ t('common.expiresIn.status', { seconds: remainingSeconds }) }}
        </p>
        <p v-if="more > 0" class="m-0 text-subtle" :data-testid="`${testidPrefix}-queue-count`">
          {{ t('common.moreWaiting.status', { count: more }) }}
        </p>
      </DialogBody>

      <DialogFooter>
        <Button
          ref="denyButton"
          variant="dialog-deny"
          size="kira-lg"
          :data-testid="`${testidPrefix}-deny`"
          @click="emit('deny')"
        >
          {{ t('common.action.deny.label') }}
        </Button>
        <Button
          variant="dialog-approve"
          size="kira-lg"
          :data-testid="`${testidPrefix}-approve`"
          @click="emit('approve')"
        >
          {{ t('common.action.approve.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
