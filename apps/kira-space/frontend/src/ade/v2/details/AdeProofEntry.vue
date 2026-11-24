<script setup lang="ts">
import type { ProofView } from '@shared/domain/proof';
import { useMutation } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@theme/components/ui/collapsible';
import { toast } from '@theme/components/ui/sonner';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { errorText } from '@workbench/bridge/codedError';
import RelativeTime from '@workbench/components/RelativeTime.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed } from 'vue';
import { control as studioControl } from '../../../studio/bridge/control';
import { proofViewIcon } from '../../../studio/proof/viewIcon';
import { useDeleteProof } from '../queries';
import { useAdeDetailsListStore } from '../state/adeDetailsList';
import type { TaskProof } from '../wire';
import AdeOriginSession from './AdeOriginSession.vue';
import AdeProofDiagramStep from './AdeProofDiagramStep.vue';

// One proof of the task chain (a folder `proofs/<slug>/`): header and summary always show; the steps
// show once expanded, diagram steps drawn inline, the rest open the proof window.
const props = defineProps<{ proof: TaskProof; taskId: string; readonly?: boolean }>();
const emit = defineEmits<{ openSession: [sessionId: string] }>();
const open = useMutation({
  mutationFn: () => studioControl.proofOpenWindow(props.taskId, props.proof.slug),
  onError: (err) => toast.error(errorText(err)),
});
const list = useAdeDetailsListStore();
const expandKey = computed(() => `proof:${props.proof.proofId}`);
const expanded = computed({
  get: () => list.isExpanded(expandKey.value),
  set: (on: boolean) => list.setExpanded(expandKey.value, on),
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
  <Card data-kind="proof" :data-slug="proof.slug" data-testid="ade-proof-entry">
    <Collapsible v-model:open="expanded" class="flex flex-col gap-1">
      <div class="flex items-center gap-2 text-kira-sm">
        <CollapsibleTrigger
          class="flex size-3.5 shrink-0 items-center justify-center text-muted-foreground"
          :aria-label="
            expanded ? t('common.action.collapse.label') : t('common.action.expand.label')
          "
          data-testid="ade-proof-toggle"
        >
          <CodiconIcon :name="expanded ? 'chevron-down' : 'chevron-right'" :size="13" />
        </CollapsibleTrigger>
        <CodiconIcon name="beaker" :size="12" />
        <TextTooltip :text="t('space.ade.proof.entry.open.tooltip')">
          <Button
            variant="link"
            size="inline"
            class="h-auto min-w-0 whitespace-normal break-words text-left font-medium"
            :pending="open.isPending.value"
            data-testid="ade-proof-open"
            @click="open.mutate()"
          >
            <span data-testid="ade-proof-name">{{ proof.name }}</span>
          </Button>
        </TextTooltip>
        <span class="select-text text-subtle" data-testid="ade-proof-path">{{ folderPath }}</span>
        <Badge v-if="proof.pending" variant="warn" data-testid="ade-proof-pending">{{
          t('space.ade.proof.entry.pending.status')
        }}</Badge>
        <span class="ml-auto text-subtle"><RelativeTime :at="proof.closedAt" /></span>
        <AdeOriginSession
          :task-id="taskId"
          :session-id="proof.sessionId"
          :archived="readonly"
          @open="emit('openSession', $event)"
        />
        <TooltipIconButton
          v-if="!readonly"
          :icon="ICON.delete"
          :label="t('studio.proof.delete.tooltip')"
          data-testid="ade-proof-remove"
          @click="askDelete()"
        />
      </div>
      <p v-if="proof.summary" class="select-text whitespace-pre-wrap text-kira-md">{{
        proof.summary
      }}</p>
      <CollapsibleContent>
        <ol class="m-0 flex list-none flex-col gap-1 p-0" :aria-label="t('studio.proof.tabs.aria')">
          <li v-for="s in proof.steps" :key="s.id" :data-view="s.view" data-testid="ade-proof-step">
            <AdeProofDiagramStep
              v-if="s.view === 'diagram'"
              :proof-id="proof.proofId"
              :rev="proof.rev"
              :ordinal="s.ordinal"
              :description="s.description"
            />
            <div v-else class="flex items-start gap-1.5 text-kira-md">
              <CodiconIcon class="mt-0.5" :name="proofViewIcon(s.view as ProofView)" :size="13" />
              <span class="text-subtle">#{{ s.ordinal }}</span>
              <span class="min-w-0 flex-1 select-text break-words">{{ s.description }}</span>
              <span v-if="s.source" class="select-text text-subtle">{{ s.source }}</span>
              <TooltipIconButton
                icon="link-external"
                :label="t('space.ade.proof.entry.open.tooltip')"
                :disabled="open.isPending.value"
                data-testid="ade-proof-step-open"
                @click="open.mutate()"
              />
            </div>
          </li>
        </ol>
      </CollapsibleContent>
    </Collapsible>
  </Card>
</template>
