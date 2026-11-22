import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { expect, studioTest as test } from '../../ui/fixtures';
import { idOf, setupKubernetes } from '../../ui/studio/support/kubernetesFixture';
import { openSettings } from '../../ui/studio/support/settings';
import { contract } from '../../ui/support/contract';
import { IPC } from '../../ui/support/ipcChannels';
import { settleFonts } from './support/settle';

// P717: the kube result review. Contract kube-review (dbmcpflow TestKubeResponseReview).

const SC = 'kube-review';
const item = (key: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0] as ApprovalItem;
const held = (key: string) =>
  contract<Record<string, unknown>>(SC, `DbMcpService.PendingResponse#${key}`);

async function show(page: Page, key: string, ready: string): Promise<void> {
  await page.setViewportSize({ width: 1400, height: 900 });
  await pushApprovals(page, [liveItem(item(key), 600, Date.now())]);
  await expect(page.locator(`[data-testid="${ready}"]`).first()).toBeVisible();
  await settleFonts(page);
}
const dialog = (page: Page) => page.locator('[data-testid="approvals-dialog"]');
const mask = (page: Page) => [page.locator('[data-testid="approval-countdown"]')];
const pending = (key: string) => [{ channel: IPC.dbMcpPendingResponse, response: held(key) }];

test('kube read offers the review option', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await show(page, 'readOption', 'kube-approval-review-result');
  await expect(dialog(page)).toHaveScreenshot('approvals-kube-read-review-option.png', {
    mask: mask(page),
  });
});

test('kube list review: table, expanded row, cell popover', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: pending('list') });
  await show(page, 'list', 'kube-review-row');
  await page.locator('[data-testid="kube-review-row"]').first().click();
  const second = page.locator('[data-testid="kube-review-row"]').nth(1);
  await second.hover();
  await second.locator('[data-testid="kube-review-expand"]').first().click();
  await expect(page.locator('[data-testid="kube-review-cell"]')).toBeVisible();
  await settleFonts(page);
  await expect(page).toHaveScreenshot('approvals-kube-review-list.png', { mask: mask(page) });
});

test('kube describe of a Secret: manifest and the redaction note', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: pending('describeSecret') });
  await show(page, 'describeSecret', 'kube-review-mask');
  await expect(dialog(page)).toHaveScreenshot('approvals-kube-review-describe.png', {
    mask: mask(page),
  });
});

test('kube logs review: log view and the truncated footnote', async ({ relaunch }) => {
  const text = JSON.stringify({
    lines: [{ text: 'line two' }, { text: 'line three' }, { text: 'line four, a longer one' }],
    truncated: true,
  });
  const { window: page } = await relaunch({
    control: [{ channel: IPC.dbMcpPendingResponse, response: { ...held('logs'), text } }],
  });
  await show(page, 'logs', 'kube-review-summary');
  await expect(dialog(page)).toHaveScreenshot('approvals-kube-review-logs.png', {
    mask: mask(page),
  });
});

test('cluster MCP dialog with the review field', async ({ relaunch }) => {
  const { page } = await setupKubernetes(relaunch, {
    Clusters: () => {
      const res = contract<{ clusters: { id: string; context: string }[] }>(
        'dbmcp-kube',
        'KubernetesService.Clusters:enabled',
      );
      return { ...res, clusters: res.clusters.map((c) => ({ ...c, id: idOf(c.context) })) };
    },
  });
  await page.setViewportSize({ width: 1400, height: 900 });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Studio MCP clusters"]');
  await page.locator(`[data-testid="db-mcp-kube-edit-${idOf('kind-dev')}"]`).click();
  const dlg = page.locator('[data-testid="kube-mcp-dialog"]');
  await expect(dlg.locator('[data-testid="kube-mcp-review"]')).toBeVisible();
  await settleFonts(page);
  await expect(dlg).toHaveScreenshot('kube-mcp-dialog-review.png');
});

test('settings: both result-review switches on the Studio MCP pane', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.settingsSet,
        response: {
          ...defaultSettings,
          dbMcp: { ...defaultSettings.dbMcp, previewResponses: true, kubeReviewResponses: true },
        },
      },
    ],
  });
  await page.setViewportSize({ width: 1400, height: 900 });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Studio MCP"]');
  await page.click('[data-testid="settings-db-mcp-preview"]');
  await page.click('[data-testid="settings-kube-mcp-preview"]');
  await expect(page.locator('[data-testid="settings-kube-mcp-preview"]')).toBeChecked();
  await settleFonts(page);
  await expect(page.locator('[data-testid="settings-dialog"]')).toHaveScreenshot(
    'settings-mcp-review-results.png',
  );
});
