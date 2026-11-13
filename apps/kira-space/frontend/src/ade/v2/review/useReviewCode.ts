import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { nextTick, watch } from 'vue';
import { type ReviewChoice, reviewChoice, reviewChoices } from '../board/reviewCode';
import { type BranchRowModel, type CardModel, usePlanModel } from '../plan/usePlanModel';
import { useTaskWrites } from '../plan/useTaskWrites';
import { useAdeBoardUiStore } from '../state/adeBoardUi';

interface OpenOpts {
  /** Replaces the default, which shows the message in the task panel. */
  onError?(message: string): void;
}

function refocus(el: HTMLElement | null): void {
  if (el?.isConnected) el.focus({ preventScroll: true });
}

/** Opens a branch's review window. Go focuses an already open one. Focus returns to the invoker. */
export function useReviewCode() {
  const ui = useAdeBoardUiStore();
  const contextMenu = useContextMenuStore();
  const { view } = usePlanModel();
  const { openReview: mutation, reviewPending } = useTaskWrites();

  function open(
    taskId: string,
    branchId: string,
    returnTo: HTMLElement | null,
    opts?: OpenOpts,
  ): void {
    delete ui.actionError[taskId];
    mutation.mutate(
      { branchId },
      {
        onError: (err) => {
          const message = errorText(err);
          if (opts?.onError) return opts.onError(message);
          if (ui.selectedTaskId !== taskId) ui.select(taskId);
          ui.actionError[taskId] = message;
        },
        onSettled: () => refocus(returnTo),
      },
    );
  }

  /** One branch opens it; several show a picker under `anchor`. */
  function openTask(card: CardModel, anchor: HTMLElement | null): void {
    const graph = view.value?.graph;
    if (!graph) return;
    const choices = reviewChoices(card, graph);
    const taskId = card.task.id;
    if (choices.length === 0) return;
    if (choices.length === 1) {
      if (!choices[0].disabled) open(taskId, choices[0].branchId, anchor);
      return;
    }
    const items: MenuItem[] = [
      { type: 'label', label: t('space.ade.board.reviewChoice.pick.tooltip') },
      ...choices.map(
        (c): MenuItem => ({
          type: 'item',
          id: `ade-review-pick-${c.branchId}`,
          label: c.label,
          disabled: c.disabled,
          hint: c.tip,
          run: () => open(taskId, c.branchId, anchor),
        }),
      ),
    ];
    const r = anchor?.getBoundingClientRect();
    contextMenu.openContextMenuAt(r?.left ?? 0, r?.bottom ?? 0, items);
    watch(
      () => contextMenu.open,
      async () => {
        await nextTick();
        refocus(anchor);
      },
      { once: true },
    );
  }

  const choicesOf = (card: CardModel): ReviewChoice[] => {
    const graph = view.value?.graph;
    return graph ? reviewChoices(card, graph) : [];
  };
  const choiceOf = (row: BranchRowModel): ReviewChoice | null => {
    const graph = view.value?.graph;
    const task = graph?.byTask.get(row.branch.taskId);
    return graph && task ? reviewChoice(row, task, graph) : null;
  };

  /** Whether the review window of any of these branches is opening. */
  const pendingFor = (branchIds: readonly string[]): boolean =>
    branchIds.some((id) => reviewPending.value.has(id));
  return { open, openTask, choicesOf, choiceOf, pendingFor };
}
