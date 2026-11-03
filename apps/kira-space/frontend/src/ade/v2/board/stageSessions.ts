import type { Session } from '../wire';

const REVIEW = 'review';

/**
 * The task's running terminal sessions a move from `from` to `to` leaves behind: those of another
 * step, plus the review agent when the move leaves the review step. Branch sessions stay. Same rule
 * as `stageSessions` in `ade/stagesessions.go`.
 */
export function stageSessions(
  sessions: readonly Session[],
  taskId: string,
  from: string,
  to: string,
): Session[] {
  return sessions.filter((s) => {
    if (s.taskId !== taskId || s.mode !== 'tui' || s.state !== 'running') return false;
    if (s.purpose === REVIEW) return from === REVIEW && to !== REVIEW;
    return s.stepId !== '' && s.stepId !== to;
  });
}
