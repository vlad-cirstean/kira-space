import { hostWords, t } from '@theme/i18n';
import type { ReviewWindowTarget } from '../wire';
import { reviewGate, useChangeRequestReview } from './useChangeRequestReview';

export interface HostCommentTarget {
  number: number;
  /** The host's short change request word, e.g. "PR". */
  short: string;
  /** Why a post is refused right now; '' when it can go. */
  blocked: string;
}

/**
 * The change request a line comment on a committed tab of the window's branch posts to. Shares the
 * header's `changeRequest.reviewState` query, so it adds no host call. `null` while the branch has no
 * readable change request.
 */
export function useHostLineComment(
  target: () => ReviewWindowTarget,
  branchTip: string,
): () => HostCommentTarget | null {
  const { state } = useChangeRequestReview(target);
  return () => {
    const s = state.data.value;
    if (s?.kind !== 'ok') return null;
    const short = hostWords(s.host).changeRequestShort;
    const gate = reviewGate(s, 'comment');
    let blocked = gate.reason;
    if (blocked === '' && s.headSha !== branchTip) {
      blocked = t('git.review.comments.host.headMoved.error', { short });
    }
    return { number: s.number, short, blocked };
  };
}
