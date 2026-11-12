import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { dragAcross } from '../support/selection';
import { openSettings } from './support/settings';

// Text in dialogs is selectable: an approval target, a host tag and the MCP command.

const dbMcp = { ...defaultSettings.dbMcp, serverEnabled: true, apiRequests: true };

test('the target of an api.request approval can be selected', async ({ relaunch }) => {
  const it = contract<ApprovalItem[]>('approvals-api', 'ApprovalsService.List')[0];
  if (!it) throw new Error('contract has no item');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const name = page.locator('[data-testid="approval-target-name"]');
  expect(await dragAcross(page, name)).toContain('127.0.0.1');
});

test('a host tag in the rule dialog can be selected', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.settingsGetAll, response: { ...defaultSettings, dbMcp } }],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');
  await page.click('[data-testid="settings-db-mcp-api-rules-add"]');
  const input = page.locator('[data-testid="api-rule-hosts"] input');
  await input.fill('api.example.com');
  await input.press('Enter');
  const tag = page.locator('[data-slot="tags-input-item-text"]');
  expect(await dragAcross(page, tag)).toContain('api.example.com');
});

test('the MCP command in Settings can be selected', async ({ relaunch }) => {
  const command = 'claude mcp add kira --transport http http://127.0.0.1:1/mcp';
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.settingsGetAll, response: { ...defaultSettings, dbMcp } },
      {
        channel: IPC.dbMcpStatus,
        response: {
          running: true,
          command,
          agentAvailable: true,
          probed: [],
          expiresAt: '',
          error: '',
        },
      },
    ],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');
  const box = page.locator('[data-testid="db-mcp-command"]');
  expect(await dragAcross(page, box)).toContain('claude mcp add');
});
