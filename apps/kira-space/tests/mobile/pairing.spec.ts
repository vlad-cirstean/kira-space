import { contract } from '../ui/support/contract';
import { expect, test } from './fixtures';

// Pairing from a fresh phone: request, the match code, and the four ways it can end.

const t = (id: string) => `[data-testid="${id}"]`;

test('an unpaired phone shows the pairing screen with a request button', async ({ page, app }) => {
  await app();
  await expect(page.locator(t('pair-screen'))).toBeVisible();
  await expect(page.locator(t('pair-request'))).toBeEnabled();
  await expect(page).toHaveURL(/\/pair$/);
});

test('requesting access shows a 4-digit code and the same code is sent to the computer', async ({
  page,
  app,
  server,
}) => {
  await app();
  await page.locator(t('pair-label')).fill('Vlad iPhone');
  await page.locator(t('pair-request')).click();
  const code = (await page.locator(t('pair-code')).textContent())?.trim() ?? '';
  expect(code).toMatch(/^\d{4}$/);
  await expect
    .poll(() => server.state.requests.find((r) => r.path === '/api/pair')?.body)
    .toBeTruthy();
  const sent = JSON.parse(server.state.requests.find((r) => r.path === '/api/pair')?.body ?? '{}');
  expect(sent).toEqual({ label: 'Vlad iPhone', code });
});

test('contract: approval lands on the app and shows the three tabs', async ({
  page,
  app,
  server,
}) => {
  server.state.replies.pair = contract('mobile-pairing', 'http:POST /api/pair#approved');
  server.state.replies.me = contract('mobile-pairing', 'http:GET /api/me#paired');
  await app();
  await page.locator(t('pair-request')).click();
  await expect(page.locator(t('pair-waiting'))).toBeVisible();
  server.resolvePair('approve');
  await expect(page.locator(t('app-shell'))).toBeVisible();
  await expect(page).toHaveURL(/\/needs$/);
  for (const tab of ['needs', 'plan', 'later'])
    await expect(page.locator(t(`tab-${tab}`))).toBeVisible();
});

test('a denied request says so and allows another try', async ({ page, app, server }) => {
  await app();
  await page.locator(t('pair-request')).click();
  await expect(page.locator(t('pair-waiting'))).toBeVisible();
  server.resolvePair('deny');
  await expect(page.locator(t('pair-notice'))).toContainText('denied');
  await expect(page.locator(t('pair-request'))).toBeEnabled();
});

test('a timed-out request says nobody answered', async ({ page, app, server }) => {
  await app();
  await page.locator(t('pair-request')).click();
  await expect(page.locator(t('pair-waiting'))).toBeVisible();
  server.resolvePair('timeout');
  await expect(page.locator(t('pair-notice'))).toContainText('Nobody answered');
});

test('a revoked device returns to the pairing screen', async ({ page, app, server }) => {
  server.state.auth = 'ok';
  await app();
  await expect(page.locator(t('app-shell'))).toBeVisible();
  // Settled: the first reads and the event stream are done, so no background request meets the revoke.
  await expect(page.locator(t('connection-dot'))).toHaveAttribute('aria-label', 'Connection live');
  await page.waitForLoadState('networkidle');
  server.state.auth = 'revoked';
  // A push makes the phone read the board, which now meets the revoke.
  server.emit('kira:adetask:board', null);
  await expect(page.locator(t('pair-screen'))).toBeVisible();
  await expect(page.locator(t('pair-notice'))).toContainText('removed in Kira Space');
});

test('contract: an expired device returns to the pairing screen with a notice', async ({
  page,
  app,
  server,
}) => {
  server.state.replies.expired = contract('mobile-pairing', 'http:GET /api/me#expired');
  server.state.auth = 'ok';
  await app();
  await expect(page.locator(t('app-shell'))).toBeVisible();
  await expect(page.locator(t('connection-dot'))).toHaveAttribute('aria-label', 'Connection live');
  await page.waitForLoadState('networkidle');
  server.state.auth = 'expired';
  server.emit('kira:adetask:board', null);
  await expect(page.locator(t('pair-screen'))).toBeVisible();
  await expect(page.locator(t('pair-notice'))).toContainText('expired');
});
