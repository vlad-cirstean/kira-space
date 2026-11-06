<script setup lang="ts">
import type { ProofView } from '@shared/domain/proof';
import { useMutation } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { t } from '@theme/i18n';
import { formatRelative } from '@workbench/behaviour/format';
import { control as studioControl } from '../../../studio/bridge/control';
import { proofViewIcon } from '../../../studio/proof/viewIcon';
import AdeChip from '../AdeChip.vue';
import { useUnlinkProof } from '../queries';
import type { TaskProof } from '../wire';
import AdeOriginSession from './AdeOriginSession.vue';
import AdeProofDiagramStep from './AdeProofDiagramStep.vue';

// One attached proof session of the task chain: its diagram steps draw inline, the rest open the
// proof window.
const props = defineProps<{ proof: TaskProof; taskId: string; readonly?: boolean }>();
const emit = defineEmits<{ openSession: [sessionId: string] }>();
const open = useMutation({
  mutationFn: () => studioControl.proofOpenWindow(props.proof.proofId),
});
const unlink = useUnlinkProof();

function stage(p: TaskProof): string {
  const name = p.stepName || p.stepId;
  return name ? t('space.ade.notes.entry.during.label', { stage: name }) : '';
}
</script>

<template>
  <Card
    class="flex flex-col gap-2 p-2 text-kira-md"
    data-kind="proof"
    :data-author="proof.source"
    data-testid="ade-proof-entry"
  >
    <div class="flex items-center gap-2 text-kira-sm">
      <CodiconIcon name="beaker" :size="12" />
      <Button
        variant="link"
        size="inline"
        class="min-w-0 font-medium"
        :title="t('space.ade.proof.entry.open.tooltip')"
        data-testid="ade-proof-open"
        @click="open.mutate()"
        ><span class="truncate">{{ proof.name }}</span></Button
      >
      <span
        class="inline-flex items-center gap-1"
        role="img"
        :aria-label="
          t(
            proof.source === 'user'
              ? 'space.ade.notes.author.user.aria'
              : 'space.ade.notes.author.agent.aria',
          )
        "
        data-testid="ade-note-author"
      >
        <CodiconIcon :name="proof.source === 'user' ? 'account' : 'hubot'" :size="12" />
        <AdeChip
          :label="
            proof.source === 'user'
              ? t('common.picker.you.label')
              : t('memory.common.origin.agent.label')
          "
          tone="grey"
        />
      </span>
      <span v-if="stage(proof)" class="text-muted-foreground" data-testid="ade-note-stage">{{
        stage(proof)
      }}</span>
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
        :label="t('space.ade.proof.entry.remove.tooltip')"
        data-testid="ade-proof-remove"
        @click="unlink.mutate({ taskId, proofId: proof.proofId })"
      />
    </div>
    <p v-if="proof.summary" class="whitespace-pre-wrap text-kira-md">{{ proof.summary }}</p>
    <ol class="m-0 flex list-none flex-col gap-1 p-0" :aria-label="t('studio.proof.tabs.aria')">
      <li
        v-for="s in proof.steps"
        :key="s.ordinal"
        :data-view="s.view"
        data-testid="ade-proof-step"
      >
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
