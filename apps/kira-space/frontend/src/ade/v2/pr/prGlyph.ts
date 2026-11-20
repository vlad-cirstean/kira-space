import { type MessageKey, t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import type { StatusTone } from '@theme/lib/statusTone';
import type { Tone } from '../board/actions';

// The one table of pull request status. A glyph is the state (shape), the checks (corner dot) and the
// tone that ranks conflicts, review, auto-merge. Branch row, task panel, host view and phone read it.

/** What any change request carries: the board wire and the host view detail both fit. */
export interface PrStatus {
  state: string;
  checksState?: string;
  reviewDecision?: string;
  mergeable?: string;
  autoMerge?: boolean;
}

type PrState = 'open' | 'draft' | 'merged' | 'closed';

export const PR_STATE_GLYPH: Record<PrState, { icon: string; tone: Tone; label: MessageKey }> = {
  open: { icon: 'git-pull-request', tone: 'blue', label: 'common.action.open.label' },
  draft: { icon: 'git-pull-request-draft', tone: 'grey', label: 'common.draft.status' },
  merged: { icon: 'git-merge', tone: 'purple', label: 'common.merged.status' },
  closed: { icon: 'git-pull-request-closed', tone: 'grey', label: 'common.closed.status' },
};

const isPrState = (state: string): state is PrState => state in PR_STATE_GLYPH;

/** The state's own word; an unknown state shows raw. */
export const prStateLabel = (state: string): string =>
  isPrState(state) ? t(PR_STATE_GLYPH[state].label) : state;

export type CheckKey =
  | 'success'
  | 'failure'
  | 'error'
  | 'pending'
  | 'expected'
  | 'skipped'
  | 'neutral';
type DotTone = 'ok' | 'error' | 'warn';

/**
 * One CI state. `icon`, `tone` and `label` draw a single check row; `rollup` and `dot` draw the
 * glyph's checks part (no `rollup` or `dot`: the state is not worth a badge).
 */
export const CHECK_GLYPH: Record<
  CheckKey,
  { icon: string; tone: StatusTone; label: MessageKey; rollup?: MessageKey; dot?: DotTone }
> = {
  success: {
    icon: ICON.success,
    tone: 'ok',
    label: 'space.common.changeRequest.check.passed.status',
    rollup: 'space.common.changeRequest.checks.passed.status',
    dot: 'ok',
  },
  failure: {
    icon: ICON.error,
    tone: 'error',
    label: 'common.state.failed.status',
    rollup: 'space.common.changeRequest.checks.failed.status',
    dot: 'error',
  },
  error: {
    icon: ICON.error,
    tone: 'error',
    label: 'space.common.changeRequest.check.errored.status',
    rollup: 'space.common.changeRequest.checks.failed.status',
    dot: 'error',
  },
  pending: {
    icon: 'clock',
    tone: 'warn',
    label: 'common.state.pending.status',
    rollup: 'space.common.changeRequest.checks.running.status',
    dot: 'warn',
  },
  expected: {
    icon: 'clock',
    tone: 'warn',
    label: 'common.state.pending.status',
    rollup: 'space.common.changeRequest.checks.expected.status',
    dot: 'warn',
  },
  skipped: { icon: 'debug-step-over', tone: 'neutral', label: 'common.skipped.status' },
  neutral: {
    icon: 'circle-outline',
    tone: 'neutral',
    label: 'space.common.changeRequest.check.neutral.status',
  },
};

/** The host's raw check state (any case); an unknown one is `null`. */
export function checkKey(raw: string | undefined): CheckKey | null {
  const key = (raw ?? '').toLowerCase();
  return key in CHECK_GLYPH ? (key as CheckKey) : null;
}

export type ReviewKey =
  | 'approved'
  | 'changesRequested'
  | 'reviewRequired'
  | 'rejected'
  | 'commented'
  | 'pending';

/** One review state, for a single reviewer or for the whole change request. */
export const REVIEW_GLYPH: Record<
  ReviewKey,
  { icon: string; tone: StatusTone; label: MessageKey }
> = {
  approved: {
    icon: ICON.success,
    tone: 'ok',
    label: 'space.common.changeRequest.review.approved.status',
  },
  changesRequested: {
    icon: 'request-changes',
    tone: 'warn',
    label: 'space.common.changeRequest.review.changesRequested.status',
  },
  reviewRequired: {
    icon: 'eye',
    tone: 'neutral',
    label: 'space.common.changeRequest.review.required.status',
  },
  rejected: {
    icon: ICON.error,
    tone: 'error',
    label: 'space.common.changeRequest.review.rejected.status',
  },
  commented: {
    icon: 'comment',
    tone: 'neutral',
    label: 'space.common.changeRequest.review.commented.status',
  },
  pending: {
    icon: 'clock',
    tone: 'neutral',
    label: 'space.common.changeRequest.review.pending.status',
  },
};

const REVIEW_DECISION: Record<string, ReviewKey> = {
  APPROVED: 'approved',
  CHANGES_REQUESTED: 'changesRequested',
  REVIEW_REQUIRED: 'reviewRequired',
};

/** The host's review rollup; `''` and unknown are `null`. */
export const reviewKey = (raw: string | undefined): ReviewKey | null =>
  REVIEW_DECISION[raw ?? ''] ?? null;

export type MergeableKey = 'mergeable' | 'conflicting';

export const MERGEABLE_GLYPH: Record<MergeableKey, { tone: StatusTone; label: MessageKey }> = {
  mergeable: { tone: 'ok', label: 'space.common.changeRequest.mergeable.clean.status' },
  conflicting: { tone: 'error', label: 'space.common.changeRequest.mergeable.conflicting.status' },
};

const isMergeable = (raw: string | undefined): raw is MergeableKey =>
  raw === 'mergeable' || raw === 'conflicting';

const AUTO_MERGE_LABEL: MessageKey = 'space.common.changeRequest.autoMerge.on.status';

export type PrPart = 'checks' | 'review' | 'mergeable' | 'autoMerge';

export interface PrGlyphModel {
  icon: string;
  tone: Tone;
  /** Open and draft only; `pending` pulses. */
  checks: { key: CheckKey; dot: DotTone; pending: boolean } | null;
  review: ReviewKey | null;
  mergeable: MergeableKey | null;
  autoMerge: boolean;
  /** Every reported part after the state, in tooltip order: checks, review, conflicts, auto-merge. */
  lines: { part: PrPart; value: string; label: MessageKey; tone: StatusTone }[];
}

/**
 * Shape is the state. Open and draft also show checks, review, conflicts and auto-merge: the shape's
 * tone is the first of conflicts (red), changes requested (amber), auto-merge (purple), approved
 * (green), else the state's own. Review required and comment counts are tooltip only.
 */
export function prGlyph(s: PrStatus): PrGlyphModel {
  const base = isPrState(s.state)
    ? PR_STATE_GLYPH[s.state]
    : { icon: 'git-pull-request', tone: 'blue' as Tone };
  const live = s.state === 'open' || s.state === 'draft';
  const out: PrGlyphModel = {
    icon: base.icon,
    tone: base.tone,
    checks: null,
    review: null,
    mergeable: null,
    autoMerge: false,
    lines: [],
  };
  if (!live) return out;

  const ck = checkKey(s.checksState);
  const g = ck ? CHECK_GLYPH[ck] : null;
  if (ck && g?.dot && g.rollup) {
    out.checks = { key: ck, dot: g.dot, pending: ck === 'pending' || ck === 'expected' };
    out.lines.push({ part: 'checks', value: ck, label: g.rollup, tone: g.tone });
  }
  out.review = reviewKey(s.reviewDecision);
  if (out.review) {
    const r = REVIEW_GLYPH[out.review];
    out.lines.push({ part: 'review', value: out.review, label: r.label, tone: r.tone });
  }
  if (isMergeable(s.mergeable)) {
    out.mergeable = s.mergeable;
    const m = MERGEABLE_GLYPH[s.mergeable];
    out.lines.push({ part: 'mergeable', value: s.mergeable, label: m.label, tone: m.tone });
  }
  if (s.autoMerge) {
    out.autoMerge = true;
    out.lines.push({ part: 'autoMerge', value: 'on', label: AUTO_MERGE_LABEL, tone: 'info' });
  }

  if (out.mergeable === 'conflicting') out.tone = 'red';
  else if (out.review === 'changesRequested') out.tone = 'amber';
  else if (out.autoMerge) out.tone = 'purple';
  else if (out.review === 'approved') out.tone = 'green';
  return out;
}

/** State and every status line as one sentence, for an accessible name. */
export function prStatusText(s: PrStatus): string {
  return prGlyph(s).lines.reduce(
    (acc, l) => t('common.dotPair.label', { first: acc, second: t(l.label) }),
    prStateLabel(s.state),
  );
}
