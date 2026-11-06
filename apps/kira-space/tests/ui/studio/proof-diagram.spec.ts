import type { Page } from '@playwright/test';
import { expect, studioTest as test } from '../fixtures';
import { installClipboardSpy } from './support/clipboard';
import { diagramControl, openProofWindow, pngHeight } from './support/proof';

// UI half of the proof-diagram scenario; the Go half is proofflow TestProofDiagram. Every answer is
// that test's recorded result, so a payload the backend stores is what the window draws here.

const CAPTURE_MS = 60_000;

const TITLES = ['#1 Diagram 1', '#2 Diagram 2', '#3 Diagram 3', '#4 Diagram 4'];
const tab = (page: Page, n: number) =>
  page.locator('[data-testid="tab"]', { hasText: TITLES[n - 1] });
const diagram = (page: Page) => page.locator('[data-testid="proof-diagram"]');
const stepReady = (page: Page) =>
  expect(page.locator('[data-testid="proof-step"][data-ready="true"]').first()).toBeVisible();

async function open(
  relaunch: (o: Record<string, unknown>) => Promise<{ window: Page }>,
  url: string,
) {
  const out = await relaunch({ control: diagramControl(), timezoneId: 'UTC' });
  await openProofWindow(out.window, url);
  await stepReady(out.window);
  return out;
}

test('contract: flowchart and sequence tabs draw', async ({ relaunch, uiServer }) => {
  const { window: page } = await open(relaunch, uiServer.url);
  await tab(page, 1).click();
  await expect(diagram(page)).toHaveAttribute('data-diagram-type', 'flowchart');
  await expect(diagram(page).locator('svg')).toBeVisible();
  await expect(diagram(page).locator('svg text', { hasText: 'Checkout' }).first()).toBeVisible();
  await stepReady(page);

  await tab(page, 2).click();
  await expect(diagram(page)).toHaveAttribute('data-diagram-type', 'sequenceDiagram');
  await expect(diagram(page).locator('svg text', { hasText: 'API' }).first()).toBeVisible();
  await stepReady(page);
});

test('contract: the drawn svg holds no script, image or outside link', async ({
  relaunch,
  uiServer,
}) => {
  const { window: page } = await open(relaunch, uiServer.url);
  await tab(page, 1).click();
  await expect(diagram(page).locator('svg')).toBeVisible();
  await stepReady(page);
  const unsafe = await diagram(page).evaluate((root) => ({
    tags: root.querySelectorAll('script, foreignObject, image, a, iframe').length,
    links: [...root.querySelectorAll('*')].filter((el) =>
      ['href', 'xlink:href'].some((a) => {
        const v = el.getAttribute(a);
        return v !== null && !v.startsWith('#');
      }),
    ).length,
  }));
  expect(unsafe).toEqual({ tags: 0, links: 0 });
});

// Mermaid 12.2.0 fails an image node whose load the CSP blocks, so the window shows its error state
// with the source; the point is that nothing leaves the window.
test('contract: a remote image node is never fetched', async ({ relaunch, uiServer }) => {
  let remote = 0;
  const { window: page } = await open(relaunch, uiServer.url);
  await page.route('**/example.invalid/**', (route) => {
    remote++;
    return route.abort();
  });
  await tab(page, 3).click();
  await expect(page.locator('[data-testid="proof-diagram-error"]')).toBeVisible();
  await expect(page.locator('[data-testid="proof-diagram-source"]')).toContainText(
    'example.invalid',
  );
  await expect(diagram(page).locator('image')).toHaveCount(0);
  await stepReady(page);
  expect(remote).toBe(0);
});

test('contract: a diagram that does not parse shows the message and its source', async ({
  relaunch,
  uiServer,
}) => {
  const { window: page } = await open(relaunch, uiServer.url);
  await tab(page, 4).click();
  const error = page.locator('[data-testid="proof-diagram-error"]');
  await expect(error).toBeVisible();
  await expect(error).toContainText('Diagram does not parse');
  await expect(error).toContainText(/expecting|parse error/i);
  await expect(page.locator('[data-testid="proof-diagram-source"]')).toContainText('flowchart TD');
  await expect(diagram(page)).toHaveCount(0);
  await stepReady(page);
});

test('contract: the copied screenshot holds the diagram', async ({ relaunch, uiServer }) => {
  test.setTimeout(180_000);
  const { window: page } = await open(relaunch, uiServer.url);
  await tab(page, 1).click();
  await expect(diagram(page).locator('svg')).toBeVisible();
  await installClipboardSpy(page, { image: true });
  const button = page.locator('[data-testid="proof-shot"]');
  await button.hover();
  await button.click();
  await expect
    .poll(
      () =>
        page.evaluate(
          () => (window as unknown as { __clipboardImages: Blob[] }).__clipboardImages.length,
        ),
      { timeout: CAPTURE_MS },
    )
    .toBe(1);
  const bytes = await page.evaluate(async () => {
    const blob = (window as unknown as { __clipboardImages: Blob[] }).__clipboardImages[0];
    const buf = new Uint8Array((await blob?.arrayBuffer()) ?? new ArrayBuffer(0));
    return btoa(String.fromCharCode(...buf.slice(0, 44)));
  });
  expect(pngHeight(bytes)).toBeGreaterThan(100);
});
