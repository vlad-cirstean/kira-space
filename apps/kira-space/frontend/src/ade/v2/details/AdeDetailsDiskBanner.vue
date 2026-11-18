<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertAction, AlertDescription, AlertTitle } from '@theme/components/ui/alert';
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
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { ref } from 'vue';
import type { DiskChange } from '../wire';
import { refreshDisk, useAcceptDisk, useDiscardDisk, useDiskChanges } from './useDetailsDisk';

// The task folder against what Kira holds: files edited outside a submit. Stat on open, window focus and
// Refresh; no watcher. Accept makes a change the held copy, Discard restores the held copy.
const props = defineProps<{ taskId: string; readonly?: boolean }>();

const changes = useDiskChanges(
  () => props.taskId,
  () => !props.readonly,
);
const accept = useAcceptDisk();
const discard = useDiscardDisk();
const reviewing = ref(false);

function resolve(kind: 'accept' | 'discard', paths: string[]): void {
  (kind === 'accept' ? accept : discard).mutate({ taskId: props.taskId, paths });
}

// A file with an open approval is the approval's to settle; one that cannot be read cannot be accepted.
function locked(c: DiskChange, needsText = false): boolean {
  return c.pending || (needsText && !!c.problem);
}

function stateLabel(c: DiskChange): string {
  return c.problem || t(`space.ade.details.diskState.${c.state}.label`);
}
</script>

<template>
  <div v-if="!readonly" class="flex flex-col gap-2" data-testid="ade-details-disk">
    <div class="flex items-center gap-1">
      <Button
        variant="toolbar"
        size="kira"
        data-testid="ade-details-disk-refresh"
        :disabled="changes.isFetching.value"
        @click="refreshDisk()"
      >
        <CodiconIcon name="refresh" :size="13" />
        {{ t('common.action.refresh.label') }}
      </Button>
    </div>
    <Alert v-if="changes.data.value?.length" variant="warn" data-testid="ade-details-disk-banner">
      <AlertTitle>{{ t('space.ade.details.diskBanner.title') }}</AlertTitle>
      <AlertDescription>{{
        t('space.ade.details.diskBanner.body', { count: changes.data.value.length })
      }}</AlertDescription>
      <AlertAction>
        <Button
          variant="dialog"
          size="kira"
          data-testid="ade-details-disk-review"
          @click="reviewing = true"
        >
          {{ t('space.ade.details.diskReview.label') }}
        </Button>
      </AlertAction>
    </Alert>
    <Dialog v-model:open="reviewing">
      <DialogContent size="xl" :dirty="false" data-testid="ade-details-disk-dialog">
        <DialogHeader>
          <DialogTitle>{{ t('space.ade.details.diskDialog.title') }}</DialogTitle>
          <DialogDescription>{{ t('space.ade.details.diskDialog.body') }}</DialogDescription>
        </DialogHeader>
        <DialogBody>
          <FieldDescription v-if="!changes.data.value?.length">{{
            t('space.ade.details.diskDialog.empty.body')
          }}</FieldDescription>
          <ul class="flex flex-col gap-3">
            <li
              v-for="c in changes.data.value ?? []"
              :key="c.path"
              class="flex flex-col gap-1"
              data-testid="ade-details-disk-file"
            >
              <div class="flex items-center gap-2">
                <code class="min-w-0 flex-1 truncate">{{ c.path }}</code>
                <span class="text-muted-foreground">{{ stateLabel(c) }}</span>
                <span v-if="c.pending" class="text-muted-foreground">{{
                  t('space.ade.details.diskPending.label')
                }}</span>
                <Button
                  variant="dialog"
                  size="kira"
                  :disabled="locked(c, true)"
                  data-testid="ade-details-disk-accept"
                  @click="resolve('accept', [c.path])"
                >
                  {{ t('space.ade.details.diskAccept.label') }}
                </Button>
                <Button
                  variant="dialog-danger"
                  size="kira"
                  :disabled="locked(c)"
                  data-testid="ade-details-disk-discard"
                  @click="resolve('discard', [c.path])"
                >
                  {{ t('common.action.discard.label') }}
                </Button>
              </div>
              <pre
                v-if="c.diff"
                class="max-h-48 overflow-auto rounded-kira-sm bg-field p-2 font-mono text-kira-sm"
              >{{ c.diff }}</pre>
            </li>
          </ul>
        </DialogBody>
        <DialogFooter>
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="ade-details-disk-discard-all"
            @click="resolve('discard', [])"
          >
            {{ t('space.ade.details.diskDiscardAll.label') }}
          </Button>
          <Button
            variant="dialog-primary"
            size="kira-lg"
            data-testid="ade-details-disk-accept-all"
            @click="resolve('accept', [])"
          >
            {{ t('space.ade.details.diskAcceptAll.label') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
