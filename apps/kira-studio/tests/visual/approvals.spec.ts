import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, test } from '../ui/fixtures';
import { contract } from '../ui/support/contract';
import { settleFonts } from './support/settle';

// P520: the approvals dialog with three queued writes, then minimised to the toast.

test('approvals dialog and toast', async ({ relaunch }) => {
  const items = contract<ApprovalItem[]>('approvals-studio', 'ApprovalsService.List').map(
    (item, i) => liveItem(item, 600, Date.now() + i),
  );
  const { window: page } = await relaunch({ control: [] });
  await page.setViewportSize({ width: 1400, height: 900 });
  await pushApprovals(page, items);
  const dialog = page.locator('[data-testid="approvals-dialog"]');
  await expect(dialog).toBeVisible();
  await expect(page.locator('[data-testid="db-mcp-approval-statement"]').first()).toBeVisible();
  await settleFonts(page);
  await expect(dialog).toHaveScreenshot('approvals-dialog.png', {
    mask: [page.locator('[data-testid="approval-countdown"]')],
  });

  await page.keyboard.press('Escape');
  const toast = page.locator('[data-testid="approvals-toast"]');
  await expect(toast).toBeVisible();
  await settleFonts(page);
  await expect(toast).toHaveScreenshot('approvals-toast.png', {
    mask: [page.locator('[data-testid="approvals-toast-expires"]')],
  });
});
