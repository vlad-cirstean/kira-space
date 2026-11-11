import { activityKind } from '../activity';
import type { Session } from '../wire';

type TaskSessionState = 'input' | 'running' | 'unseen' | 'finished';

export interface TaskSessions {
  state: TaskSessionState;
  /** Sessions with `state === 'running'`. */
  running: number;
}

/** One state for a task's agent sessions on its card; `null` when it has none. Review sessions
 *  belong to the review agent and never count. */
export function taskSessionsOf(sessions: readonly Session[]): TaskSessions | null {
  const own = sessions.filter((s) => s.purpose !== 'review');
  if (own.length === 0) return null;
  const live = own.filter((s) => s.state === 'running');
  let state: TaskSessionState = 'finished';
  if (live.some((s) => activityKind(s) === 'input')) state = 'input';
  else if (
    live.some((s) => {
      const kind = activityKind(s);
      return s.mode === 'headless' || kind === 'working' || kind === 'waiting';
    })
  )
    state = 'running';
  else if (own.some((s) => activityKind(s) === 'unseen')) state = 'unseen';
  return { state, running: live.length };
}
