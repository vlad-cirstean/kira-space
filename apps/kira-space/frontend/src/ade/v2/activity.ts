import type { AgentActivity } from '@shared/domain/agent';
import { idBody, isUuid } from '@shared/ids';
import { type MessageKey, messageDefaults, t } from '@theme/i18n';
import type { Session } from './wire';

export type ActivityKind = 'input' | 'unseen' | 'working' | 'waiting' | 'idle' | 'stopped';

const ACTIVITY_KEY = {
  input: 'space.ade.activity.input.status',
  unseen: 'space.ade.activity.unseen.status',
  working: 'space.ade.activity.working.status',
  waiting: 'space.ade.activity.waiting.status',
  idle: 'common.state.idle.status',
  stopped: 'space.ade.activity.stopped.status',
} as const satisfies Record<ActivityKind, MessageKey>;

export const activityLabel = (kind: ActivityKind): string => t(ACTIVITY_KEY[kind]);

/** Urgency order of the All sessions list: input first. */
export const ACTIVITY_RANK: Record<ActivityKind, number> = {
  input: 0,
  unseen: 1,
  working: 2,
  waiting: 3,
  idle: 4,
  stopped: 5,
};

/** Last four characters of a UUID (a v7 head is a timestamp), else the first four of a prefixed id's body. */
/** `Claude 9ab0`: the agent's name and a short session id. */
export const agentSessionName = (shortSessionId: string): string =>
  `${messageDefaults().agent} ${shortSessionId}`;

export function shortId(id: string): string {
  return isUuid(id) ? id.slice(-4) : idBody(id).slice(0, 4);
}

const PHASE: Record<AgentActivity['phase'], Session['activity']> = {
  attention: 'input',
  working: 'working',
  waiting: 'waiting',
  idle: 'idle',
};

/** The server reports no activity for a TUI session: it comes from the agent-hook store, keyed by
 *  terminal id. A running TUI session with no hook seen yet reads `idle`. An idle phase with
 *  background tasks left reads `working`; a wake armed (`waiting`) wins over background work. The
 *  count comes from the hook store when it has the terminal, else from the wire (a window opened
 *  after the last event). */
export function withTuiActivity(
  sessions: readonly Session[],
  activity: ReadonlyMap<string, Pick<AgentActivity, 'phase' | 'background'>>,
): Session[] {
  return sessions.map((s) => {
    if (s.mode !== 'tui' || s.state !== 'running') return s;
    const entry = activity.get(s.terminalId);
    const phase = entry ? PHASE[entry.phase] : 'idle';
    const background = entry ? entry.background : s.background;
    return { ...s, activity: phase === 'idle' && background > 0 ? 'working' : phase };
  });
}

/** Display precedence: a busy running session shows that; else unseen; else idle or stopped. */
export function activityKind(s: Session): ActivityKind {
  if (
    s.state === 'running' &&
    (s.activity === 'input' || s.activity === 'working' || s.activity === 'waiting')
  ) {
    return s.activity;
  }
  if (s.unseen) return 'unseen';
  if (s.state !== 'running') return 'stopped';
  return 'idle';
}
