import { expect, test } from './fixtures';
import { openDetails, openPlan } from './support/adeV2';
import { detailsEditItem, seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Edit approvals by author: an agent's edit asks the user only for another agent's content, with no
// reason, and a save from outside the app is a disk banner row, not an approval. Contract
// ade-details-rules; backend half: flows/adeflow TestDetailsEditRules (detailsrules_test.go).

const t = (id: string) => `[data-testid="${id}"]`;
const TASK = 'T_alerts';
const SCENARIO = 'ade-details-rules';

interface ItemFx {
  payload: { items: { path: string; kind: string; cause: string }[] };
}
interface DiskFx {
  path: string;
  state: string;
  pending: boolean;
  diff: string;
}

test('contract: another agent link edit waits with its cause and no reason', async ({
  relaunch,
}) => {
  const it = detailsEditItem(SCENARIO, 'otherAgentLink');
  const { items } = it.payload as ItemFx['payload'];
  expect(items[0]?.cause).toBe('link');
  const { window: page } = await openPlan(relaunch, [...seedApprovals([it])]);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog).toBeVisible();
  await expect(dialog.locator(t('ade-details-edit-reason'))).toHaveCount(0);
  await expect(dialog.locator(t('ade-details-edit-file'))).toHaveCount(items.length);
  await expect(dialog.locator(t('ade-details-edit-file'))).toContainText('Another agent’s link');
});

test('contract: a save from outside shows as a banner row with Accept and Discard, no approval', async ({
  relaunch,
}) => {
  const changes = contract<DiskFx[]>(SCENARIO, 'AdeTaskService.DetailsDiskChanges#outside');
  const [change] = changes;
  if (!change) throw new Error('contract has no outside change');
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskDetailsDiskChanges, response: changes },
  ]);
  await openDetails(page, TASK);
  await expect(page.locator(t('ade-details-disk-banner'))).toBeVisible();
  await page.locator(t('ade-details-disk-review')).click();
  const file = page.locator(t('ade-details-disk-file'));
  await expect(file).toContainText(change.path);
  await expect(file.locator(t('ade-details-disk-accept'))).toBeVisible();
  await expect(file.locator(t('ade-details-disk-discard'))).toBeVisible();
  await expect(page.locator(t('approvals-dialog'))).toHaveCount(0);
  expect(calls(control, IPC.approvalsAnswer)).toHaveLength(0);
});
