import type { Page } from '@playwright/test';

export interface FirstFrame {
  /** Frame index (from arming) at which the dialog element first existed; null if never. */
  dialogFrame: number | null;
  /** First frame at which the content element existed with text; null if never. */
  contentFrame: number | null;
  /** Content text and height in the dialog's first frame. */
  textAtDialogFrame: string;
  heightAtDialogFrame: number;
}

interface Probe {
  settled: boolean;
  result: FirstFrame;
}

declare global {
  interface Window {
    __firstFrame?: Probe;
  }
}

const MAX_FRAMES = 180;

/** Starts a per-frame probe; call before the action that opens the dialog, then `firstFrame`. */
export async function armFirstFrame(
  page: Page,
  dialogTestId: string,
  contentTestId: string,
): Promise<void> {
  await page.evaluate(
    ([dialogId, contentId, max]) => {
      const probe: Probe = {
        settled: false,
        result: {
          dialogFrame: null,
          contentFrame: null,
          textAtDialogFrame: '',
          heightAtDialogFrame: 0,
        },
      };
      window.__firstFrame = probe;
      let frame = 0;
      const tick = (): void => {
        const dialog = document.querySelector(`[data-testid="${dialogId}"]`);
        const content = dialog?.querySelector(`[data-testid="${contentId}"]`) ?? null;
        const text = content?.textContent?.trim() ?? '';
        if (dialog && probe.result.dialogFrame === null) {
          probe.result.dialogFrame = frame;
          probe.result.textAtDialogFrame = text;
          probe.result.heightAtDialogFrame = content?.getBoundingClientRect().height ?? 0;
        }
        if (dialog && text && probe.result.contentFrame === null) probe.result.contentFrame = frame;
        frame += 1;
        if (probe.result.contentFrame !== null || frame > (max as number)) {
          probe.settled = true;
          return;
        }
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    },
    [dialogTestId, contentTestId, MAX_FRAMES] as const,
  );
}

/** Resolves once the armed probe settles. Content is instant when both frames are equal. */
export async function firstFrame(page: Page): Promise<FirstFrame> {
  await page.waitForFunction(() => window.__firstFrame?.settled === true);
  return page.evaluate(() => (window.__firstFrame as Probe).result);
}

/** Frames the content lagged the dialog by; Infinity when the content never appeared. */
export function framesLate(r: FirstFrame): number {
  if (r.dialogFrame === null || r.contentFrame === null) return Number.POSITIVE_INFINITY;
  return r.contentFrame - r.dialogFrame;
}
