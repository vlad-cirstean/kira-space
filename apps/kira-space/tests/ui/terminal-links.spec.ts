import { registerTerminalLinkScenarios } from '@workbench/testing/ui/scenarios/terminalLinks';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { repoRowById } from './support/repoRows';

// Cmd/Ctrl-click opens a terminal link. Backend half: appflow TestTerminalLinkOpen.

const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

registerTerminalLinkScenarios(
  { test, contract, ipc: IPC },
  {
    extraBase: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
      { channel: IPC.codeWorkspaceListFiles, args: { id: REPO.id }, response: [] },
    ],
    openTerminal: async (page) => {
      await repoRowById(page, REPO.id).click();
      await page.locator('[data-testid="tab-strip-new"]').click();
    },
  },
);

// The pty hears a size only when the grid changed and then settles: a container wobble that keeps
// the same cols and rows sends nothing; a real change sends once.
test('container resizes that keep the grid send no resize; a real change sends one', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
      { channel: IPC.codeWorkspaceListFiles, args: { id: REPO.id }, response: [] },
      { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
      { channel: IPC.terminalResize, response: null },
    ],
  });
  await repoRowById(page, REPO.id).click();
  await page.locator('[data-testid="tab-strip-new"]').click();
  const host = page.locator('[data-testid="repo-terminal-host"]');
  await expect(host.locator('.xterm-rows')).toBeVisible();
  await expect.poll(() => calls(control, IPC.terminalOpen)).toHaveLength(1);

  await host.evaluate(async (el) => {
    const frame = () => new Promise<void>((r) => requestAnimationFrame(() => r()));
    // Sub-cell wobble: the pixel size changes, the cols and rows do not.
    for (let i = 0; i < 6; i++) {
      el.style.paddingRight = i % 2 === 0 ? '1px' : '0px';
      await frame();
    }
    el.style.paddingRight = '';
  });
  await page.waitForTimeout(400);
  expect(calls(control, IPC.terminalResize)).toHaveLength(0);

  await host.evaluate((el) => {
    el.style.marginRight = '300px';
  });
  await expect.poll(() => calls(control, IPC.terminalResize)).toHaveLength(1);
  await page.waitForTimeout(400);
  expect(calls(control, IPC.terminalResize)).toHaveLength(1);
});
