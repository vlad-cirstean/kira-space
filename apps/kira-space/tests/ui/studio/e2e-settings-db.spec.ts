import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openSettings } from './support/settings';

// Contract e2e-settings-db. Backend half: dbmcpflow TestE2ESettingsDB, the same scenario. The scripts
// switch patches apiScripts; the cards parked while the switch flips are answered by item id; what the agent
// then gets (result#step7, result#step8) is the backend's to assert.

const SC = 'e2e-settings-db';
const first = (key: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0] as ApprovalItem;
const answerArgs = (key: string) =>
  contract<{ ids: string[] }>(SC, `args:ApprovalsService.Answer#${key}`);

test('contract: the scripts switch patches apiScripts off and on', async ({ relaunch }) => {
  const off = contract<unknown>(SC, 'args:SettingsService.Set#scriptsOff');
  const on = contract<unknown>(SC, 'args:SettingsService.Set#scriptsOn');
  const enabled = { ...defaultSettings.dbMcp, apiRequests: true };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.settingsGetAll, response: { ...defaultSettings, dbMcp: enabled } },
      {
        channel: IPC.settingsSet,
        args: off,
        response: { ...defaultSettings, dbMcp: { ...enabled, apiScripts: false } },
      },
      { channel: IPC.settingsSet, args: on, response: { ...defaultSettings, dbMcp: enabled } },
    ],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Studio MCP API"]');
  const scripts = page.locator('[data-testid="settings-db-mcp-api-scripts"]');
  await expect(scripts).toBeChecked();
  await scripts.click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.settingsSet)?.args)
    .toEqual(off);
  await expect(scripts).not.toBeChecked();
  await scripts.click();
  await expect
    .poll(
      () =>
        control
          .log()
          .filter((e) => e.channel === IPC.settingsSet)
          .at(-1)?.args,
    )
    .toEqual(on);
});

test('contract: the read card offers the review box and Approve answers the item id', async ({
  relaunch,
}) => {
  const it = first('step6');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answerArgs('step6').ids) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await expect(page.locator('[data-testid="db-mcp-approval-review-result"]')).toBeVisible();
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});

for (const [key, what] of [
  ['step6-held', 'a result held after review turned on'],
  ['step8', 'a scripted result held while scripts go off'],
] as const) {
  test(`contract: ${what} is a db.response answered by its id`, async ({ relaunch }) => {
    const it = first(key);
    const { window: page, control } = await relaunch({
      control: [
        {
          channel: IPC.dbMcpPendingResponse,
          response: { requestId: 'r', text: '', raw: '', isError: false, script: null, keep: [] },
        },
        { channel: IPC.approvalsAnswer, response: answeredOk(answerArgs(key).ids) },
      ],
    });
    await pushApprovals(page, [liveItem(it)]);
    await page.click(`[data-testid="approval-approve-${it.id}"]`);
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
      .toEqual({ ids: [it.id], approve: true, options: null });
  });
}

test('contract: a scripted write card shows its script and Approve answers the id', async ({
  relaunch,
}) => {
  const it = first('step7');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answerArgs('step7').ids) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await expect(page.locator('[data-testid="db-approval-script"]')).toBeVisible();
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});
