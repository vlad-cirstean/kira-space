import { expect, test } from './fixtures';
import { openDetails, openPlan } from './support/adeV2';
import { detailsEditItem, seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// The task file watcher: Kira applies a live agent's saved edits, so the newest note shows its new
// text with no banner and no approval, an older note's edit waits with the agent's reason, and a board
// push refetches the disk banner. Contract ade-details-watch; backend half: flows/adeflow
// TestDetailsWatch (detailswatch_test.go).

const t = (id: string) => `[data-testid="${id}"]`;
const TASK = 'T_alerts';
const SCENARIO = 'ade-details-watch';

interface ChainFx {
  id: string;
  taskId: string;
  body: string;
  source: string;
  state: string;
}

const applied = (): ChainFx[] =>
  contract<{ handovers: ChainFx[] }>(SCENARIO, 'AdeTaskService.Handovers#applied').handovers.map(
    (h, i) => ({ ...h, id: `h_${i}`, taskId: TASK }),
  );

test('contract: the newest note and the draft show the applied text with no disk banner', async ({
  relaunch,
}) => {
  const entries = applied();
  const newest = entries.find((h) => h.state === 'note' && h.body === 'Newer note, edited.');
  if (!newest) throw new Error('contract has no applied note');
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskHandovers, response: { handovers: entries } },
    { channel: IPC.adeTaskDetailsDiskChanges, response: [] },
  ]);
  await openDetails(page, TASK);
  const chain = page.locator(`${t('ade-task-chain')} [data-author]`);
  await expect(chain.filter({ hasText: newest.body })).toHaveCount(1);
  await expect(chain.filter({ hasText: 'Draft v1.' })).toHaveCount(1);
  await expect(page.locator(t('ade-details-disk-banner'))).toHaveCount(0);
});

test('contract: an older note edit waits with the agent reason', async ({ relaunch }) => {
  const it = detailsEditItem(SCENARIO);
  const { reason, items } = it.payload as { reason: string; items: { path: string }[] };
  expect(reason).toBe('typo fixed');
  const { window: page } = await openPlan(relaunch, [...seedApprovals([it])]);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog).toBeVisible();
  await expect(dialog.locator(t('ade-details-edit-reason'))).toContainText(reason);
  await expect(dialog.locator(t('ade-details-edit-file'))).toHaveCount(items.length);
});

test('a board push refetches the disk changes of the open task', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskDetailsDiskChanges, response: [] },
  ]);
  await openDetails(page, TASK);
  await expect(page.locator(t('ade-details-disk'))).toBeVisible();
  await expect.poll(() => calls(control, IPC.adeTaskDetailsDiskChanges).length).toBeGreaterThan(0);
  const before = calls(control, IPC.adeTaskDetailsDiskChanges).length;
  await emitWailsEvent(page, IPC.adeTaskBoardChanged, { refs: false, taskIds: [TASK] });
  await expect
    .poll(() => calls(control, IPC.adeTaskDetailsDiskChanges).length)
    .toBeGreaterThan(before);
});
