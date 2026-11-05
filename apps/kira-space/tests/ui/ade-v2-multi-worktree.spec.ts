import { expect, test } from './fixtures';
import { adeBoard, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// A task with several branches in one repository: each renders its own row and the repository stays in
// the Add repository picker. Contract ade-multi-worktree; backend half: flows/adeflow TestMultiWorktree.

const t = (id: string) => `[data-testid="${id}"]`;

interface RowFx {
  id: string;
  name: string;
  setup: Record<string, unknown> | null;
}

test('two branches of one repository show as two rows and the repository stays pickable', async ({
  relaunch,
}) => {
  const [one, two] = contract<RowFx[]>('ade-multi-worktree', 'AdeTaskBoard.Board.branches');
  if (!one || !two) throw new Error('contract lacks two branches');
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        const task = b.tasks.find((x) => x.id === 'T_alerts');
        if (!task) throw new Error('T_alerts missing');
        const api = b.branches.find((x) => x.id === 'd_alerts_api');
        if (!api) throw new Error('d_alerts_api missing');
        Object.assign(api, {
          ...one,
          id: 'd_alerts_api',
          taskId: 'T_alerts',
          codeRepoId: 'repo-api',
          base: 'main',
        });
        (task.branchIds as string[]).push('b_alerts_api2');
        b.branches.push({
          ...two,
          id: 'b_alerts_api2',
          taskId: 'T_alerts',
          codeRepoId: 'repo-api',
          base: 'main',
        });
      }),
    },
  ]);
  await openTask(page, 'T_alerts');
  const row = (id: string) => page.locator(`${t('ade-task-branch')}[data-branch-id="${id}"]`);
  await expect(row('d_alerts_api')).toContainText(one.name);
  await expect(row('b_alerts_api2')).toContainText(two.name);
  await page.locator(t('ade-add-repo')).click();
  await expect(page.locator(t('ade-add-repo-option-repo-api'))).toBeVisible();
});
