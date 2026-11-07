import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { Locator } from '@playwright/test';
import { expect, test } from './fixtures';
import type { MobileServer } from './support/mockServer';

// P266 Stream B: touch targets and shared empty states on the phone.

const t = (id: string) => `[data-testid="${id}"]`;

const ASK = {
  terminalId: 'term-9ab0',
  event: 'Notification',
  sessionId: 'claude-9ab0',
  cwd: '',
  toolName: '',
  toolUseId: '',
  notificationType: 'permission_prompt',
  message: 'Run the migration?',
  source: '',
  reason: '',
  lastAssistantMessage: '',
};

const waitForQuestion = (server: MobileServer, item: Locator) =>
  expect(async () => {
    server.emit('kira:agent:event', ASK);
    await expect(item).toBeVisible({ timeout: 500 });
  }).toPass();

test('the reply sheet buttons are at least 44px tall', async ({ page, server, app }) => {
  server.state.auth = 'ok';
  server.state.permissions = { write: true, agentInput: true };
  server.state.agentInputGlobal = true;
  server.state.agentSessions = [
    { terminalId: 'term-9ab0', cwd: '~/kira-space-wts/web-app/oauth-login' },
  ];
  await app();
  const item = page.locator(t('needs-item')).filter({ hasText: 'waiting for your answer' });
  await waitForQuestion(server, item);
  await item.locator(t('needs-reply')).click();
  const box = await page.locator(t('reply-cancel')).boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
});

test('empty Needs and Plan screens render the shared empty state', async ({
  page,
  server,
  app,
}) => {
  server.state.auth = 'ok';
  const board = JSON.parse(
    readFileSync(resolve(__dirname, '../fixtures/ade-v2/board.json'), 'utf8'),
  ) as { tasks: unknown[]; branches: unknown[] };
  server.state.replies.board = { ...board, tasks: [], branches: [] };
  await app();
  await expect(page.locator(`${t('needs-empty')}[data-slot="empty-state"]`)).toBeVisible();
  await page.locator(t('tab-plan')).click();
  await expect(page.locator(`${t('plan-empty')}[data-slot="empty-state"]`)).toBeVisible();
});
