<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
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
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { ref, watch } from 'vue';
import { newIntentKey, useAdeWrites } from '../state/useAdeWrites';

// A quick reply to an agent that waits in its Claude Code session: the text is pasted into the
// terminal and submitted, without taking the terminal over.
const props = defineProps<{ open: boolean; sessionId: string; context: string }>();
const emit = defineEmits<{ close: [] }>();

const writes = useAdeWrites();
const message = ref('');
const error = ref('');
let key = newIntentKey();

watch(
  () => props.open,
  (o) => {
    if (!o) return;
    message.value = '';
    error.value = '';
    key = newIntentKey();
  },
);

async function send(): Promise<void> {
  const text = message.value.trim();
  if (!text || writes.sendReply.isPending.value) return;
  error.value = '';
  try {
    await writes.sendReply.mutateAsync({ sessionId: props.sessionId, message: text, key });
    emit('close');
  } catch (err) {
    error.value = errorText(err);
  }
}
</script>

<template>
  <Dialog :open="open" @update:open="(v) => !v && emit('close')">
    <DialogContent size="sm" :dirty="message !== ''" data-testid="reply-sheet">
      <DialogHeader>
        <DialogTitle>{{ t('mobile.reply.dialog.title') }}</DialogTitle>
        <DialogDescription>{{ context }}</DialogDescription>
      </DialogHeader>
      <DialogBody>
        <Textarea
          :model-value="message"
          rows="4"
          class="text-kira-touch"
          data-testid="reply-input"
          @update:model-value="(v) => (message = String(v))"
        />
        <Alert v-if="error" variant="error" data-testid="reply-error">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
      </DialogBody>
      <DialogFooter>
        <Button
          size="touch"
          variant="dialog"
          class="flex-1"
          data-testid="reply-cancel"
          @click="emit('close')"
          >{{
            t('common.action.cancel.label')
          }}</Button
        >
        <Button
          size="touch"
          variant="dialog-approve"
          class="flex-1"
          :pending="writes.sendReply.isPending.value"
          :disabled="!message.trim()"
          data-testid="reply-send"
          @click="send"
        >
          {{ t('common.action.send.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
