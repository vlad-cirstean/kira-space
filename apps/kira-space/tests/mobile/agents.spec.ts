import type { Locator } from '@playwright/test';
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
