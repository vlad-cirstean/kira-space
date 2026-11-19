import { type MessageKey, t } from '@theme/i18n';
import type { ReviewPhase } from './wire';

const RUN_STATE = {
  done: 'common.done.status',
  failed: 'common.state.failed.status',
  stuck: 'common.needsYou.status',
  back: 'space.ade.outcome.back.status',
  running: 'common.state.running.status',
  review: 'space.ade.board.branchTag.review.label',
  pending: 'common.state.pending.status',
  skipped: 'common.skipped.status',
} as const satisfies Record<string, MessageKey>;

/** A run's state as the user reads it; an unknown state shows as sent. */
export function runStateLabel(state: string): string {
  return state in RUN_STATE ? t(RUN_STATE[state as keyof typeof RUN_STATE]) : state;
}

const REVIEW_PHASE = {
  waiting: 'space.ade.reviewStep.waiting.status',
  sent: 'space.ade.reviewStep.sent.status',
  working: 'space.ade.reviewStep.working.status',
  stuck: 'space.ade.reviewStep.stuck.status',
  done: 'common.done.status',
} as const satisfies Record<ReviewPhase, MessageKey>;

/** Where a review step stands, as the run line and Needs-you read it. */
export function reviewPhaseLabel(phase: ReviewPhase): string {
  return t(REVIEW_PHASE[phase]);
}
