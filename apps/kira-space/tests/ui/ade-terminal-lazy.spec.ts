import { expect, test } from './fixtures';
import { adeFixture, openPlan, openTask } from './support/adeV2';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// An agent terminal nobody opened keeps its output but renders no DOM; opening its pane shows it.
// Backend half: terminal output is routed by the pty session, nothing renders there.

const t = (id: string) => `[data-testid="${id}"]`;

test('a never-opened agent terminal has no xterm DOM; its pane shows the earlier output', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.mobileLaunchOpened, response: null },
  ]);
  await openTask(page, 'T_cart');
  await emitWailsEvent(page, IPC.mobileOpenLaunch, {
    launch: adeFixture('launch'),
    taskId: 'T_bill',
    branchId: 'b_billdash',
  });
  await expect.poll(() => calls(control, IPC.terminalOpen)).toHaveLength(1);
  await emitWailsEvent(page, IPC.terminal, {
    terminalId: 'term-tk01',
    data: Buffer.from('hello from the agent\r\n').toString('base64'),
    exited: false,
  });
  await expect(page.locator('.xterm')).toHaveCount(0);

  await openTask(page, 'T_bill');
  await page.locator(t('ade-panel-tab-sessions')).click();
  await expect(page.locator(`${t('repo-terminal-host')} .xterm-rows`)).toContainText(
    'hello from the agent',
  );
});
