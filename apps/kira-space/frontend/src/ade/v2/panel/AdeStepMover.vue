<script setup lang="ts">
import PickerTrigger from '@theme/components/PickerTrigger.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { computed } from 'vue';
import { stepMoves } from '../board/stepMoves';
import type { CardModel } from '../plan/usePlanModel';
import { useAdeStageDialogStore } from '../state/adeStageDialog';
import AdeRow from './AdeRow.vue';

// Moves the task to any step of its workflow, or to Done, in either direction: each move opens the
// stage dialog, where the handover notes go.
const props = defineProps<{ card: CardModel }>();
const stage = useAdeStageDialogStore();

const moves = computed(() =>
  stepMoves(
    props.card.blocks,
    props.card.progress.finished
      ? 'done'
      : (props.card.progress.step?.id ?? props.card.task.entryStepId),
    props.card.task.runs,
  ),
);
const options = computed(() => moves.value.options);
const at = computed(() => moves.value.at);
const prev = computed(() => moves.value.prev);
const next = computed(() => moves.value.next);
const live = computed(() => moves.value.live);
const reason = computed(() =>
  live.value ? t('space.ade.board.taskMenu.liveBlocked.tooltip') : '',
);

function move(toStepId: string): void {
  stage.open({ taskId: props.card.task.id, toStepId });
}
</script>

<template>
  <AdeRow
    v-if="card.blocks.length"
    :label="t('space.common.step.label')"
    data-testid="ade-step-mover"
  >
    <TooltipIconButton
      :icon="ICON.back"
      :label="reason || t('space.ade.stepMover.back.tooltip')"
      :aria-label="t('space.ade.stepMover.back.tooltip')"
      :disabled="live || !prev"
      disabled-trigger
      data-testid="ade-step-back"
      @click="move(prev?.id ?? '')"
    />
    <DropdownMenu>
      <DropdownMenuTrigger as-child>
        <PickerTrigger
          size="kira"
          :label="options[at]?.name"
          :disabled="live"
          data-testid="ade-step-pick"
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" data-testid="ade-step-menu">
        <DropdownMenuItem
          v-for="o in options"
          :key="o.id"
          :data-testid="`ade-step-option-${o.id}`"
          :disabled="o.skipped"
          @select="move(o.id)"
        >
          {{ o.name
          }}<span v-if="o.skipped" class="text-muted-foreground">
            {{ t('common.skipped.status') }}</span
          >
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
    <TooltipIconButton
      icon="arrow-right"
      :label="reason || t('space.ade.stepMover.next.tooltip')"
      :aria-label="t('space.ade.stepMover.next.tooltip')"
      :disabled="live || !next"
      disabled-trigger
      data-testid="ade-step-next"
      @click="move(next?.id ?? '')"
    />
  </AdeRow>
</template>
