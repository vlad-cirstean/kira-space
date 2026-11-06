import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { editorText } from './support/editorText';
import { openSettings } from './support/settings';

// Contract dbmcp-preview. Backend half: dbmcpflow TestDbMcpResponsePreview. With the setting on, a
// run_query result that already ran waits in a review dialog; Send releases the held bytes.

interface Held {
  requestId: string;
  text: string;
}
interface Tabular {
  columns: { name: string }[];
  rows: (string | null)[][];
}

const items = () => contract<ApprovalItem[]>('dbmcp-preview', 'ApprovalsService.List');
const held = () => contract<Held>('dbmcp-preview', 'DbMcpService.PendingResponse');

function first(alreadyRan?: boolean): ApprovalItem {
  const [item] = items();
  if (!item) throw new Error('contract has no item');
  const live = liveItem(item);
  return alreadyRan === undefined
    ? live
    : { ...live, payload: { ...(live.payload as object), alreadyRan } };
}

function mocks(id: string) {
  return [
    { channel: IPC.dbMcpPendingResponse, response: held() },
    { channel: IPC.approvalsAnswer, response: answeredOk([id]) },
  ];
}

test('contract: the held result shows as a table and Send answers by item id', async ({
  relaunch,
}) => {
  const item = first();
  const result = JSON.parse(held().text) as Tabular;
  const answer = contract<{ ids: string[] }>('dbmcp-preview', 'args:ApprovalsService.Answer');
  const { window: page, control } = await relaunch({ control: mocks(item.id) });

  await pushApprovals(page, [item]);
  await expect(page.locator('[data-testid="approval-operation"]')).toHaveText(
    new RegExp(`^${item.operation.class}$`, 'i'),
  );
  const headers = page.locator('[data-testid="db-mcp-preview-column"]');
  await expect(headers).toHaveText(result.columns.map((c) => c.name));
  const rows = page.locator('[data-testid="db-mcp-preview-row"]');
  await expect(rows).toHaveCount(result.rows.length);
  for (const [i, row] of result.rows.entries()) {
    await expect(rows.nth(i).locator('td')).toHaveText(row.map((cell) => cell ?? 'NULL'));
  }
  await expect(page.locator('[data-testid="db-mcp-preview-ran"]')).toHaveCount(0);

  await page.click('[data-testid="db-mcp-preview-view-as-sent"]');
  await expect
    .poll(() => editorText(page.locator('[data-testid="db-mcp-preview-raw"]')))
    .toBe(held().text);

  await page.click(`[data-testid="approval-approve-${item.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ...answer, options: null });
});

test('contract: Withhold denies the held result', async ({ relaunch }) => {
  const item = first();
  const { window: page, control } = await relaunch({ control: mocks(item.id) });
  await pushApprovals(page, [item]);
  await expect(page.locator(`[data-testid="approval-deny-${item.id}"]`)).toHaveText('Withhold');
  await page.click(`[data-testid="approval-deny-${item.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [item.id], approve: false, options: null });
});

test('contract: a write that already ran warns that withholding does not undo it', async ({
  relaunch,
}) => {
  const item = first(true);
  const { window: page } = await relaunch({ control: mocks(item.id) });
  await pushApprovals(page, [item]);
  await expect(page.locator('[data-testid="db-mcp-preview-ran"]')).toBeVisible();
});

test('contract: the preview switch sends the settings patch', async ({ relaunch }) => {
  const args = contract<unknown>('dbmcp-preview', 'args:SettingsService.Set');
  const { window: page, control } = await relaunch({
    control: [
      {
        channel: IPC.settingsSet,
        response: {
          ...defaultSettings,
          dbMcp: { ...defaultSettings.dbMcp, previewResponses: true },
        },
      },
    ],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');
  await page.click('[data-testid="settings-db-mcp-preview"]');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.settingsSet)?.args)
    .toEqual(args);
  await expect(page.locator('[data-testid="settings-db-mcp-preview"]')).toBeChecked();
});
