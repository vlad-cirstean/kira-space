import type { Page } from '@playwright/test';
import { captureShot } from '@workbench/testing/screenshots/capture';
import { captureCrop } from '@workbench/testing/screenshots/crop';
import { capturePhone } from '@workbench/testing/screenshots/phone';
import {
  SHOT_CAPTURE,
  type ShotCapture,
  type ShotId,
} from '../../../../../site/src/data/shotCapture';

/** Captures one shot the way `site/src/data/shotCapture.ts` lists it. */
export async function shoot(
  page: Page,
  id: ShotId,
  consoleErrors: readonly string[],
): Promise<void> {
  const capture: ShotCapture = SHOT_CAPTURE[id];
  if (capture.kind === 'window') {
    await captureShot(page, { id, consoleErrors });
    return;
  }
  if (capture.kind === 'phone') {
    await capturePhone(page, { id, consoleErrors });
    return;
  }
  await captureCrop(page, {
    id,
    kind: capture.kind,
    selector: capture.selector,
    padding: capture.padding,
    top: capture.kind === 'region' ? capture.top : undefined,
    bottom: capture.kind === 'region' ? capture.bottom : undefined,
    consoleErrors,
  });
}

/** Lets the approvals dialog grow to its content, so a crop shows header, card and actions whole.
 *  The app pins the dialog at 640 px and scrolls the card; call on a viewport tall enough. */
export async function expandApprovalsDialog(page: Page): Promise<void> {
  await page.addStyleTag({
    content:
      '[data-testid="approvals-dialog"]{height:auto!important;max-height:none!important}[data-testid="approvals-dialog"] *{max-height:none!important}',
  });
}
