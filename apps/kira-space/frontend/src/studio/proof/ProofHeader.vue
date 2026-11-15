<script setup lang="ts">
import type { ProofDetail } from '@shared/domain/proof';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { formatDateTime } from '@theme/format';
import { t } from '@theme/i18n';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import TitleBarBase from '@workbench/components/TitleBar.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed } from 'vue';
import { control } from '../bridge/control';
import { useDeleteProof } from './queries';

const props = defineProps<{ detail: ProofDetail; steps: number }>();
const folderPath = computed(() => `proofs/${props.detail.slug}`);
const remove = useDeleteProof();
const confirmDialog = useConfirmDialogStore();

async function askDelete(): Promise<void> {
  await confirmDialog.confirm({
    title: t('studio.proof.delete.title'),
    message: t('studio.proof.delete.body', { name: props.detail.name }),
    confirmLabel: t('common.action.delete.label'),
    tone: 'danger',
    run: () => remove.mutateAsync(props.detail.id),
  });
}
</script>

<template>
  <TitleBarBase>
    <div class="flex min-w-0 flex-1 items-center gap-2" data-testid="proof-header">
      <span class="min-w-0 truncate text-kira-md font-medium" data-testid="proof-title">{{
        detail.name
      }}</span>
      <span class="shrink-0 text-kira-sm text-muted-foreground" data-testid="proof-steps">{{
        t('studio.proof.session.steps.label', { count: steps })
      }}</span>
      <span class="shrink-0 text-kira-sm text-muted-foreground">{{
        formatDateTime(detail.closedAt)
      }}</span>
      <Badge v-if="detail.pending" variant="warn" class="shrink-0" data-testid="proof-pending">{{
        t('studio.proof.pending.label')
      }}</Badge>
      <span
        class="min-w-0 truncate font-mono text-kira-sm text-muted-foreground"
        :aria-label="t('studio.proof.folder.path.aria')"
        data-testid="proof-path"
        >{{
          folderPath
        }}</span
      >
      <div class="wails-no-drag ml-auto flex shrink-0 items-center gap-1.5">
        <TooltipIconButton
          icon="folder-opened"
          :label="t('studio.proof.folder.open.label')"
          data-testid="proof-folder-open"
          @click="fireAndForget(control.proofOpenFolder(detail.id), 'proof.openFolder')"
        />
        <TooltipIconButton
          icon="folder"
          :label="t('studio.proof.folder.reveal.label')"
          data-testid="proof-folder-reveal"
          @click="fireAndForget(control.proofRevealFolder(detail.id), 'proof.revealFolder')"
        />
        <TooltipIconButton
          icon="trash"
          :label="t('space.ade.proof.entry.delete.tooltip')"
          data-testid="proof-delete"
          @click="askDelete()"
        />
        <slot />
      </div>
    </div>
  </TitleBarBase>
</template>
