import { pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, test } from '../../ui/fixtures';
import { seedApprovals } from '../../ui/support/approvals';
import { contract } from '../../ui/support/contract';
import { IPC } from '../../ui/support/ipcChannels';
import {
  STORY,
  STORY_LAUNCH,
  storyApprovals,
  storyBoard,
  storyDbAudit,
} from '../support/buildStory';
import { shoot } from '../support/shoot';

const t = (id: string) => `[data-testid="${id}"]`;

test('build db prompt: approvals', async ({ relaunch, consoleErrors }) => {
  const { write, exemption } = storyApprovals();
  const { window: win } = await relaunch({
    ...STORY_LAUNCH,
    control: storyBoard(seedApprovals([write])),
  });
  const dialog = win.locator(t('approvals-dialog'));
  await expect(dialog.locator(t('approvals-count'))).toHaveText('1');
  await expect(dialog.locator(`[data-testid="approval-item-${write.id}"]`)).toContainText(
    STORY.connection,
  );
  await shoot(win, 'build-db-approval', consoleErrors);
  await shoot(win, 'build-db-statement', consoleErrors);
  await pushApprovals(win, [exemption]);
  await expect(dialog.locator(t('db-mcp-exemption-limit'))).toBeVisible();
  await shoot(win, 'build-db-exemption', consoleErrors);
});

test('build db prompt: audit trail', async ({ relaunch, consoleErrors }) => {
  const { window: win } = await relaunch({
    ...STORY_LAUNCH,
    control: storyBoard([{ channel: IPC.adeTaskDbAudit, response: { entries: storyDbAudit() } }]),
  });
  await win.locator(t('ade-plan')).waitFor();
  await win
    .locator(`${t('ade-card')}[data-task-id="${STORY.taskId}"] ${t('ade-card-head')}`)
    .click();
  await win.locator(t('ade-panel-tab-details')).click();
  await expect(win.locator(t('ade-details-dbaudit-entry')).first()).toBeVisible();
  await win.locator(t('ade-details-tab')).evaluate((el) => {
    const toggle = el.querySelector('[data-testid="ade-details-dbaudit-toggle"]');
    if (toggle)
      el.scrollTop += toggle.getBoundingClientRect().top - el.getBoundingClientRect().top - 8;
  });
  await shoot(win, 'build-db-audit', consoleErrors);
});

test('build db prompt: settings', async ({ relaunch, consoleErrors }) => {
  const { window: win } = await relaunch({
    ...STORY_LAUNCH,
    control: storyBoard([
      { channel: IPC.kubernetesClusters, response: { clusters: [] } },
      { channel: IPC.connectionsList, response: [] },
      { channel: IPC.apiConfigAgentAccess, response: [] },
      {
        channel: IPC.memoryMcpStatus,
        response: contract('memory-settings', 'MemoryService.McpStatus'),
      },
    ]),
  });
  await win.locator(t('ade-plan')).waitFor();
  await win.click(t('open-settings'));
  await win.click(t('settings-section-Database MCP'));
  await expect(win.locator(t('settings-db-mcp-focus'))).toBeVisible();
  await shoot(win, 'build-db-settings', consoleErrors);
});
