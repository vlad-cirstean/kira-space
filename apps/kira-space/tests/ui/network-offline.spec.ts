import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { CHANNEL } from '@shared/protocol/events';
import { answeredOk, liveItem } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { FIXED_NOW, openPlan, openWorkflows } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// The webview `offline` event flips TanStack's onlineManager. Desktop calls are bridge calls to the
// local Go process and must not pause. The network stays up, so a stall can only come from TanStack.
// Dispatched after boot: the manager is subscribed at VueQueryPlugin install, an earlier event is missed.

const t = (id: string) => `[data-testid="${id}"]`;
const goOffline = (page: Page) => page.evaluate(() => window.dispatchEvent(new Event('offline')));

interface Listing {
  steps: { fileName: string }[];
  [key: string]: unknown;
}
const before = contract<Listing>('ade-readable-names', 'AdeTaskService.Workflows#before');
const renamedList = contract<Listing>('ade-readable-names', 'AdeTaskService.Workflows#renamed');

test('a query refetches after the window goes offline', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch, [
    { channel: IPC.adeTaskWorkflows, response: before },
  ]);
  await page.locator(t('ade-wf-tab-steps')).click();
  await expect(page.locator(`${t('ade-step-row')}[data-file="lint-repo.yaml"]`)).toHaveCount(0);
  await goOffline(page);
  control.respond(IPC.adeTaskWorkflows, renamedList);
  await emitWailsEvent(page, CHANNEL.adeTaskWorkflows, null);
  await expect(page.locator(`${t('ade-step-row')}[data-file="lint-repo.yaml"]`)).toHaveCount(1);
});

test('a raw mutation runs after the window goes offline', async ({ relaunch }) => {
  const [item] = contract<ApprovalItem[]>('approvals-space', 'ApprovalsService.List');
  if (!item) throw new Error('contract has no item');
  const it = liveItem(item, 1800, FIXED_NOW);
  const answer = contract<{ ids: string[] }>('approvals-space', 'args:ApprovalsService.Answer');
  const { window: page, control } = await openPlan(relaunch, [
    ...seedApprovals([it]),
    { channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) },
  ]);
  await expect(page.locator(t('approvals-dialog'))).toBeVisible();
  await goOffline(page);
  await page.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
});

test('a useAsyncAction write runs after the window goes offline', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await page.locator(t('ade-wf-tab-steps')).click();
  await goOffline(page);
  await page.locator(t('ade-step-new')).click();
  await page.locator(t('ade-step-new-script')).click();
  await expect.poll(() => calls(control, IPC.adeTaskNewStep)).toHaveLength(1);
});
