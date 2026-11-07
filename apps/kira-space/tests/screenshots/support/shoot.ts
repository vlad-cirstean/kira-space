import type { Page } from '@playwright/test';
import { captureShot } from '@workbench/testing/screenshots/capture';
import { captureCrop } from '@workbench/testing/screenshots/crop';
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
