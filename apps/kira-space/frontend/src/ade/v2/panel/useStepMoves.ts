import { t } from '@theme/i18n';
import { computed, type MaybeRefOrGetter, toValue } from 'vue';
import { stepMoves } from '../board/stepMoves';
import type { CardModel } from '../plan/usePlanModel';
import { useAdeStageDialogStore } from '../state/adeStageDialog';

// Step moves of one task: every step of its workflow or Done, either direction. A move opens the
// stage dialog, where the handover notes go.
export function useStepMoves(card: MaybeRefOrGetter<CardModel>) {
  const stage = useAdeStageDialogStore();
  const moves = computed(() => {
    const c = toValue(card);
    return stepMoves(
      c.blocks,
      c.progress.finished ? 'done' : (c.progress.step?.id ?? c.task.entryStepId),
      c.task.runs,
    );
  });
  const options = computed(() => moves.value.options);
  const at = computed(() => moves.value.at);
  const prev = computed(() => moves.value.prev);
  const next = computed(() => moves.value.next);
  const live = computed(() => moves.value.live);
  const reason = computed(() =>
    live.value ? t('space.ade.board.taskMenu.liveBlocked.tooltip') : '',
  );

  function move(toStepId: string): void {
    stage.open({ taskId: toValue(card).task.id, toStepId });
  }

  return { options, at, prev, next, live, reason, move };
}
