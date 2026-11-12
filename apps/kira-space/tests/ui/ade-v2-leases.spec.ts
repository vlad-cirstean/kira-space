import { expect, test } from './fixtures';
import { openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// Resource leases: the queue panel and the lease chip on task rows. Contract ade-leases; backend
// half: flows/adeflow TestLeases (leases_test.go), same scenario: queued, granted, forced, ended.

const t = (id: string) => `[data-testid="${id}"]`;

interface Entry extends Record<string, unknown> {
  taskTitle: string;
}
interface Snapshot {
  queues: { name: string; holder: Entry | null; waiters: Entry[] }[];
}

// The contract's masked task ids become the fixture board's tasks.
const TASKS: Record<string, string> = {
  'Task A': 'T_bill',
  'Task B': 'T_cart',
  'Task C': 'T_alerts',
};

function snapshot(key: 'queued' | 'granted' | 'forced' | 'ended'): Snapshot {
  const snap = contract<Snapshot>('ade-leases', `LeasesService.Snapshot#${key}`);
  const own = (e: Entry): Entry => ({ ...e, taskId: TASKS[e.taskTitle] });
  return {
    queues: snap.queues.map((q) => ({
      ...q,
      holder: q.holder && own(q.holder),
      waiters: q.waiters.map(own),
    })),
  };
}

const open = [{ channel: IPC.leasesSnapshot, response: snapshot('queued') }];

async function openLeases(page: import('@playwright/test').Page): Promise<void> {
  await page.locator(t('ade-tab-agents')).click();
  await page.locator(t('ade-leases-tab')).click();
  await expect(page.locator(t('ade-leases'))).toBeVisible();
}

test('contract: the panel lists holder and waiters in grant order, rows show the chip', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, open);
  const chip = (id: string) =>
    page.locator(`${t('ade-card')}[data-task-id="${id}"] ${t('ade-lease-chip')}`);
  await expect(chip('T_bill')).toContainText('#1 in push');
  await expect(chip('T_cart')).toContainText('#2 in push');

  await openLeases(page);
  await expect(page.locator(t('ade-lease-holder-push'))).toHaveText('Task C');
  await expect(page.locator(t('ade-lease-waiter-push-1'))).toContainText('Task A');
  await expect(page.locator(t('ade-lease-waiter-push-2'))).toContainText('Task B');

  // The grant moves the holder; the pushed snapshot re-reads.
  control.respond(IPC.leasesSnapshot, snapshot('granted'));
  await emitWailsEvent(page, IPC.adeLeasesChanged, null);
  await expect(page.locator(t('ade-lease-holder-push'))).toHaveText('Task A');
  await expect(page.locator(t('ade-lease-waiter-push-1'))).toContainText('Task B');
  await page.locator(t('ade-tab-plan')).click();
  await expect(chip('T_bill')).toContainText('Holds push');
  await expect(chip('T_bill')).toHaveAttribute('data-holds', 'true');
});

test('contract: Request release asks the holder, Force release revokes after a confirm', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    ...open,
    { channel: IPC.leasesRequestRelease },
    { channel: IPC.leasesForceRelease },
  ]);
  await openLeases(page);

  await page.locator(t('ade-lease-holder-push')).click();
  await page.locator(t('ade-lease-request')).click();
  await expect.poll(() => calls(control, IPC.leasesRequestRelease)).toHaveLength(1);
  expect(calls(control, IPC.leasesRequestRelease)[0]?.args).toEqual({ queue: 'push' });

  await page.locator(t('ade-lease-holder-push')).click();
  await page.locator(t('ade-lease-force')).click();
  await expect(page.locator(t('confirm-dialog-message'))).toContainText('Task C');
  await page.locator(t('confirm-dialog-cancel')).click();
  expect(calls(control, IPC.leasesForceRelease)).toHaveLength(0);

  await page.locator(t('ade-lease-holder-push')).click();
  await page.locator(t('ade-lease-force')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect.poll(() => calls(control, IPC.leasesForceRelease)).toHaveLength(1);
  expect(calls(control, IPC.leasesForceRelease)[0]?.args).toEqual({ queue: 'push' });

  control.respond(IPC.leasesSnapshot, snapshot('forced'));
  await emitWailsEvent(page, IPC.adeLeasesChanged, null);
  await expect(page.locator(t('ade-lease-holder-push'))).toHaveText('Task B');
  await expect(page.locator(t('ade-lease-waiter-push-1'))).toHaveCount(0);
});

test('contract: a queue whose holder ended shows as free', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.leasesSnapshot, response: snapshot('ended') },
  ]);
  await openLeases(page);
  await expect(page.locator(t('ade-lease-free-push'))).toBeVisible();
  await expect(page.locator(t('ade-lease-holder-push'))).toHaveCount(0);
});

test('the queue editor adds a name and refuses an empty or duplicate one', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    ...open,
    { channel: IPC.leasesSaveQueues },
  ]);
  await openLeases(page);
  const input = page.locator(t('ade-lease-queue-input'));
  const add = page.locator(t('ade-lease-queue-add'));

  await expect(add).toBeDisabled();
  await input.fill('push');
  await expect(add).toBeDisabled();
  await input.fill('port-3000');
  await add.click();
  await expect.poll(() => calls(control, IPC.leasesSaveQueues)).toHaveLength(1);
  expect(calls(control, IPC.leasesSaveQueues)[0]?.args).toEqual({ names: ['push', 'port-3000'] });

  await page.locator(t('ade-lease-remove-push')).click();
  await expect.poll(() => calls(control, IPC.leasesSaveQueues)).toHaveLength(2);
  expect(calls(control, IPC.leasesSaveQueues)[1]?.args).toEqual({ names: [] });
});
