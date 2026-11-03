import type { ReviewComment, ReviewCommentState } from '@kira/git-ipc';
import { useMutation, useQuery } from '@tanstack/vue-query';
import { tryOnScopeDispose } from '@vueuse/core';
import { queryClient } from '@workbench/state/queryClient';
import { computed } from 'vue';
import { onReviewRepaint } from '../../../repo/git/reviewRepaint';
import { gitTransportFor } from '../../../repo/git/transport';
import { ensureRepoOpen } from '../../../state/repoOpenHold';
import type { ReviewWindowTarget } from '../wire';

const commentsKey = (gitRepoId: string, branch: string) =>
  ['reviewComments', gitRepoId, branch] as const;

/** The branch's review comments over the repo's git transport; a comment repaint refetches them. */
export function useReviewComments(target: () => ReviewWindowTarget) {
  const key = computed(() => commentsKey(target().gitRepoId, target().branch));
  const query = useQuery({
    queryKey: key,
    queryFn: async (): Promise<readonly ReviewComment[]> => {
      const t = target();
      const transport = gitTransportFor(t.codeRepoId);
      await ensureRepoOpen(transport, t.gitRepoId);
      const res = await transport.request('review.comment.list', {
        repoId: t.gitRepoId,
        branch: t.branch,
      });
      return res.comments;
    },
  });
  tryOnScopeDispose(
    onReviewRepaint((e) => {
      if (
        e.kind === 'comments' &&
        e.repoId === target().gitRepoId &&
        e.branch === target().branch
      ) {
        void queryClient.invalidateQueries({ queryKey: key.value });
      }
    }),
  );
  const setState = useMutation({
    mutationFn: async (a: { id: number; state: ReviewCommentState }) => {
      const t = target();
      await gitTransportFor(t.codeRepoId).request('review.comment.setState', {
        repoId: t.gitRepoId,
        branch: t.branch,
        id: a.id,
        state: a.state,
      });
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: key.value }),
  });
  return { query, setState, refresh: () => queryClient.invalidateQueries({ queryKey: key.value }) };
}
