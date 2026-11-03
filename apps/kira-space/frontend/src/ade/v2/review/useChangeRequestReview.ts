import type { ChangeRequestReviewEvent, ChangeRequestReviewState } from '@kira/git-ipc';
import { useMutation, useQuery } from '@tanstack/vue-query';
import { hostWords, t } from '@theme/i18n';
import { computed } from 'vue';
import { control } from '../../../bridge/control';
import { gitCall } from '../../../repo/git/gitCall';
import { useGitRepoHold } from '../../../repo/git/repoHolds';
import type { ReviewWindowTarget } from '../wire';

type OkState = Extract<ChangeRequestReviewState, { kind: 'ok' }>;

const reviewStateKey = (gitRepoId: string, branch: string, number: number) =>
  ['changeRequestReviewState', gitRepoId, branch, number] as const;

/** The local branch tip differs from the head the host reviews. False when the branch is gone locally. */
export function headStale(s: OkState): boolean {
  return s.localHeadSha !== '' && s.localHeadSha !== s.headSha;
}

export interface ReviewGate {
  enabled: boolean;
  reason: string;
}

/** Whether `event` can be submitted on the change request in `s`, and why not. */
export function reviewGate(s: OkState, event: ChangeRequestReviewEvent): ReviewGate {
  const words = hostWords(s.host);
  const vars = { short: words.changeRequestShort };
  if (s.state === 'merged')
    return { enabled: false, reason: t('space.ade.review.actions.merged.hint', vars) };
  if (s.state === 'closed')
    return { enabled: false, reason: t('space.ade.review.actions.closed.hint', vars) };
  if (s.own && event !== 'comment') {
    return { enabled: false, reason: t('space.ade.review.actions.own.hint', vars) };
  }
  return { enabled: true, reason: '' };
}

/** The review facts of the window's change request, the review and comment posts, and the open link. */
export function useChangeRequestReview(target: () => ReviewWindowTarget) {
  const key = computed(() =>
    reviewStateKey(target().gitRepoId, target().branch, target().changeRequest),
  );
  const hold = useGitRepoHold(() => target().gitRepoId);
  const state = useQuery({
    queryKey: key,
    queryFn: async (): Promise<ChangeRequestReviewState> => {
      const tg = target();
      await hold.ready();
      return gitCall('changeRequest.reviewState', {
        repoId: tg.gitRepoId,
        branch: tg.branch,
        ...(tg.changeRequest > 0 ? { number: tg.changeRequest } : {}),
      });
    },
    staleTime: 60_000,
    refetchOnWindowFocus: true,
  });
  const submitReview = useMutation({
    mutationFn: async (a: {
      number: number;
      event: ChangeRequestReviewEvent;
      body: string;
      headSha: string;
    }) => {
      await hold.ready();
      return gitCall('changeRequest.submitReview', { repoId: target().gitRepoId, ...a });
    },
  });
  const comment = useMutation({
    mutationFn: async (a: { number: number; body: string }) => {
      await hold.ready();
      return gitCall('changeRequest.comment', { repoId: target().gitRepoId, ...a });
    },
  });
  async function openChangeRequest(number: number): Promise<void> {
    await hold.ready();
    const res = await gitCall('changeRequest.browserUrl', { repoId: target().gitRepoId, number });
    if (res.url !== null) await control.gitHostOpenChangeRequestUrl(res.url);
  }
  return { state, submitReview, comment, openChangeRequest };
}
