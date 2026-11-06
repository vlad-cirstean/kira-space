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
import { control } from '../bridge/control';

// The routed `proof` popup: a validation session is ready, or has an edit waiting. Open raises the
// proof window on it (the backend withdraws the popup); Later hides it here and keeps it routed.
const props = defineProps<{ entry: RoutedPrompt; more: number }>();
const emit = defineEmits<{ hide: [] }>();
const open = useMutation({ mutationFn: () => control.proofOpenWindow(props.entry.ref) });
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('hide')">
    <DialogContent size="md" data-testid="proof-ready-dialog">
      <DialogHeader>
        <DialogTitle>{{ entry.title }}</DialogTitle>
      </DialogHeader>
      <DialogBody v-if="more > 0">
        <p class="m-0 text-muted-foreground" data-testid="proof-ready-more">
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
