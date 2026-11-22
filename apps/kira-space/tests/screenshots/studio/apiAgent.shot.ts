import { CAPTURE_LAUNCH, CAPTURE_NOW } from '@workbench/testing/screenshots/capture';
import { expect, test } from '../../ui/fixtures';
import { seedApprovals } from '../../ui/support/approvals';
import { IPC } from '../../ui/support/ipcChannels';
import { STORY, storyBoard } from '../support/buildStory';
import { expandApprovalsDialog, shoot } from '../support/shoot';
import {
  API_HOST,
  storyApiRequest,
  storyApiReview,
  storyApiValues,
  storyKeptList,
} from '../support/storyApi';

// P734: agent API requests: the approval card, the held response, typed values and the saved
// variables. Run by `bun run screenshots`; output goes to /screenshots/crop.

const t = (id: string) => `[data-testid="${id}"]`;
// Taller than a window shot: a crop clips to the viewport, and a card with a script is long.
const TALL = { ...CAPTURE_LAUNCH, viewport: { width: 1024, height: 1100 } } as const;

test('agent api: request card', async ({ relaunch, consoleErrors }) => {
  const item = storyApiRequest();
  const { window: win } = await relaunch({
    ...TALL,
    control: storyBoard(seedApprovals([item])),
  });
  const card = win.locator(`[data-testid="approval-item-${item.id}"]`);
  await expect(card).toContainText(API_HOST);
  await expect(card.locator(t('api-approval-script-transform'))).toBeVisible();
  await expect(card.locator(t('api-approval-script-transform-formatted'))).toBeVisible();
  await expect
    .poll(() =>
      card.locator(t('api-approval-script-transform')).locator('.cm-line, .view-line').count(),
    )
    .toBeGreaterThan(3);
  await win.clock.setFixedTime(CAPTURE_NOW + 1000);
  await expandApprovalsDialog(win);
  await shoot(win, 'build-api-request', consoleErrors);
});

test('agent api: response review', async ({ relaunch, consoleErrors }) => {
  const { item, held } = storyApiReview();
  const { window: win } = await relaunch({
    ...TALL,
    control: storyBoard([
      ...seedApprovals([item]),
      { channel: IPC.dbMcpPendingResponse, response: held },
    ]),
  });
  const card = win.locator(`[data-testid="approval-item-${item.id}"]`);
  await expect(card.locator('.view-line').filter({ hasText: /^\s*file:/ })).toHaveCount(1, {
    timeout: 15_000,
  });
  await win.clock.setFixedTime(CAPTURE_NOW + 1000);
  await expandApprovalsDialog(win);
  await shoot(win, 'build-api-review', consoleErrors);
});

test('agent api: typed values', async ({ relaunch, consoleErrors }) => {
  const item = storyApiValues();
  const { window: win } = await relaunch({
    ...TALL,
    control: storyBoard(seedApprovals([item])),
  });
  await expect(win.locator(t('api-input-otp'))).toBeVisible();
  await win.locator(t('api-input-otp')).fill('482913');
  await win.locator(t('api-approval-input-replace')).click();
  await win.clock.setFixedTime(CAPTURE_NOW + 1000);
  await expandApprovalsDialog(win);
  await shoot(win, 'studio-api-values', consoleErrors);
});

test('agent api: saved variables', async ({ relaunch, consoleErrors }) => {
  const { window: win } = await relaunch({
    ...CAPTURE_LAUNCH,
    control: storyBoard([
      { channel: IPC.kubernetesClusters, response: { clusters: [] } },
      { channel: IPC.connectionsList, response: [] },
      { channel: IPC.apiConfigAgentAccess, response: [] },
      { channel: IPC.keptList, response: storyKeptList() },
    ]),
  });
  await win.locator(t('ade-plan')).waitFor();
  await win.click(t('open-settings'));
  await win.click(t('settings-section-Studio MCP variables'));
  await expect(win.locator(t('settings-kept-entry'))).toHaveCount(3);
  await win.clock.setFixedTime(CAPTURE_NOW + 1000);
  await shoot(win, 'studio-saved-variables', consoleErrors);
});
