import { adeBoard, adeFixture } from './adeV2';
import { contract } from './contract';
import { IPC } from './ipcChannels';
import type { ControlSnapshot } from './types';

// Stage fixtures shared by the stage-move and action-tone specs. Contract ade-stage-move.

export interface SessionFx extends Record<string, unknown> {
  id: string;
  taskId: string;
  mode: string;
  state: string;
  stepId: string;
}

const recorded = contract<{ sessions: SessionFx[] }>(
  'ade-stage-move',
  'AdeTaskService.Sessions#stage-open',
).sessions;

/** A recorded running terminal session placed on `taskId` and `stepId` ('' = a branch session). */
export function stageSession(id: string, taskId: string, stepId: string): SessionFx {
  const base = recorded[0];
  if (!base) throw new Error('contract has no sessions');
  return { ...base, id, taskId, stepId };
}

/** The fixture sessions with `taskId`'s terminal sessions replaced by `own`. */
export function sessionsOf(taskId: string, own: SessionFx[]): ControlSnapshot {
  const fx = adeFixture<{ sessions: SessionFx[] }>('sessions');
  const rest = fx.sessions.filter((s) => !(s.taskId === taskId && s.mode === 'tui'));
  return { channel: IPC.adeTaskSessions, response: { ...fx, sessions: [...rest, ...own] } };
}

export const taskAtStep = (taskId: string, stepId: string): ControlSnapshot => ({
  channel: IPC.adeTaskBoard,
  response: adeBoard((b) => {
    const row = b.tasks.find((x) => x.id === taskId);
    if (!row) throw new Error(`fixture task ${taskId} missing`);
    row.entryStepId = stepId;
    row.workflowId = 'standard';
  }),
});

/** The standard workflow without its script tail, so Review is the last stage. */
export function reviewLast(): ControlSnapshot {
  const fx = adeFixture<{
    workflows: { workflow: { id: string; steps: { id: string; skip: boolean }[] } | null }[];
  }>('workflows');
  const std = fx.workflows.find((w) => w.workflow?.id === 'standard')?.workflow;
  const release = std?.steps.find((s) => s.id === 'release');
  if (!release) throw new Error('fixture workflow missing');
  release.skip = true;
  return { channel: IPC.adeTaskWorkflows, response: fx };
}
