import { ACTIVITY_RANK, type ActivityKind, activityKind } from '../activity';
import type { Session } from '../wire';

export interface AgentsSummary {
  total: number;
  byKind: Record<ActivityKind, number>;
  /** The most urgent kind present; `idle` when none run. */
  top: ActivityKind;
}

/** Counts the running sessions by activity, plus finished-not-viewed ones under `unseen`; `total`
 *  is the running count. Sessions already carry their TUI activity. */
export function agentsSummary(sessions: readonly Session[]): AgentsSummary {
  const byKind: Record<ActivityKind, number> = {
    input: 0,
    unseen: 0,
    working: 0,
    waiting: 0,
    idle: 0,
    stopped: 0,
  };
  let total = 0;
  for (const s of sessions) {
    const kind = activityKind(s);
    // A stopped row counts only as unseen; running rows count as before.
    if (s.state === 'running') total++;
    if (s.state === 'running' || kind === 'unseen') byKind[kind]++;
  }
  const kinds = (Object.keys(byKind) as ActivityKind[])
    .filter((k) => byKind[k] > 0)
    .sort((a, b) => ACTIVITY_RANK[a] - ACTIVITY_RANK[b]);
  return {
    total,
    byKind,
    top: kinds[0] ?? 'idle',
  };
}
