import { expect, test } from './fixtures';

// P223: the phone app runs from plain HTTP on a LAN address, which is not a secure context. The
// host name below maps to the mock server (playwright.config.ts, mobile-android), so the page
// really is insecure: no crypto.randomUUID, no service worker, ws:// for the terminal.

const t = (id: string) => `[data-testid="${id}"]`;

test.skip(({ browserName }) => browserName !== 'chromium', 'host mapping is Chromium-only');

test('pairs, writes and attaches a terminal on an insecure origin', async ({ page, server }) => {
  server.reset();
  server.state.permissions = { write: true, agentInput: true };
  server.state.agentInputGlobal = true;
  const port = new URL(server.url).port;
  const base = `http://kira-lan.test:${port}/`;

  await page.routeWebSocket(/\/api\/agent\/sessions\/[^/]+\/terminal/, (ws) => {
    ws.send(JSON.stringify({ type: 'hello', offset: 0, cols: 80, rows: 24 }));
    ws.send(Buffer.from('claude> ready\r\n'));
  });

  await page.goto(base);
  expect(await page.evaluate(() => window.isSecureContext)).toBe(false);
  expect(await page.evaluate(() => navigator.serviceWorker?.controller ?? null)).toBeNull();

  await page.locator(t('pair-request')).click();
  await expect(page.locator(t('pair-waiting'))).toBeVisible();
  server.resolvePair('approve');
  await expect(page.locator(t('app-shell'))).toBeVisible();

  await page.locator(t('tab-later')).click();
  await page.locator(t('later-add')).fill('Works over plain HTTP');
  await page.locator(t('later-add-submit')).click();
  await expect(page.locator(t('later-add'))).toHaveValue('');
  const sent = server.state.requests.filter(
    (r) => r.method === 'POST' && r.path === '/api/ade/later/items',
  );
  expect(sent).toHaveLength(1);
  expect(sent[0]?.key).toMatch(
    /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
  );

  await page.goto(`${base}terminal/9ab0`);
  await expect(page.locator(t('term-status'))).toHaveText('Live');
  await expect(page.locator('.xterm-rows')).toContainText('claude> ready');
});
