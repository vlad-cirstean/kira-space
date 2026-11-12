import { useMutationState } from '@tanstack/vue-query';
import { createSharedComposable } from '@vueuse/core';
import { computed } from 'vue';
import {
  openReviewWindowKey,
  useApprove,
  useOpenReviewWindow,
  useResumeRun,
  useRetryRun,
  useSetPlan,
  useShelveTask,
  useStartRun,
  useStopRun,
  useUpdateTask,
} from '../queries';

/**
 * The task writes every card, menu and panel button shares: one mutation observer each instead of
 * about fifteen per card. A write's own pending state lives in `adeOps` (per task, shown
 * everywhere); only the review window, which has no op, reads pending from the mutation cache.
 */
function buildTaskWrites() {
  const setPlan = useSetPlan();
  const shelve = useShelveTask();
  const stopRun = useStopRun();
  const update = useUpdateTask();
  const start = useStartRun();
  const approve = useApprove();
  const retry = useRetryRun();
  const resume = useResumeRun();
  const openReview = useOpenReviewWindow();
  const reviewVariables = useMutationState({
    filters: { mutationKey: openReviewWindowKey, status: 'pending' },
    select: (m) => (m.state.variables as { branchId: string } | undefined)?.branchId ?? '',
  });
  /** Branches whose review window is opening now. */
  const reviewPending = computed(() => new Set(reviewVariables.value));
  return {
    setPlan,
    shelve,
    stopRun,
    update,
    start,
    approve,
    retry,
    resume,
    openReview,
    reviewPending,
  };
}

export const useTaskWrites = createSharedComposable(buildTaskWrites);
