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
import { formatElapsed } from '@theme/format';
import { t } from '@theme/i18n';
import { usePendingDecision } from '@workbench/behaviour/pendingDecision';
import { formatShortcut } from '@workbench/shortcuts/keys';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';

// Shared shell for a pairing prompt PromptHost mounts. Renders nothing while `requestId` is absent;
// Shift+Escape and the frame's close hide it (`hide`), only Deny denies. Mod+Enter approves,
// Mod+Backspace denies; single keys do nothing. The countdown keys on the
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

const onChord = shortcutKeydown(['form.submit', 'prompt.deny'], (id) =>
  id === 'form.submit' ? emit('approve') : emit('deny'),
);
</script>

<template>
  <Dialog v-if="requestId" :open="true" @update:open="(v) => !v && emit('hide')">
    <DialogContent size="sm" prompt :data-testid="`${testidPrefix}-dialog`" @keydown="onChord">
      <DialogHeader>
        <DialogTitle>{{ title }}</DialogTitle>
      </DialogHeader>

      <DialogBody>
        <slot />
        <p class="m-0 text-subtle" :data-testid="`${testidPrefix}-expires`">
          {{ t('common.expires.status', { time: formatElapsed(remainingSeconds * 1000) }) }}
        </p>
      </DialogBody>

      <DialogFooter>
        <template v-if="more > 0" #start>
          <span :data-testid="`${testidPrefix}-queue-count`">{{
            t('common.moreWaiting.status', { count: more })
          }}</span>
        </template>
        <Button
          ref="denyButton"
          :kbd="formatShortcut('prompt.deny')"
          variant="dialog-deny"
          size="kira-lg"
          :data-testid="`${testidPrefix}-deny`"
          @click="emit('deny')"
        >
          {{ t('common.action.deny.label') }}
        </Button>
        <Button
          variant="dialog-approve"
          :kbd="formatShortcut('form.submit')"
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
