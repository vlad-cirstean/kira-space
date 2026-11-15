<script setup lang="ts">
import type { ProofView } from '@shared/domain/proof';
import { useMutation } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { toast } from '@theme/components/ui/sonner';
import { t } from '@theme/i18n';
import { formatRelative } from '@workbench/behaviour/format';
import { errorText } from '@workbench/bridge/codedError';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed } from 'vue';
import { control as studioControl } from '../../../studio/bridge/control';
import { proofViewIcon } from '../../../studio/proof/viewIcon';
import AdeChip from '../AdeChip.vue';
import { useDeleteProof } from '../queries';
import type { TaskProof } from '../wire';
import AdeOriginSession from './AdeOriginSession.vue';
import AdeProofDiagramStep from './AdeProofDiagramStep.vue';

// One proof of the task chain (a folder `proofs/<slug>/`): its diagram steps draw inline, the rest
// open the proof window.
const props = defineProps<{ proof: TaskProof; taskId: string; readonly?: boolean }>();
const emit = defineEmits<{ openSession: [sessionId: string] }>();
const open = useMutation({
  mutationFn: () => studioControl.proofOpenWindow(props.taskId, props.proof.slug),
  onError: (err) => toast.error(errorText(err)),
});
const folderPath = computed(() => `proofs/${props.proof.slug}`);
const remove = useDeleteProof();
const confirmDialog = useConfirmDialogStore();

async function askDelete(): Promise<void> {
  await confirmDialog.confirm({
    title: t('studio.proof.delete.title'),
    message: t('studio.proof.delete.body', { name: props.proof.name }),
    confirmLabel: t('common.action.delete.label'),
    tone: 'danger',
    run: () => remove.mutateAsync(props.proof.proofId),
  });
}
</script>

<template>
  <Card
    class="flex flex-col gap-2 p-2 text-kira-md"
    data-kind="proof"
    :data-slug="proof.slug"
    data-testid="ade-proof-entry"
  >
    <div class="flex items-center gap-2 text-kira-sm">
      <CodiconIcon name="beaker" :size="12" />
      <Button
        variant="link"
        size="inline"
        class="min-w-0 font-medium"
        :title="t('space.ade.proof.entry.open.tooltip')"
        :pending="open.isPending.value"
        data-testid="ade-proof-open"
        @click="open.mutate()"
        ><span class="truncate">{{ proof.name }}</span></Button
      >
      <span class="text-subtle" data-testid="ade-proof-path">{{ folderPath }}</span>
      <AdeChip
        v-if="proof.pending"
        :label="t('space.ade.proof.entry.pending.status')"
        tone="amber"
        data-testid="ade-proof-pending"
      />
      <span class="ml-auto text-subtle">{{ formatRelative(proof.closedAt) }}</span>
      <AdeOriginSession
        :task-id="taskId"
        :session-id="proof.sessionId"
        :archived="readonly"
        @open="emit('openSession', $event)"
      />
      <TooltipIconButton
        v-if="!readonly"
        icon="close"
        :label="t('space.ade.proof.entry.delete.tooltip')"
        data-testid="ade-proof-remove"
        @click="askDelete()"
      />
    </div>
    <p v-if="proof.summary" class="whitespace-pre-wrap text-kira-md">{{ proof.summary }}</p>
    <ol class="m-0 flex list-none flex-col gap-1 p-0" :aria-label="t('studio.proof.tabs.aria')">
      <li v-for="s in proof.steps" :key="s.id" :data-view="s.view" data-testid="ade-proof-step">
        <AdeProofDiagramStep
          v-if="s.view === 'diagram'"
          :proof-id="proof.proofId"
          :rev="proof.rev"
          :ordinal="s.ordinal"
          :description="s.description"
        />
        <Button
          v-else
          variant="toolbar"
          size="kira"
          class="w-full justify-start font-normal"
          :pending="open.isPending.value"
          data-testid="ade-proof-step-open"
          @click="open.mutate()"
        >
          <CodiconIcon :name="proofViewIcon(s.view as ProofView)" :size="12" />
          <span class="text-subtle">#{{ s.ordinal }}</span>
          <span class="truncate">{{ s.description }}</span>
          <span v-if="s.source" class="ml-auto text-subtle">{{ s.source }}</span>
        </Button>
      </li>
    </ol>
  </Card>
</template>
