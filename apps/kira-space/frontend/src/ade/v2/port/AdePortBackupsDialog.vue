<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
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
import { formatBytes, formatDateTime } from '@workbench/behaviour/format';
import { errorText } from '@workbench/bridge/codedError';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref, watch } from 'vue';
import AdePortResult from './AdePortResult.vue';
import { usePortBackups, useRestore } from './queries';
import type { Backup, PortProvider, PortResult } from './types';

// The backups an export took beside the provider's files, newest first, and the undo: Restore writes
// one back after a backup of the file it replaces. ~/.claude.json lists but is restored by hand.
const props = defineProps<{ provider: PortProvider }>();
const open = defineModel<boolean>('open', { required: true });

const backups = usePortBackups(
  () => props.provider.id,
  () => open.value,
);
const restoreMutation = useRestore();
const confirmDialogStore = useConfirmDialogStore();

const result = ref<PortResult | null>(null);
const error = ref('');
const rows = computed(() => backups.data.value ?? []);

watch(open, (isOpen) => {
  if (!isOpen) return;
  result.value = null;
  error.value = '';
});

async function restore(b: Backup): Promise<void> {
  error.value = '';
  const ok = await confirmDialogStore.confirm({
    title: t('space.ade.port.backupList.confirm.title'),
    message: t('space.ade.port.backupList.confirm.body', { file: b.file, target: b.target }),
    confirmLabel: t('common.action.restore.label'),
    tone: 'danger',
  });
  if (!ok) return;
  try {
    result.value = await restoreMutation.mutateAsync({ provider: props.provider.id, file: b.file });
  } catch (err) {
    error.value = errorText(err);
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent size="2xl" fixed-height data-testid="ade-port-backups-dialog">
      <DialogHeader>
        <DialogTitle>{{
          t('space.ade.port.backupList.title', { agent: provider.label })
        }}</DialogTitle>
        <DialogDescription>{{ t('space.ade.port.backupList.hint') }}</DialogDescription>
      </DialogHeader>
      <DialogBody>
        <Alert v-if="error || backups.error.value" variant="error" data-testid="ade-port-error">
          <AlertDescription>{{ error || errorText(backups.error.value) }}</AlertDescription>
        </Alert>
        <AdePortResult v-if="result" :result="result" />
        <ul
          v-if="rows.length"
          class="m-0 flex list-none flex-col gap-2 p-0"
          data-testid="ade-port-backup-list"
        >
          <li
            v-for="b in rows"
            :key="b.file"
            class="flex flex-col gap-1"
            :data-testid="`ade-port-backup-${b.file}`"
            :data-role="b.role"
          >
            <div class="flex items-center gap-2">
              <span class="min-w-0 flex-1 truncate font-data text-kira-md">{{ b.file }}</span>
              <Badge>{{ b.target }}</Badge>
              <span class="shrink-0 text-kira-sm text-subtle">{{ formatDateTime(b.taken) }}</span>
              <span class="shrink-0 text-kira-sm text-subtle">{{ formatBytes(b.size) }}</span>
              <Button
                variant="dialog"
                size="kira-lg"
                :disabled="!b.restorable"
                :pending="restoreMutation.isPending.value"
                :data-testid="`ade-port-restore-${b.file}`"
                @click="restore(b)"
              >
                {{ t('common.action.restore.label') }}
              </Button>
            </div>
            <FieldDescription
              v-if="!b.restorable"
              :data-testid="`ade-port-backup-reason-${b.file}`"
            >
              {{ b.reason }}
            </FieldDescription>
          </li>
        </ul>
        <EmptyState
          v-else-if="!backups.isPending.value"
          icon="history"
          :title="t('space.ade.port.backupList.empty.title')"
          data-testid="ade-port-backups-empty"
        />
      </DialogBody>
      <DialogFooter>
        <Button variant="dialog" size="kira-lg" data-testid="ade-port-close" @click="open = false">
          {{ t('common.action.close.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
