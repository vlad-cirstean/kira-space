import { useQueryClient } from '@tanstack/vue-query';
import { finishVars, runMessageVars } from '../board/runMessage';
import type { CardModel } from '../plan/usePlanModel';
import { adePromptPreviewOptions } from '../queries';

/** The variables of the three prompts the Run dialog previews, `null` while there is nothing to show. */
export interface RunPreviewVars {
  step: Record<string, unknown> | null;
  finish: Record<string, unknown> | null;
  space: Record<string, unknown> | null;
}

/** Registry prompt ids of the three previews. */
export const RUN_PREVIEW_IDS = {
  step: 'ade.step.prompt',
  finish: 'headless.finishStep.suffix',
  space: 'headless.space.suffix',
} as const;

/** What the Run dialog previews for `card`'s current step; the dialog and the hover prefetch share it. */
export function runPreviewVars(card: CardModel | null): RunPreviewVars {
  const step = card?.progress.step ?? null;
  if (!card || !step) return { step: null, finish: null, space: null };
  const def = card.workflow?.steps.find((x) => x.id === step.id) ?? null;
  // Position among the workflow's automatic steps, as `ade.step.prompt` counts them.
  const auto = (card.workflow?.steps ?? []).filter((x) => x.kind !== 'manual' && !x.skip);
  return {
    step: runMessageVars({
      title: card.title,
      jira: card.task.jira,
      step: { name: step.name, prompt: def?.prompt ?? '', results: step.results },
      stepNumber: auto.findIndex((x) => x.id === step.id) + 1,
      stepCount: auto.length,
      noRepo: !card.rows.some((r) => r.branch.kind === 'mine'),
    }),
    finish: finishVars(step.results),
    space: {},
  };
}

/** Warms the Run dialog's three previews, so the message is there when the dialog opens. */
export function usePrefetchRunPreview(): (card: CardModel) => void {
  const queryClient = useQueryClient();
  return (card) => {
    const vars = runPreviewVars(card);
    for (const key of ['step', 'finish', 'space'] as const) {
      if (vars[key] === null) continue;
      void queryClient.prefetchQuery(adePromptPreviewOptions(RUN_PREVIEW_IDS[key], vars[key]));
    }
  };
}
