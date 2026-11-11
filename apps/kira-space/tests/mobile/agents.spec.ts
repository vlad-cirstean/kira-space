import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { Locator } from '@playwright/test';
import { contract } from '../ui/support/contract';
import { expect, test } from './fixtures';
import type { MobileServer } from './support/mockServer';

// Replying to an agent and taking over a run, from the Need you tab.

const t = (id: string) => `[data-testid="${id}"]`;

// Claude Code asks a question: a Notification hook event for the TUI session's terminal.
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
  agentId: '',
  background: 0,
};

// Hook events sent before the app subscribed are lost, so send again until the question shows.
const waitForQuestion = (server: MobileServer, item: Locator) =>
  expect(async () => {
    server.emit('kira:agent:event', ASK);
    await expect(item).toBeVisible({ timeout: 500 });
  }).toPass();

test.describe('with agent input', () => {
  test.beforeEach(async ({ app, server }) => {
    server.state.auth = 'ok';
    server.state.permissions = { write: true, agentInput: true };
    server.state.agentInputGlobal = true;
    server.state.agentSessions = [
      { terminalId: 'term-9ab0', cwd: '~/kira-space-wts/web-app/oauth-login' },
    ];
    await app();
  });

  test('Reply pastes the text into the waiting session', async ({ page, server }) => {
    const item = page.locator(t('needs-item')).filter({ hasText: 'waiting for your answer' });
    await waitForQuestion(server, item);
    await item.locator(t('needs-reply')).click();
    await page.locator(t('reply-input')).fill('yes, run it');
    await page.locator(t('reply-send')).click();
    await expect(page.locator(t('reply-sheet'))).toHaveCount(0);
    const sent = server.state.requests.filter((r) => r.path === '/api/agent/sessions/9ab0/send');
    expect(sent).toHaveLength(1);
    expect(JSON.parse(sent[0]?.body ?? '{}')).toEqual({ message: 'yes, run it' });
    expect(sent[0]?.key).not.toBe('');
    await expect(item.locator(t('needs-terminal'))).toBeVisible();
  });

  test('closing a reply with text asks before discarding it', async ({ page, server }) => {
    const item = page.locator(t('needs-item')).filter({ hasText: 'waiting for your answer' });
    await waitForQuestion(server, item);
    await item.locator(t('needs-reply')).click();
    await page.locator(t('reply-input')).fill('yes, run it');
    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog', { name: 'Discard unsaved edits' })).toBeVisible();
    await expect(page.locator(t('reply-sheet'))).toBeVisible();
  });
});

test('without agent input a waiting agent shows the hint, not Reply', async ({
  page,
  server,
  app,
}) => {
  server.state.auth = 'ok';
  server.state.permissions = { write: true, agentInput: false };
  server.state.agentSessions = [
    { terminalId: 'term-9ab0', cwd: '~/kira-space-wts/web-app/oauth-login' },
  ];
  await app();
  await waitForQuestion(
    server,
    page.locator(t('needs-item')).filter({ hasText: 'waiting for your answer' }),
  );
  await expect(page.locator(t('permission-hint'))).toBeVisible();
  await expect(page.locator(t('needs-reply'))).toHaveCount(0);
});

// Contract ade-session-status, backend half: adeflow TestSessionStatusBackgroundAndUnseen. The
// terminal button of a finished session nobody viewed carries the unseen glyph.
test('contract: a finished session nobody viewed shows the unseen glyph on its terminal button', async ({
  page,
  server,
  app,
}) => {
  const [finished] = contract<{ sessions: Record<string, unknown>[] }>(
    'ade-session-status',
    'AdeTaskService.Sessions#unseen',
  ).sessions;
  const sessions = readFileSync(resolve(__dirname, '../fixtures/ade-v2/sessions.json'), 'utf8');
  server.state.auth = 'ok';
  server.state.permissions = { write: true, agentInput: true };
  server.state.agentInputGlobal = true;
  server.state.replies.sessions = {
    sessions: [
      ...(JSON.parse(sessions) as { sessions: Record<string, unknown>[] }).sessions,
      { ...finished, id: 'ss01', terminalId: 'term-ss01', taskId: 'T_cart', branchId: 'b_cart' },
    ],
  };
  await app();
  await page.locator(t('tab-plan')).click();
  const card = page
    .locator(t('plan-task'))
    .filter({ hasText: 'Cart total test fails intermittently' });
  await card.locator('button').first().click();
  const terminal = card.locator(t('plan-terminal'));
  await expect(terminal).toHaveCount(1);
  await expect(terminal).toHaveAttribute('data-activity', 'unseen');
  await expect(terminal.locator('[data-activity="unseen"]')).toBeVisible();
});
