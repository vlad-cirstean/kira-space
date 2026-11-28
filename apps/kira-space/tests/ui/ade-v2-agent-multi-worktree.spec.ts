import { expect, test } from './fixtures';
import { adeBoard, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// An agent names one of several branches of one repository by its worktree (P762): the board holds two
// rows before and one after. Contract ade-agent-multi-worktree; backend half: flows/adeflow
// TestAgentMultiWorktree.

const t = (id: string) => `[data-testid="${id}"]`;

interface RowFx {
  id: string;
  name: string;
  setup: Record<string, unknown> | null;
}

// The task's api rows from one contract board; the unnamed row is the web repo's, not under test.
function apiRows(key: string): RowFx[] {
  return contract<RowFx[]>('ade-agent-multi-worktree', key).filter((r) => r.name !== '');
}

function planFor(rows: RowFx[]) {
  return [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        const task = b.tasks.find((x) => x.id === 'T_alerts');
        if (!task) throw new Error('T_alerts missing');
        const api = b.branches.find((x) => x.id === 'd_alerts_api');
        if (!api) throw new Error('d_alerts_api missing');
        const [first, ...rest] = rows;
        if (!first) throw new Error('contract lacks a branch');
        Object.assign(api, {
          ...first,
          id: 'd_alerts_api',
          taskId: 'T_alerts',
          codeRepoId: 'repo-api',
          base: 'main',
        });
        task.branchIds = (task.branchIds as string[]).filter(
          (id) =>
            !b.branches.some(
              (x) => x.id === id && x.codeRepoId === 'repo-api' && id !== 'd_alerts_api',
            ),
        );
        rest.forEach((row, i) => {
          const id = `b_alerts_api${i + 2}`;
          (task.branchIds as string[]).push(id);
          b.branches.push({ ...row, id, taskId: 'T_alerts', codeRepoId: 'repo-api', base: 'main' });
        });
      }),
    },
  ];
}

test('two agent branches of one repository render as two rows', async ({ relaunch }) => {
  const rows = apiRows('AdeTaskService.Board#before');
  expect(rows.map((r) => r.name)).toEqual(['feat/one', 'feat/two']);
  const { window: page } = await openPlan(relaunch, planFor(rows));
  await openTask(page, 'T_alerts');
  const row = (id: string) => page.locator(`${t('ade-task-branch')}[data-branch-id="${id}"]`);
  await expect(row('d_alerts_api')).toContainText('feat/one');
  await expect(row('b_alerts_api2')).toContainText('feat/two');
});

test('after the agent removes one worktree only the other branch stays', async ({ relaunch }) => {
  const rows = apiRows('AdeTaskService.Board#afterRemove');
  expect(rows.map((r) => r.name)).toEqual(['feat/one']);
  const { window: page } = await openPlan(relaunch, planFor(rows));
  await openTask(page, 'T_alerts');
  const branches = page.locator(t('ade-task-branch'));
  await expect(
    page.locator(`${t('ade-task-branch')}[data-branch-id="d_alerts_api"]`),
  ).toContainText('feat/one');
  await expect(branches.filter({ hasText: 'feat/two' })).toHaveCount(0);
  await expect(branches.filter({ hasText: 'feat/one' })).toHaveCount(1);
});
