import { type MessageKey, t } from '@theme/i18n';
import { CHANGE_REQUEST_TONE } from '@theme/lib/statusTone';
import type { Tone } from '../board/actions';
import { PASTEL } from '../tones';

export const PR_TONE: Record<string, Tone> = Object.fromEntries(
  Object.entries(CHANGE_REQUEST_TONE).map(([k, v]) => [k, PASTEL[v]]),
);

const PR_STATE = {
  open: 'common.action.open.label',
  merged: 'common.merged.status',
  closed: 'common.closed.status',
  draft: 'common.draft.status',
} as const satisfies Record<string, MessageKey>;

export const prStateLabel = (state: string): string =>
  state in PR_STATE ? t(PR_STATE[state as keyof typeof PR_STATE]) : state;
